/*
 * Функции, определенные в этом файле:
 *
 * - (r *GatewayReconciler) getCertificatesForGateway(ctx, gateway) ([]GatewayCertificate, error)
 *   Получает список сертификатов, используемых в Gateway
 *
 * - (r *GatewayReconciler) getAllGatewaysWithCertificates(ctx) (map[string]GatewayInfo, error)
 *   Получает все Gateway с их доменами и сертификатами
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// GatewayCertificate содержит информацию о сертификате, используемом в Gateway
type GatewayCertificate struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	DNSNames  []string `json:"dnsNames"`
	Ready     bool     `json:"ready"`
}

// GatewayInfo содержит информацию о Gateway, его доменах и сертификатах
type GatewayInfo struct {
	Domains      []string             `json:"domains"`
	Certificates []GatewayCertificate `json:"certificates"`
}

// getCertificatesForGateway получает список сертификатов, используемых в Gateway
func (r *GatewayReconciler) getCertificatesForGateway(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) ([]GatewayCertificate, error) {
	var certificates []GatewayCertificate
	seenCertificates := make(map[string]bool) // Для исключения дубликатов

	// Получение всех Certificate во всех namespace
	certList := &certmanagerv1.CertificateList{}
	if err := r.List(ctx, certList, client.InNamespace("")); err != nil {
		return nil, fmt.Errorf("failed to list Certificates: %w", err)
	}

	// Собираем все credentialName из Gateway
	credentialNames := make(map[string]string) // credentialName -> namespace
	for _, server := range gateway.Spec.Servers {
		if server.Tls == nil {
			continue
		}
		credentialName := server.Tls.CredentialName
		if credentialName == "" {
			continue
		}

		// Определяем namespace для credentialName
		var secretNamespace string
		if strings.Contains(credentialName, "/") {
			// Формат "namespace/name"
			parts := strings.Split(credentialName, "/")
			if len(parts) == 2 {
				secretNamespace = parts[0]
				credentialName = parts[1]
			}
		} else {
			// Формат "name" - секрет в том же namespace, что и Gateway
			secretNamespace = gateway.Namespace
		}

		credentialNames[credentialName] = secretNamespace
	}

	// Ищем Certificate, которые создают секреты с этими именами
	for i := range certList.Items {
		cert := certList.Items[i]
		secretName := cert.Spec.SecretName
		if secretName == "" {
			continue
		}

		// Проверяем, совпадает ли secretName с одним из credentialName
		for credName, credNamespace := range credentialNames {
			if secretName == credName {
				// Проверяем namespace
				if cert.Namespace == credNamespace || credNamespace == "" {
					// Проверяем, не добавляли ли мы уже этот сертификат
					certKey := fmt.Sprintf("%s/%s", cert.Namespace, cert.Name)
					if !seenCertificates[certKey] {
						// Проверяем готовность сертификата
						isReady := false
						for _, condition := range cert.Status.Conditions {
							if condition.Type == certmanagerv1.CertificateConditionReady {
								isReady = condition.Status == certmanagermetav1.ConditionTrue
								break
							}
						}

						certificates = append(certificates, GatewayCertificate{
							Name:      cert.Name,
							Namespace: cert.Namespace,
							DNSNames:  cert.Spec.DNSNames,
							Ready:     isReady,
						})
						seenCertificates[certKey] = true
					}
					break
				}
			}
		}
	}

	return certificates, nil
}

// getAllGatewaysWithCertificates получает все Gateway с их доменами и сертификатами
func (r *GatewayReconciler) getAllGatewaysWithCertificates(ctx context.Context) (map[string]GatewayInfo, error) {
	// Получение всех Gateway во всех namespace
	gatewayList := &istionetworkingv1beta1.GatewayList{}
	if err := r.List(ctx, gatewayList, client.InNamespace("")); err != nil {
		return nil, err
	}

	gatewayInfoMap := make(map[string]GatewayInfo)

	// Для каждого Gateway получаем домены и сертификаты
	for i := range gatewayList.Items {
		gateway := gatewayList.Items[i]

		// Получаем домены
		domains, err := r.getDomainsForGateway(ctx, gateway)
		if err != nil {
			continue // Пропускаем Gateway с ошибками
		}

		// Получаем сертификаты
		certificates, err := r.getCertificatesForGateway(ctx, gateway)
		if err != nil {
			// Логируем ошибку, но продолжаем обработку
			log.FromContext(ctx).Error(err, "failed to get certificates for Gateway",
				"gateway", gateway.Name,
				"gatewayNamespace", gateway.Namespace,
			)
			certificates = []GatewayCertificate{} // Используем пустой список при ошибке
		}

		// Используем формат "namespace/name" как ключ
		gatewayKey := fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name)
		gatewayInfoMap[gatewayKey] = GatewayInfo{
			Domains:      domains,
			Certificates: certificates,
		}
	}

	return gatewayInfoMap, nil
}
