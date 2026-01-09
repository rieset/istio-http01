/*
 * Функции, определенные в этом файле:
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
 * - (r *CertificateReconciler) isGatewayUsingSecret(ctx, gateway, secretName, secretNamespace) bool
 *   Проверяет, использует ли Gateway указанный секрет
 *
 * - (r *CertificateReconciler) findCertificateBySecretName(ctx, secretName, secretNamespace) *Certificate
 *   Находит Certificate по имени секрета
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// isCertificateReady проверяет, готов ли Certificate (выпущен ли сертификат)
func (r *CertificateReconciler) isCertificateReady(cert *certmanagerv1.Certificate) bool {
	// Проверяем условие Ready в статусе Certificate
	for _, condition := range cert.Status.Conditions {
		if condition.Type == certmanagerv1.CertificateConditionReady {
			return condition.Status == certmanagermetav1.ConditionTrue
		}
	}
	// Если условие Ready не найдено, считаем, что сертификат не готов
	return false
}

// findGatewaysUsingCertificate находит все Gateway, которые используют указанный сертификат
// Ищет Gateway, которые используют либо оригинальный секрет, либо временный секрет (с суффиксом -temp)
func (r *CertificateReconciler) findGatewaysUsingCertificate(ctx context.Context, secretName, secretNamespace string) ([]*istionetworkingv1beta1.Gateway, error) {
	logger := log.FromContext(ctx)

	// Получение всех Gateway во всех namespace
	gatewayList := &istionetworkingv1beta1.GatewayList{}
	if err := r.List(ctx, gatewayList, client.InNamespace("")); err != nil {
		return nil, fmt.Errorf("failed to list Gateways: %w", err)
	}

	var matchingGateways []*istionetworkingv1beta1.Gateway
	tempSecretName := fmt.Sprintf("%s-temp", secretName)

	// Проверяем каждый Gateway
	for i := range gatewayList.Items {
		gateway := gatewayList.Items[i]
		gatewayFound := false

		// Проверяем все серверы в Gateway
		for _, server := range gateway.Spec.Servers {
			if server.Tls == nil {
				continue
			}

			credentialName := server.Tls.CredentialName
			if credentialName == "" {
				continue
			}

			// Проверяем, используется ли наш secretName (оригинальный или временный) в credentialName
			// credentialName может быть в формате "name" или "namespace/name"
			var matches bool
			if strings.Contains(credentialName, "/") {
				// Формат "namespace/name"
				parts := strings.Split(credentialName, "/")
				if len(parts) == 2 && parts[0] == secretNamespace {
					// Проверяем совпадение с оригинальным или временным секретом
					if parts[1] == secretName || parts[1] == tempSecretName {
						matches = true
					}
				}
			} else {
				// Формат "name" - проверяем совпадение имени
				// Если имя совпадает с оригинальным или временным секретом, считаем это совпадением
				if credentialName == secretName || credentialName == tempSecretName {
					matches = true
				}
			}

			// Также проверяем аннотации Gateway на наличие ссылки на оригинальный секрет
			if !matches && gateway.Annotations != nil {
				originalCredentialKey := fmt.Sprintf("istio-http01.rieset.io/original-credential-name-%s", secretName)
				if originalCredentialValue, exists := gateway.Annotations[originalCredentialKey]; exists {
					// Если credentialName совпадает с временным секретом, а в аннотации есть ссылка на оригинальный
					if strings.Contains(credentialName, tempSecretName) {
						// Проверяем, что аннотация указывает на наш оригинальный секрет
						if strings.Contains(originalCredentialValue, secretName) {
							matches = true
						}
					}
				}
			}

			if matches {
				gatewayFound = true
				break // Найден сервер с совпадающим credentialName, переходим к следующему Gateway
			}
		}

		if gatewayFound {
			// Используем указатель на элемент массива
			matchingGateways = append(matchingGateways, gateway)
			logger.V(1).Info("Found Gateway using certificate",
				"gatewayName", gateway.Name,
				"gatewayNamespace", gateway.Namespace,
				"secretName", secretName,
				"secretNamespace", secretNamespace,
			)
		}
	}

	logger.V(1).Info("Search for Gateways using certificate completed",
		"secretName", secretName,
		"secretNamespace", secretNamespace,
		"foundCount", len(matchingGateways),
	)

	return matchingGateways, nil
}

// hasHTTPSRedirect проверяет, включен ли httpsRedirect в Gateway
func (r *CertificateReconciler) hasHTTPSRedirect(gateway *istionetworkingv1beta1.Gateway) bool {
	// Проверяем все серверы в Gateway
	for _, server := range gateway.Spec.Servers {
		// Ищем HTTP сервер (обычно порт 80) с включенным httpsRedirect
		if server.Port != nil && server.Port.Number == 80 {
			if server.Tls != nil && server.Tls.HttpsRedirect {
				return true
			}
		}
	}
	return false
}

// isGatewayUsingSecret проверяет, использует ли Gateway указанный секрет
func (r *CertificateReconciler) isGatewayUsingSecret(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, secretName, secretNamespace string) bool {
	// Получаем актуальную версию Gateway
	gatewayKey := client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
	updatedGateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, gatewayKey, updatedGateway); err != nil {
		return false
	}

	// Проверяем все серверы в Gateway
	for _, server := range updatedGateway.Spec.Servers {
		if server.Tls == nil {
			continue
		}

		credentialName := server.Tls.CredentialName
		if credentialName == "" {
			continue
		}

		// Проверяем, используется ли наш secretName в credentialName
		// credentialName может быть в формате "name" или "namespace/name"
		if strings.Contains(credentialName, "/") {
			// Формат "namespace/name"
			parts := strings.Split(credentialName, "/")
			if len(parts) == 2 && parts[0] == secretNamespace && parts[1] == secretName {
				return true
			}
		} else {
			// Формат "name" - проверяем совпадение имени
			if credentialName == secretName {
				return true
			}
		}
	}

	return false
}

// findCertificateBySecretName находит Certificate по имени секрета
func (r *CertificateReconciler) findCertificateBySecretName(ctx context.Context, secretName, secretNamespace string) *certmanagerv1.Certificate {
	logger := log.FromContext(ctx)

	// Получение всех Certificate в указанном namespace
	certList := &certmanagerv1.CertificateList{}
	if err := r.List(ctx, certList, client.InNamespace(secretNamespace)); err != nil {
		logger.V(1).Info("Failed to list Certificates",
			"secretNamespace", secretNamespace,
			"error", err,
		)
		return nil
	}

	// Ищем Certificate с совпадающим SecretName
	for i := range certList.Items {
		cert := certList.Items[i]
		if cert.Spec.SecretName == secretName {
			logger.V(1).Info("Found Certificate by secret name",
				"certificateName", cert.Name,
				"certificateNamespace", cert.Namespace,
				"secretName", secretName,
			)
			return &cert
		}
	}

	logger.V(1).Info("Certificate not found by secret name",
		"secretName", secretName,
		"secretNamespace", secretNamespace,
	)
	return nil
}
