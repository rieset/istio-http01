/*
 * Функции, определенные в этом файле:
 *
 * - (r *CertificateReconciler) createSelfSignedCertificate(ctx, cert, gateway) error
 *   Создает самоподписанный сертификат для Gateway когда основной не готов
 *
 * - (r *CertificateReconciler) deleteTemporarySelfSignedCertificate(ctx, cert) error
 *   Удаляет временный самоподписанный сертификат и issuer
 *
 * - (r *CertificateReconciler) ensureTemporaryCertificateSetup(ctx, cert, gateway) error
 *   Проверяет и восстанавливает состояние временного сертификата, httpRedirect и EnvoyFilter
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createSelfSignedCertificate создает самоподписанный сертификат для Gateway
func (r *CertificateReconciler) createSelfSignedCertificate(ctx context.Context, cert *certmanagerv1.Certificate, gateway *istionetworkingv1beta1.Gateway) error {
	logger := log.FromContext(ctx)

	// Проверяем, что Gateway действительно связан с этим сертификатом
	originalSecretName := cert.Spec.SecretName
	tempSecretName := fmt.Sprintf("%s-temp", cert.Spec.SecretName)
	isGatewayRelated := r.isGatewayUsingSecret(ctx, gateway, originalSecretName, cert.Namespace) ||
		r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace) ||
		(gateway.Annotations != nil && gateway.Annotations[fmt.Sprintf("istio-http01.rieset.io/original-credential-name-%s", originalSecretName)] != "")

	if !isGatewayRelated {
		logger.Error(nil, "Gateway is not related to certificate, skipping temporary certificate creation",
			"certificateName", cert.Name,
			"certificateSecretName", originalSecretName,
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		return fmt.Errorf("gateway %s/%s is not related to certificate %s", gateway.Namespace, gateway.Name, cert.Name)
	}

	// Проверяем, не создан ли уже временный сертификат
	tempCertName := fmt.Sprintf("%s-temp-selfsigned", cert.Name)
	tempCert := &certmanagerv1.Certificate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      tempCertName,
		Namespace: cert.Namespace,
	}, tempCert); err == nil {
		// Временный сертификат уже существует - проверяем, готов ли он
		if r.isCertificateReady(tempCert) {
			// Временный сертификат готов - обновляем Gateway, если еще не обновлен
			if err := r.updateGatewayWithTemporarySecret(ctx, gateway, cert, cert.Spec.SecretName, tempSecretName, cert.Namespace); err != nil {
				logger.Error(err, "failed to update Gateway with temporary secret",
					"gatewayName", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
				)
			}
		}
		// Временный сертификат уже существует (готов или еще не готов)
		return nil
	}

	// Проверяем, не использует ли Gateway уже временный секрет
	if r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace) {
		logger.V(1).Info("Gateway already uses temporary secret",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"tempSecretName", tempSecretName,
		)
		return nil
	}

	// Создаем временный Issuer с self-signed типом
	issuerName := fmt.Sprintf("%s-temp-selfsigned-issuer", cert.Name)
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuerName,
			Namespace: cert.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "istio-http01",
				"istio-http01.rieset.io/temp":  tempLabelValue,
			},
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		},
	}

	// Создаем Issuer
	if err := r.Create(ctx, issuer); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create self-signed issuer: %w", err)
		}
		logger.V(1).Info("Self-signed issuer already exists",
			"issuerName", issuerName,
			"namespace", cert.Namespace,
		)
	}

	// Получаем домены Gateway из связанных VirtualService
	// Временный сертификат должен покрывать все домены Gateway, а не только DNS имена из оригинального сертификата
	gatewayDomains, err := r.getDomainsForGateway(ctx, gateway)
	if err != nil {
		logger.Error(err, "failed to get domains for Gateway, using certificate DNS names",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		// Если не удалось получить домены Gateway, используем DNS имена из оригинального сертификата
		gatewayDomains = cert.Spec.DNSNames
	}

	// Объединяем домены Gateway и DNS имена из сертификата, чтобы покрыть все возможные домены
	// Используем map для исключения дубликатов
	allDNSNames := make(map[string]bool)
	for _, dnsName := range cert.Spec.DNSNames {
		allDNSNames[dnsName] = true
	}
	for _, domain := range gatewayDomains {
		allDNSNames[domain] = true
	}

	// Преобразуем map в slice
	dnsNamesList := make([]string, 0, len(allDNSNames))
	for dnsName := range allDNSNames {
		dnsNamesList = append(dnsNamesList, dnsName)
	}

	logger.Info("Creating temporary certificate with DNS names from Gateway and Certificate",
		"gatewayDomains", gatewayDomains,
		"certificateDNSNames", cert.Spec.DNSNames,
		"combinedDNSNames", dnsNamesList,
	)

	// Создаем временный Certificate с self-signed issuer
	tempCertificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tempCertName,
			Namespace: cert.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":         "istio-http01",
				"istio-http01.rieset.io/temp":          tempLabelValue,
				"istio-http01.rieset.io/original-cert": cert.Name,
			},
		},
		Spec: certmanagerv1.CertificateSpec{
			SecretName:  tempSecretName,
			DNSNames:    dnsNamesList,
			CommonName:  cert.Spec.CommonName,
			Duration:    &metav1.Duration{Duration: 24 * 60 * 60 * 1000000000}, // 24 часа
			RenewBefore: &metav1.Duration{Duration: 1 * 60 * 60 * 1000000000},  // 1 час до истечения
			IssuerRef: certmanagermetav1.ObjectReference{
				Name:  issuerName,
				Kind:  "Issuer",
				Group: "cert-manager.io",
			},
		},
	}

	// Создаем Certificate
	if err := r.Create(ctx, tempCertificate); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create temporary self-signed certificate: %w", err)
		}
		logger.V(1).Info("Temporary self-signed certificate already exists",
			"certificateName", tempCertName,
			"namespace", cert.Namespace,
		)
	}

	logger.Info("Created temporary self-signed certificate for Gateway",
		"certificateName", tempCertName,
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"tempSecretName", tempSecretName,
	)

	// Создаем EnvoyFilter СРАЗУ при создании временного сертификата, ДО того как он станет готовым
	// Это предотвращает кеширование HSTS заголовка браузером при первом обращении
	// EnvoyFilter будет активен с момента создания, даже если временный сертификат еще не готов
	if err := r.createEnvoyFilterToDisableHSTS(ctx, gateway, cert.Spec.SecretName); err != nil {
		logger.Error(err, "failed to create EnvoyFilter to disable HSTS (will retry later)",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		// Не возвращаем ошибку, так как EnvoyFilter будет создан при следующей реконсиляции
	}

	// Получаем созданный сертификат из кластера для проверки готовности
	createdTempCert := &certmanagerv1.Certificate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      tempCertName,
		Namespace: cert.Namespace,
	}, createdTempCert); err == nil {
		// Проверяем готовность временного сертификата и обновляем Gateway, если готов
		// Если не готов, обновление произойдет при следующей реконсиляции
		if r.isCertificateReady(createdTempCert) {
			if err := r.updateGatewayWithTemporarySecret(ctx, gateway, cert, cert.Spec.SecretName, tempSecretName, cert.Namespace); err != nil {
				logger.Error(err, "failed to update Gateway with temporary secret",
					"gatewayName", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
				)
			}
		}
	}

	return nil
}

// deleteTemporarySelfSignedCertificate удаляет временный самоподписанный сертификат и issuer
func (r *CertificateReconciler) deleteTemporarySelfSignedCertificate(ctx context.Context, cert *certmanagerv1.Certificate) error {
	logger := log.FromContext(ctx)

	tempCertName := fmt.Sprintf("%s-temp-selfsigned", cert.Name)
	tempCert := &certmanagerv1.Certificate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      tempCertName,
		Namespace: cert.Namespace,
	}, tempCert); err != nil {
		// Временный сертификат не найден, возможно уже удален
		return nil
	}

	// Проверяем, что это временный сертификат, созданный нами
	if tempCert.Labels == nil || tempCert.Labels["istio-http01.rieset.io/temp"] != tempLabelValue {
		return nil
	}

	// Удаляем временный Certificate
	if err := r.Delete(ctx, tempCert); err != nil {
		logger.Error(err, "failed to delete temporary certificate",
			"certificateName", tempCertName,
		)
		return fmt.Errorf("failed to delete temporary certificate: %w", err)
	}
	logger.Info("Deleted temporary self-signed certificate",
		"certificateName", tempCertName,
	)

	// Удаляем временный Issuer
	issuerName := fmt.Sprintf("%s-temp-selfsigned-issuer", cert.Name)
	issuer := &certmanagerv1.Issuer{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      issuerName,
		Namespace: cert.Namespace,
	}, issuer); err == nil {
		// Проверяем, что это временный issuer, созданный нами
		if issuer.Labels != nil && issuer.Labels["istio-http01.rieset.io/temp"] == tempLabelValue {
			if err := r.Delete(ctx, issuer); err != nil {
				logger.Error(err, "failed to delete temporary issuer",
					"issuerName", issuerName,
				)
			} else {
				logger.Info("Deleted temporary self-signed issuer",
					"issuerName", issuerName,
				)
			}
		}
	}

	return nil
}

// ensureTemporaryCertificateSetup проверяет и восстанавливает состояние временного сертификата, httpRedirect и EnvoyFilter
func (r *CertificateReconciler) ensureTemporaryCertificateSetup(ctx context.Context, cert *certmanagerv1.Certificate, gateway *istionetworkingv1beta1.Gateway) error {
	logger := log.FromContext(ctx)

	tempCertName := fmt.Sprintf("%s-temp-selfsigned", cert.Name)
	tempSecretName := fmt.Sprintf("%s-temp", cert.Spec.SecretName)
	envoyFilterName := fmt.Sprintf("disable-hsts-%s-%s", gateway.Namespace, gateway.Name)

	// 1. Проверяем наличие временного сертификата
	tempCert := &certmanagerv1.Certificate{}
	tempCertExists := r.Get(ctx, client.ObjectKey{
		Name:      tempCertName,
		Namespace: cert.Namespace,
	}, tempCert) == nil

	if !tempCertExists {
		// Временный сертификат не существует - создаем его
		logger.Info("Temporary certificate not found, creating it",
			"certificateName", cert.Name,
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		if err := r.createSelfSignedCertificate(ctx, cert, gateway); err != nil {
			return fmt.Errorf("failed to create temporary certificate: %w", err)
		}
		// После создания сертификата нужно подождать его готовности
		return nil
	}

	// 2. Проверяем готовность временного сертификата
	if !r.isCertificateReady(tempCert) {
		logger.V(1).Info("Temporary certificate not ready yet, waiting",
			"certificateName", cert.Name,
			"tempCertName", tempCertName,
		)
		return nil
	}

	// 3. Проверяем, использует ли Gateway временный секрет
	usesTempSecret := r.isGatewayUsingSecret(ctx, gateway, tempSecretName, cert.Namespace)

	// 4. Проверяем, отключен ли httpRedirect
	httpsRedirectDisabled := false
	gatewayKey := client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
	updatedGateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, gatewayKey, updatedGateway); err == nil {
		for _, server := range updatedGateway.Spec.Servers {
			if server.Port != nil && server.Port.Number == 80 && server.Tls != nil {
				if !server.Tls.HttpsRedirect {
					httpsRedirectDisabled = true
					break
				}
			}
		}
	}

	// 5. Проверяем наличие EnvoyFilter
	envoyFilterExists := false
	envoyFilter := &istionetworkingv1alpha3.EnvoyFilter{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      envoyFilterName,
		Namespace: gateway.Namespace,
	}, envoyFilter); err == nil {
		if envoyFilter.Labels != nil && envoyFilter.Labels["istio-http01.rieset.io/temp"] == tempLabelValue {
			envoyFilterExists = true
		}
	}

	// Логируем текущее состояние
	logger.V(1).Info("Checking temporary certificate setup state",
		"certificateName", cert.Name,
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"tempCertExists", tempCertExists,
		"tempCertReady", r.isCertificateReady(tempCert),
		"usesTempSecret", usesTempSecret,
		"httpsRedirectDisabled", httpsRedirectDisabled,
		"envoyFilterExists", envoyFilterExists,
	)

	// Восстанавливаем недостающие компоненты
	needsUpdate := false

	// Если временный сертификат готов, но Gateway не использует временный секрет
	if !usesTempSecret {
		logger.Info("Temporary certificate ready but Gateway not using it, updating Gateway",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		if err := r.updateGatewayWithTemporarySecret(ctx, gateway, cert, cert.Spec.SecretName, tempSecretName, cert.Namespace); err != nil {
			return fmt.Errorf("failed to update Gateway with temporary secret: %w", err)
		}
		needsUpdate = true
	}

	// Если httpRedirect не отключен
	if !httpsRedirectDisabled {
		logger.Info("httpsRedirect not disabled, disabling it",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		if err := r.disableHTTPSRedirectForHTTP01(ctx, gateway, cert.Spec.SecretName, cert.Namespace); err != nil {
			return fmt.Errorf("failed to disable httpsRedirect: %w", err)
		}
		needsUpdate = true
	}

	// Если EnvoyFilter не существует
	if !envoyFilterExists {
		logger.Info("EnvoyFilter not found, creating it",
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		if err := r.createEnvoyFilterToDisableHSTS(ctx, gateway, cert.Spec.SecretName); err != nil {
			return fmt.Errorf("failed to create EnvoyFilter: %w", err)
		}
		needsUpdate = true
	}

	if needsUpdate {
		logger.Info("Restored missing components for temporary certificate setup",
			"certificateName", cert.Name,
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
	}

	return nil
}
