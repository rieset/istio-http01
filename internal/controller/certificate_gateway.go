/*
 * Функции, определенные в этом файле:
 *
 * - (r *CertificateReconciler) updateGatewayWithTemporarySecret(ctx, gateway, cert, originalSecretName, tempSecretName, secretNamespace) error
 *   Обновляет Gateway для использования временного секрета и отключает HSTS
 *
 * - (r *CertificateReconciler) restoreGatewayOriginalSecret(ctx, gateway, originalSecretName, secretNamespace) error
 *   Восстанавливает оригинальный секрет в Gateway и включает обратно HSTS
 *
 * - (r *CertificateReconciler) disableHTTPSRedirectForHTTP01(ctx, gateway, originalSecretName, secretNamespace) error
 *   Отключает httpsRedirect в Gateway для прохождения HTTP01 challenge
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// updateGatewayWithTemporarySecret обновляет Gateway для использования временного секрета и отключает HSTS
func (r *CertificateReconciler) updateGatewayWithTemporarySecret(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, cert *certmanagerv1.Certificate, originalSecretName, tempSecretName, secretNamespace string) error {
	logger := log.FromContext(ctx)

	// Получаем актуальную версию Gateway
	gatewayKey := client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
	updatedGateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, gatewayKey, updatedGateway); err != nil {
		return fmt.Errorf("failed to get Gateway: %w", err)
	}

	// Определяем формат credentialName (с namespace или без)
	credentialName := tempSecretName
	if secretNamespace != "" && secretNamespace != gateway.Namespace {
		credentialName = fmt.Sprintf("%s/%s", secretNamespace, tempSecretName)
	}

	// Обновляем credentialName в HTTPS серверах и отключаем httpsRedirect на HTTP серверах
	updated := false
	httpsRedirectDisabled := false

	for i := range updatedGateway.Spec.Servers {
		server := updatedGateway.Spec.Servers[i]

		// Обновляем HTTPS серверы (порт 443) - меняем credentialName на временный
		if server.Port != nil && server.Port.Number == 443 && server.Tls != nil {
			currentCredentialName := server.Tls.CredentialName
			var matches bool
			if strings.Contains(currentCredentialName, "/") {
				parts := strings.Split(currentCredentialName, "/")
				if len(parts) == 2 && parts[0] == secretNamespace && parts[1] == originalSecretName {
					matches = true
				}
			} else {
				// Формат "name" - проверяем совпадение имени независимо от namespace
				if currentCredentialName == originalSecretName {
					matches = true
				}
			}

			if matches {
				updatedGateway.Spec.Servers[i].Tls.CredentialName = credentialName
				updated = true
			}
		}

		// Отключаем httpsRedirect на HTTP серверах (порт 80) для прохождения HTTP01 challenge
		if server.Port != nil && server.Port.Number == 80 && server.Tls != nil && server.Tls.HttpsRedirect {
			// Сохраняем оригинальное значение httpsRedirect в аннотации
			if updatedGateway.Annotations == nil {
				updatedGateway.Annotations = make(map[string]string)
			}
			httpsRedirectKey := fmt.Sprintf("istio-http01.rieset.io/original-https-redirect-%s", originalSecretName)
			if _, exists := updatedGateway.Annotations[httpsRedirectKey]; !exists {
				updatedGateway.Annotations[httpsRedirectKey] = tempLabelValue
			}

			// Отключаем httpsRedirect
			updatedGateway.Spec.Servers[i].Tls.HttpsRedirect = false
			httpsRedirectDisabled = true
			updated = true
		}
	}

	if updated {
		// Добавляем аннотацию для отслеживания оригинального секрета
		if updatedGateway.Annotations == nil {
			updatedGateway.Annotations = make(map[string]string)
		}
		originalCredentialKey := fmt.Sprintf("istio-http01.rieset.io/original-credential-name-%s", originalSecretName)
		originalCredentialValue := originalSecretName
		if secretNamespace != "" && secretNamespace != gateway.Namespace {
			originalCredentialValue = fmt.Sprintf("%s/%s", secretNamespace, originalSecretName)
		}
		updatedGateway.Annotations[originalCredentialKey] = originalCredentialValue

		if err := r.Update(ctx, updatedGateway); err != nil {
			return fmt.Errorf("failed to update Gateway: %w", err)
		}

		logger.Info("Updated Gateway to use temporary self-signed certificate",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"originalSecretName", originalSecretName,
			"tempSecretName", tempSecretName,
			"httpsRedirectDisabled", httpsRedirectDisabled,
		)

		// Создаем EnvoyFilter для отключения HSTS (если еще не создан)
		// EnvoyFilter должен быть создан уже при создании временного сертификата,
		// но проверяем на случай, если он был удален или не был создан
		if err := r.createEnvoyFilterToDisableHSTS(ctx, gateway, originalSecretName); err != nil {
			logger.Error(err, "failed to create EnvoyFilter to disable HSTS",
				"gatewayName", gateway.Name,
				"gatewayNamespace", gateway.Namespace,
			)
		}

		// Проверяем временный сертификат через HTTPS
		// Получаем домены для Gateway
		domains, err := r.getDomainsForGateway(ctx, gateway)
		if err != nil {
			logger.Error(err, "failed to get domains for Gateway",
				"gatewayName", gateway.Name,
				"gatewayNamespace", gateway.Namespace,
			)
		} else if len(domains) > 0 {
			// Получаем IP адрес ingress gateway
			ingressIP, err := r.getIngressGatewayIP(ctx, gateway)
			if err != nil {
				logger.Error(err, "failed to get ingress gateway IP",
					"gatewayName", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
				)
			} else if ingressIP != "" {
				// Получаем DNS имена из временного сертификата
				tempCertName := fmt.Sprintf("%s-temp-selfsigned", cert.Name)
				tempCert := &certmanagerv1.Certificate{}
				if err := r.Get(ctx, client.ObjectKey{
					Name:      tempCertName,
					Namespace: cert.Namespace,
				}, tempCert); err == nil {
					// Проверяем сертификат через HTTPS
					if err := r.verifyCertificateViaHTTPS(ctx, gateway, tempCert.Spec.DNSNames, ingressIP); err != nil {
						logger.Error(err, "failed to verify temporary certificate via HTTPS",
							"gatewayName", gateway.Name,
							"gatewayNamespace", gateway.Namespace,
							"ingressIP", ingressIP,
						)
					}
				}
			}
		}
	}

	return nil
}

// restoreGatewayOriginalSecret восстанавливает оригинальный секрет в Gateway и включает обратно HSTS
func (r *CertificateReconciler) restoreGatewayOriginalSecret(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, originalSecretName, secretNamespace string) error {
	logger := log.FromContext(ctx)

	// Получаем актуальную версию Gateway
	gatewayKey := client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
	updatedGateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, gatewayKey, updatedGateway); err != nil {
		return fmt.Errorf("failed to get Gateway: %w", err)
	}

	// Определяем формат credentialName (с namespace или без)
	originalCredentialName := originalSecretName
	if secretNamespace != "" && secretNamespace != gateway.Namespace {
		originalCredentialName = fmt.Sprintf("%s/%s", secretNamespace, originalSecretName)
	}

	// Проверяем, используется ли временный секрет
	tempSecretPrefix := fmt.Sprintf("%s-temp", originalSecretName)
	needsRestoreSecret := false

	for i := range updatedGateway.Spec.Servers {
		server := updatedGateway.Spec.Servers[i]
		if server.Tls == nil {
			continue
		}

		currentCredentialName := server.Tls.CredentialName
		// Проверяем, является ли текущий секрет временным
		if strings.Contains(currentCredentialName, tempSecretPrefix) {
			updatedGateway.Spec.Servers[i].Tls.CredentialName = originalCredentialName
			needsRestoreSecret = true
		}
	}

	// Проверяем, нужно ли восстановить httpsRedirect (даже если секрет уже оригинальный)
	httpsRedirectKey := fmt.Sprintf("istio-http01.rieset.io/original-https-redirect-%s", originalSecretName)
	shouldRestoreRedirect := false
	if updatedGateway.Annotations != nil {
		if redirectValue, exists := updatedGateway.Annotations[httpsRedirectKey]; exists && redirectValue == tempLabelValue {
			shouldRestoreRedirect = true
		}
	}

	// Восстанавливаем httpsRedirect на HTTP серверах, если он был отключен
	needsRestoreRedirect := false
	if shouldRestoreRedirect {
		for i := range updatedGateway.Spec.Servers {
			server := updatedGateway.Spec.Servers[i]
			// Восстанавливаем httpsRedirect на HTTP серверах (порт 80), если он отключен
			if server.Port != nil && server.Port.Number == 80 && server.Tls != nil && !server.Tls.HttpsRedirect {
				updatedGateway.Spec.Servers[i].Tls.HttpsRedirect = true
				needsRestoreRedirect = true
			}
		}
	}

	// Обновляем Gateway, если нужно восстановить секрет или httpsRedirect
	if needsRestoreSecret || needsRestoreRedirect {
		// Удаляем аннотации с оригинальными значениями
		if updatedGateway.Annotations != nil {
			originalCredentialKey := fmt.Sprintf("istio-http01.rieset.io/original-credential-name-%s", originalSecretName)
			delete(updatedGateway.Annotations, originalCredentialKey)
			if needsRestoreRedirect {
				delete(updatedGateway.Annotations, httpsRedirectKey)
			}
		}

		if err := r.Update(ctx, updatedGateway); err != nil {
			return fmt.Errorf("failed to update Gateway: %w", err)
		}

		logger.Info("Restored original secret and httpsRedirect in Gateway",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"originalSecretName", originalSecretName,
			"secretRestored", needsRestoreSecret,
			"httpsRedirectRestored", needsRestoreRedirect,
		)
	}

	// Удаляем EnvoyFilter для отключения HSTS (включаем обратно HSTS)
	// Делаем это всегда, даже если Gateway уже восстановлен, чтобы убедиться, что EnvoyFilter удален
	logger.Info("Попытка удалить EnvoyFilter для HSTS",
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"originalSecretName", originalSecretName,
	)
	if err := r.deleteEnvoyFilterForHSTS(ctx, gateway, originalSecretName); err != nil {
		logger.Error(err, "failed to delete EnvoyFilter for HSTS",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
	}

	// Проверяем финальный сертификат через HTTPS
	// Получаем домены для Gateway
	domains, err := r.getDomainsForGateway(ctx, gateway)
	if err != nil {
		logger.Error(err, "failed to get domains for Gateway",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
	} else if len(domains) > 0 {
		// Получаем IP адрес ingress gateway
		ingressIP, err := r.getIngressGatewayIP(ctx, gateway)
		if err != nil {
			logger.Error(err, "failed to get ingress gateway IP",
				"gatewayName", gateway.Name,
				"gatewayNamespace", gateway.Namespace,
			)
		} else if ingressIP != "" {
			// Получаем Certificate для проверки DNS имен
			cert := r.findCertificateBySecretName(ctx, originalSecretName, secretNamespace)
			if cert != nil {
				// Проверяем сертификат через HTTPS
				if err := r.verifyCertificateViaHTTPS(ctx, gateway, cert.Spec.DNSNames, ingressIP); err != nil {
					logger.Error(err, "failed to verify certificate via HTTPS after restore",
						"gatewayName", gateway.Name,
						"gatewayNamespace", gateway.Namespace,
						"ingressIP", ingressIP,
						"certificateName", cert.Name,
						"certificateNamespace", cert.Namespace,
					)
				}
			} else {
				// Если не удалось получить Certificate, проверяем доступность через HTTP
				if err := r.verifyCertificateViaHTTP(ctx, gateway, ingressIP); err != nil {
					logger.Error(err, "failed to verify certificate via HTTP",
						"gatewayName", gateway.Name,
						"gatewayNamespace", gateway.Namespace,
						"ingressIP", ingressIP,
					)
				}
			}
		}
	}

	return nil
}

// disableHTTPSRedirectForHTTP01 отключает httpsRedirect в Gateway для прохождения HTTP01 challenge
// НЕ меняет HTTPS сервер, чтобы избежать проблем с HSTS
func (r *CertificateReconciler) disableHTTPSRedirectForHTTP01(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, originalSecretName, _ string) error {
	logger := log.FromContext(ctx)

	// Получаем актуальную версию Gateway
	gatewayKey := client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
	updatedGateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, gatewayKey, updatedGateway); err != nil {
		return fmt.Errorf("failed to get Gateway: %w", err)
	}

	// Проверяем, не отключен ли уже httpsRedirect
	httpsRedirectKey := fmt.Sprintf("istio-http01.rieset.io/original-https-redirect-%s", originalSecretName)
	alreadyDisabled := false
	if updatedGateway.Annotations != nil {
		if _, exists := updatedGateway.Annotations[httpsRedirectKey]; exists {
			alreadyDisabled = true
		}
	}

	// Проверяем и отключаем httpsRedirect на HTTP серверах (порт 80)
	updated := false
	for i := range updatedGateway.Spec.Servers {
		server := updatedGateway.Spec.Servers[i]
		if server.Port != nil && server.Port.Number == 80 && server.Tls != nil && server.Tls.HttpsRedirect {
			// Сохраняем оригинальное значение httpsRedirect в аннотации
			if updatedGateway.Annotations == nil {
				updatedGateway.Annotations = make(map[string]string)
			}
			if !alreadyDisabled {
				updatedGateway.Annotations[httpsRedirectKey] = tempLabelValue
			}

			// Отключаем httpsRedirect
			updatedGateway.Spec.Servers[i].Tls.HttpsRedirect = false
			updated = true
		}
	}

	if updated {
		if err := r.Update(ctx, updatedGateway); err != nil {
			return fmt.Errorf("failed to update Gateway: %w", err)
		}

		logger.Info("Disabled httpsRedirect in Gateway for HTTP01 challenge (HTTPS server unchanged to avoid HSTS issues)",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"originalSecretName", originalSecretName,
		)
	}

	return nil
}
