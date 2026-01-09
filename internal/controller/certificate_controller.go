/*
 * Функции, определенные в этом файле:
 *
 * - (r *CertificateReconciler) Reconcile(ctx, req) (ctrl.Result, error)
 *   Обрабатывает изменения Certificate ресурсов и выводит информацию в логи
 *
 * - (r *CertificateReconciler) SetupWithManager(mgr) error
 *   Настраивает контроллер для работы с менеджером
 *
 * - (r *CertificateReconciler) isCertificateReady(cert) bool
 *   Проверяет, готов ли Certificate (выпущен ли сертификат)
 *
 * - (r *CertificateReconciler) findGatewaysUsingCertificate(ctx, secretName, secretNamespace) ([]*Gateway, error)
 *   Находит все Gateway, которые используют указанный сертификат
 *
 * - (r *CertificateReconciler) hasHTTPSRedirect(gateway) bool
 *   Проверяет, включен ли httpsRedirect в Gateway
 *
 * - (r *CertificateReconciler) createSelfSignedCertificate(ctx, cert, gateway) error
 *   Создает самоподписанный сертификат для Gateway когда основной не готов
 *
 * - (r *CertificateReconciler) updateGatewayWithTemporarySecret(ctx, gateway, originalSecretName, tempSecretName) error
 *   Обновляет Gateway для использования временного секрета и отключает HSTS
 *
 * - (r *CertificateReconciler) restoreGatewayOriginalSecret(ctx, gateway, originalSecretName) error
 *   Восстанавливает оригинальный секрет в Gateway и включает обратно HSTS
 *
 * - (r *CertificateReconciler) deleteTemporarySelfSignedCertificate(ctx, cert) error
 *   Удаляет временный самоподписанный сертификат и issuer
 *
 * - (r *CertificateReconciler) disableHTTPSRedirectForHTTP01(ctx, gateway, originalSecretName) error
 *   Отключает httpsRedirect в Gateway для прохождения HTTP01 challenge
 *
 * - (r *CertificateReconciler) createEnvoyFilterToDisableHSTS(ctx, gateway, originalSecretName) error
 *   Создает EnvoyFilter для отключения HSTS заголовка
 *
 * - (r *CertificateReconciler) deleteEnvoyFilterForHSTS(ctx, gateway, originalSecretName) error
 *   Удаляет EnvoyFilter для отключения HSTS
 *
 * - (r *CertificateReconciler) ensureTemporaryCertificateSetup(ctx, cert, gateway) error
 *   Проверяет и восстанавливает состояние временного сертификата, httpRedirect и EnvoyFilter
 *
 * - (r *CertificateReconciler) getDomainsForGateway(ctx, gateway) ([]string, error)
 *   Получает список доменов для Gateway из связанных VirtualService
 *
 * - (r *CertificateReconciler) getIngressGatewayIP(ctx, gateway) (string, error)
 *   Получает IP адрес ingress gateway для Gateway
 *
 * - (r *CertificateReconciler) verifyCertificateViaHTTPS(ctx, gateway, expectedDNSNames, ingressIP) error
 *   Проверяет сертификат через HTTPS запрос и сравнивает домены
 *
 * - (r *CertificateReconciler) verifyCertificateViaHTTP(ctx, gateway, ingressIP) error
 *   Проверяет доступность через HTTP запрос
 *
 * - (r *CertificateReconciler) findCertificateBySecretName(ctx, secretName, secretNamespace) *Certificate
 *   Находит Certificate по имени секрета
 */

package controller

import (
	"context"
	"fmt"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// tempLabelValue значение метки для временных ресурсов
	tempLabelValue = "true"
)

// CertificateReconciler реконсилирует Certificate ресурсы
type CertificateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	DebugMode bool
}

// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates/status,verbs=get
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=envoyfilters,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// Reconcile обрабатывает Certificate ресурсы
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Получение Certificate
	cert := &certmanagerv1.Certificate{}
	if err := r.Get(ctx, req.NamespacedName, cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Вывод информации о Certificate
	logger.Info("Certificate detected",
		"certificateName", cert.Name,
		"certificateNamespace", cert.Namespace,
		"dnsNames", cert.Spec.DNSNames,
		"issuerRef", fmt.Sprintf("%s/%s", cert.Spec.IssuerRef.Kind, cert.Spec.IssuerRef.Name),
		"secretName", cert.Spec.SecretName,
	)

	// Дополнительная информация о статусе
	if len(cert.Status.Conditions) > 0 {
		for _, condition := range cert.Status.Conditions {
			logger.Info("Certificate condition",
				"certificateName", cert.Name,
				"certificateNamespace", cert.Namespace,
				"type", condition.Type,
				"status", condition.Status,
				"reason", condition.Reason,
				"message", condition.Message,
			)
		}
	}

	// Информация о DNS names
	if len(cert.Spec.DNSNames) > 0 {
		logger.Info("Certificate DNS names",
			"certificateName", cert.Name,
			"certificateNamespace", cert.Namespace,
			"dnsNames", cert.Spec.DNSNames,
		)
	}

	// Информация о Common Name
	if cert.Spec.CommonName != "" {
		logger.Info("Certificate common name",
			"certificateName", cert.Name,
			"certificateNamespace", cert.Namespace,
			"commonName", cert.Spec.CommonName,
		)
	}

	// Проверка готовности сертификата
	isReady := r.isCertificateReady(cert)
	if !isReady {
		logger.Info("Certificate is not ready yet",
			"certificateName", cert.Name,
			"certificateNamespace", cert.Namespace,
			"secretName", cert.Spec.SecretName,
		)

		// Поиск Gateway, которые используют этот сертификат
		gateways, err := r.findGatewaysUsingCertificate(ctx, cert.Spec.SecretName, cert.Namespace)
		if err != nil {
			logger.Error(err, "failed to find Gateways using certificate",
				"certificateName", cert.Name,
				"secretName", cert.Spec.SecretName,
			)
		} else if len(gateways) > 0 {
			// Периодическая проверка и восстановление состояния для Gateway с httpsRedirect
			for _, gateway := range gateways {
				// Проверяем, что Gateway действительно связан с этим сертификатом
				// Проверяем по оригинальному или временному секрету
				originalSecretName := cert.Spec.SecretName
				tempSecretName := fmt.Sprintf("%s-temp", cert.Spec.SecretName)
				isGatewayRelated := r.isGatewayUsingSecret(ctx, gateway, originalSecretName, cert.Namespace) ||
					r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace) ||
					(gateway.Annotations != nil && gateway.Annotations[fmt.Sprintf("istio-http01.rieset.io/original-credential-name-%s", originalSecretName)] != "")

				if !isGatewayRelated {
					logger.V(1).Info("Gateway not related to certificate, skipping",
						"certificateName", cert.Name,
						"gatewayName", gateway.Name,
						"gatewayNamespace", gateway.Namespace,
					)
					continue
				}

				if r.hasHTTPSRedirect(gateway) {
					// Проверяем и восстанавливаем состояние временного сертификата, httpRedirect и EnvoyFilter
					if err := r.ensureTemporaryCertificateSetup(ctx, cert, gateway); err != nil {
						logger.Error(err, "failed to ensure temporary certificate setup",
							"certificateName", cert.Name,
							"gatewayName", gateway.Name,
							"gatewayNamespace", gateway.Namespace,
						)
					}
				} else {
					// Проверяем, использует ли Gateway временный секрет, но httpsRedirect не отключен
					if r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace) {
						// Gateway использует временный секрет, но httpsRedirect может быть включен
						// Отключаем его для прохождения HTTP01 challenge
						if err := r.disableHTTPSRedirectForHTTP01(ctx, gateway, cert.Spec.SecretName, cert.Namespace); err != nil {
							logger.Error(err, "failed to disable httpsRedirect for HTTP01 challenge",
								"gatewayName", gateway.Name,
								"gatewayNamespace", gateway.Namespace,
							)
						}
					}
				}
			}
		}
	} else {
		// Сертификат готов - проверяем, нужно ли восстановить оригинальный секрет в Gateway
		gateways, err := r.findGatewaysUsingCertificate(ctx, cert.Spec.SecretName, cert.Namespace)
		if err != nil {
			logger.Error(err, "failed to find Gateways using certificate",
				"certificateName", cert.Name,
				"secretName", cert.Spec.SecretName,
			)
		} else if len(gateways) > 0 {
			// Проверяем, использует ли Gateway временный секрет - если да, убеждаемся, что EnvoyFilter существует
			tempSecretName := fmt.Sprintf("%s-temp", cert.Spec.SecretName)
			for _, gateway := range gateways {
				usesTempSecret := r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace)
				logger.Info("Проверка использования секрета в Gateway для очистки EnvoyFilter",
					"gatewayName", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
					"usesTempSecret", usesTempSecret,
					"tempSecretName", tempSecretName,
				)
				if usesTempSecret {
					// Gateway использует временный секрет - проверяем наличие EnvoyFilter
					envoyFilterName := fmt.Sprintf("disable-hsts-%s-%s", gateway.Namespace, gateway.Name)
					envoyFilter := &istionetworkingv1alpha3.EnvoyFilter{}
					if err := r.Get(ctx, client.ObjectKey{
						Name:      envoyFilterName,
						Namespace: gateway.Namespace,
					}, envoyFilter); err != nil {
						// EnvoyFilter не существует - создаем его
						logger.Info("Certificate ready but Gateway uses temporary secret, ensuring EnvoyFilter exists",
							"gatewayName", gateway.Name,
							"gatewayNamespace", gateway.Namespace,
						)
						if err := r.createEnvoyFilterToDisableHSTS(ctx, gateway, cert.Spec.SecretName); err != nil {
							logger.Error(err, "failed to create EnvoyFilter for temporary certificate",
								"gatewayName", gateway.Name,
								"gatewayNamespace", gateway.Namespace,
							)
						}
					}
				} else {
					// Gateway использует оригинальный секрет - проверяем и удаляем EnvoyFilter, если он существует
					// Используем unstructured для проверки, так как тип v1alpha3.EnvoyFilter не зарегистрирован в схеме
					envoyFilterName := fmt.Sprintf("disable-hsts-%s-%s", gateway.Namespace, gateway.Name)
					envoyFilter := &unstructured.Unstructured{}
					envoyFilter.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "networking.istio.io",
						Version: "v1alpha3",
						Kind:    "EnvoyFilter",
					})
					envoyFilter.SetName(envoyFilterName)
					envoyFilter.SetNamespace(gateway.Namespace)
					err := r.Get(ctx, client.ObjectKey{
						Name:      envoyFilterName,
						Namespace: gateway.Namespace,
					}, envoyFilter)
					if err == nil {
						// EnvoyFilter существует, но Gateway использует оригинальный секрет - удаляем его
						logger.Info("Сертификат готов и Gateway использует оригинальный секрет, удаляем EnvoyFilter",
							"gatewayName", gateway.Name,
							"gatewayNamespace", gateway.Namespace,
							"envoyFilterName", envoyFilterName,
						)
						if err := r.deleteEnvoyFilterForHSTS(ctx, gateway, cert.Spec.SecretName); err != nil {
							logger.Error(err, "не удалось удалить EnvoyFilter для HSTS",
								"gatewayName", gateway.Name,
								"gatewayNamespace", gateway.Namespace,
							)
						}
					} else {
						logger.V(1).Info("EnvoyFilter не найден (возможно, уже удален)",
							"gatewayName", gateway.Name,
							"gatewayNamespace", gateway.Namespace,
							"envoyFilterName", envoyFilterName,
						)
					}
				}
			}

			// В debug режиме проверяем, прошло ли 5 минут с момента создания временного сертификата
			if r.DebugMode {
				tempCertName := fmt.Sprintf("%s-temp-selfsigned", cert.Name)
				tempCert := &certmanagerv1.Certificate{}
				if err := r.Get(ctx, client.ObjectKey{
					Name:      tempCertName,
					Namespace: cert.Namespace,
				}, tempCert); err == nil {
					// Временный сертификат существует - проверяем время создания
					creationTime := tempCert.CreationTimestamp.Time
					elapsed := time.Since(creationTime)
					minElapsed := 5 * time.Minute

					if elapsed < minElapsed {
						logger.Info("Debug mode: delaying certificate restoration",
							"certificateName", cert.Name,
							"elapsed", elapsed,
							"required", minElapsed,
							"remaining", minElapsed-elapsed,
						)
						// Возвращаем результат с временем до следующей проверки
						remaining := minElapsed - elapsed
						if remaining < 30*time.Second {
							remaining = 30 * time.Second
						}
						return ctrl.Result{RequeueAfter: remaining}, nil
					}
					logger.Info("Debug mode: 5 minutes elapsed, restoring certificate",
						"certificateName", cert.Name,
						"elapsed", elapsed,
					)
				}
			}

			// Восстанавливаем оригинальный секрет в Gateway и включаем обратно HSTS
			for _, gateway := range gateways {
				if err := r.restoreGatewayOriginalSecret(ctx, gateway, cert.Spec.SecretName, cert.Namespace); err != nil {
					logger.Error(err, "failed to restore original secret in Gateway",
						"certificateName", cert.Name,
						"gatewayName", gateway.Name,
						"gatewayNamespace", gateway.Namespace,
					)
				}
			}

			// Удаляем временный самоподписанный сертификат
			if err := r.deleteTemporarySelfSignedCertificate(ctx, cert); err != nil {
				logger.Error(err, "failed to delete temporary self-signed certificate",
					"certificateName", cert.Name,
				)
			}
		}
	}

	// Периодическая проверка для отслеживания готовности временных сертификатов
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// Вспомогательные функции перенесены в certificate_helpers.go

// createSelfSignedCertificate перенесена в certificate_temporary.go

// Функции Gateway перенесены в certificate_gateway.go

// ensureTemporaryCertificateSetup перенесена в certificate_temporary.go

// Функции проверки сертификатов перенесены в certificate_verification.go

// SetupWithManager настраивает контроллер
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certmanagerv1.Certificate{}).
		Complete(r)
}
