/*
 * Функции, определенные в этом файле:
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
 */

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// getDomainsForGateway получает список доменов для Gateway из связанных VirtualService
func (r *CertificateReconciler) getDomainsForGateway(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) ([]string, error) {
	logger := log.FromContext(ctx)

	// Получение всех VirtualService во всех namespace
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace("")); err != nil {
		return nil, fmt.Errorf("failed to list VirtualServices: %w", err)
	}

	gatewayName := gateway.Name
	gatewayNamespace := gateway.Namespace
	gatewayRef := gatewayName
	if gatewayNamespace != "" {
		gatewayRef = gatewayNamespace + "/" + gatewayName
	}

	// Используем map для исключения дубликатов доменов
	domainMap := make(map[string]bool)

	// Проверка всех VirtualService, связанных с этим Gateway
	for i := range virtualServiceList.Items {
		vs := virtualServiceList.Items[i]

		// Исключаем VirtualService, созданные оператором istio-http01
		isOperatorManaged := false
		if vs.Labels != nil {
			if vs.Labels["app.kubernetes.io/managed-by"] == "istio-http01" ||
				vs.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue {
				isOperatorManaged = true
			}
		}
		// Также исключаем VirtualService по имени (если содержат http01-solver или acme-solver)
		isSolverVS := strings.Contains(vs.Name, "http01-solver") || strings.Contains(vs.Name, "acme-solver")

		if isOperatorManaged || isSolverVS {
			continue
		}

		// Проверка, ссылается ли VirtualService на этот Gateway
		linkedToGateway := false
		for _, gw := range vs.Spec.Gateways {
			if gw == gatewayName || gw == gatewayRef {
				linkedToGateway = true
				break
			}
		}

		if linkedToGateway {
			// Добавляем все домены из VirtualService
			for _, host := range vs.Spec.Hosts {
				domainMap[host] = true
			}
		}
	}

	// Преобразуем map в slice
	domains := make([]string, 0, len(domainMap))
	for domain := range domainMap {
		domains = append(domains, domain)
	}

	logger.V(1).Info("Got domains for Gateway",
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"domainCount", len(domains),
	)

	return domains, nil
}

// getIngressGatewayIP получает IP адрес ingress gateway для Gateway
func (r *CertificateReconciler) getIngressGatewayIP(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) (string, error) {
	logger := log.FromContext(ctx)

	// Получаем селектор из Gateway
	selector := gateway.Spec.Selector
	if len(selector) == 0 {
		// Если селектор не указан, используем стандартный istio ingressgateway
		selector = map[string]string{
			"istio": "ingressgateway",
		}
	}

	// Ищем Service с соответствующим селектором
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace("")); err != nil {
		return "", fmt.Errorf("failed to list Services: %w", err)
	}

	// Ищем Service, который соответствует селектору Gateway
	for i := range serviceList.Items {
		svc := serviceList.Items[i]

		// Проверяем, соответствует ли селектор Service селектору Gateway
		matches := true
		for key, value := range selector {
			if svc.Spec.Selector == nil {
				matches = false
				break
			}
			if svc.Spec.Selector[key] != value {
				matches = false
				break
			}
		}

		if matches {
			// Получаем IP адрес из Service
			// Сначала проверяем LoadBalancer IP
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				if len(svc.Status.LoadBalancer.Ingress) > 0 {
					ingress := svc.Status.LoadBalancer.Ingress[0]
					if ingress.IP != "" {
						logger.V(1).Info("Found ingress gateway IP from LoadBalancer",
							"serviceName", svc.Name,
							"serviceNamespace", svc.Namespace,
							"ip", ingress.IP,
						)
						return ingress.IP, nil
					}
					if ingress.Hostname != "" {
						// Если есть hostname, резолвим его в IP
						ips, err := net.LookupIP(ingress.Hostname)
						if err == nil && len(ips) > 0 {
							logger.V(1).Info("Found ingress gateway IP from LoadBalancer hostname",
								"serviceName", svc.Name,
								"serviceNamespace", svc.Namespace,
								"hostname", ingress.Hostname,
								"ip", ips[0].String(),
							)
							return ips[0].String(), nil
						}
					}
				}
			}

			// Если LoadBalancer не найден, проверяем ExternalIPs
			if len(svc.Spec.ExternalIPs) > 0 {
				logger.V(1).Info("Found ingress gateway IP from ExternalIPs",
					"serviceName", svc.Name,
					"serviceNamespace", svc.Namespace,
					"ip", svc.Spec.ExternalIPs[0],
				)
				return svc.Spec.ExternalIPs[0], nil
			}
		}
	}

	return "", fmt.Errorf("ingress gateway IP not found for Gateway %s/%s", gateway.Namespace, gateway.Name)
}

// verifyCertificateViaHTTPS проверяет сертификат через HTTPS запрос и сравнивает домены
func (r *CertificateReconciler) verifyCertificateViaHTTPS(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, expectedDNSNames []string, ingressIP string) error {
	logger := log.FromContext(ctx)

	// Получаем домены для Gateway
	domains, err := r.getDomainsForGateway(ctx, gateway)
	if err != nil {
		return fmt.Errorf("failed to get domains for Gateway: %w", err)
	}

	if len(domains) == 0 {
		return fmt.Errorf("no domains found for Gateway %s/%s", gateway.Namespace, gateway.Name)
	}

	// Используем первый домен для проверки
	domain := domains[0]

	// Создаем HTTP клиент с проверкой сертификата
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false, // Проверяем сертификат
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						return fmt.Errorf("no certificate provided")
					}

					// Парсим первый сертификат
					cert, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						return fmt.Errorf("failed to parse certificate: %w", err)
					}

					// Получаем DNS имена из сертификата
					certDNSNames := cert.DNSNames
					if cert.Subject.CommonName != "" {
						certDNSNames = append(certDNSNames, cert.Subject.CommonName)
					}

					// Проверяем, что домены в сертификате соответствуют доменам Gateway
					domainFound := false
					for _, certDNSName := range certDNSNames {
						for _, gatewayDomain := range domains {
							if certDNSName == gatewayDomain || strings.HasSuffix(gatewayDomain, "."+certDNSName) {
								domainFound = true
								break
							}
						}
						if domainFound {
							break
						}
					}

					if !domainFound {
						return fmt.Errorf("certificate DNS names (%v) do not match Gateway domains (%v)", certDNSNames, domains)
					}

					// Проверяем, что домены в сертификате соответствуют ожидаемым DNS именам из Certificate
					expectedDNSNamesMatch := false
					for _, certDNSName := range certDNSNames {
						for _, expectedDNSName := range expectedDNSNames {
							if certDNSName == expectedDNSName {
								expectedDNSNamesMatch = true
								break
							}
						}
						if expectedDNSNamesMatch {
							break
						}
					}

					if !expectedDNSNamesMatch {
						return fmt.Errorf("certificate DNS names (%v) do not match expected Certificate DNS names (%v)", certDNSNames, expectedDNSNames)
					}

					logger.Info("Certificate verified via HTTPS",
						"gatewayName", gateway.Name,
						"gatewayNamespace", gateway.Namespace,
						"domain", domain,
						"ingressIP", ingressIP,
						"certDNSNames", certDNSNames,
						"gatewayDomains", domains,
					)

					return nil
				},
			},
		},
	}

	// Выполняем HTTPS запрос
	url := fmt.Sprintf("https://%s", ingressIP)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Host = domain

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTPS request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error(closeErr, "failed to close response body")
		}
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTPS request failed with status code %d", resp.StatusCode)
	}

	logger.Info("HTTPS certificate verification successful",
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"domain", domain,
		"ingressIP", ingressIP,
		"statusCode", resp.StatusCode,
	)

	return nil
}

// verifyCertificateViaHTTP проверяет доступность через HTTP запрос
func (r *CertificateReconciler) verifyCertificateViaHTTP(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, ingressIP string) error {
	logger := log.FromContext(ctx)

	// Получаем домены для Gateway
	domains, err := r.getDomainsForGateway(ctx, gateway)
	if err != nil {
		return fmt.Errorf("failed to get domains for Gateway: %w", err)
	}

	if len(domains) == 0 {
		return fmt.Errorf("no domains found for Gateway %s/%s", gateway.Namespace, gateway.Name)
	}

	// Используем первый домен для проверки
	domain := domains[0]

	// Создаем HTTP клиент
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Заменяем домен на IP адрес
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				if host == domain {
					addr = net.JoinHostPort(ingressIP, port)
				}
				dialer := &net.Dialer{
					Timeout: 5 * time.Second,
				}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}

	// Делаем HTTP запрос к домену через IP адрес
	url := fmt.Sprintf("http://%s", domain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Устанавливаем Host заголовок
	req.Host = domain

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error(closeErr, "failed to close response body")
		}
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
	}

	logger.Info("HTTP certificate verification successful",
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
		"domain", domain,
		"ingressIP", ingressIP,
		"statusCode", resp.StatusCode,
	)

	return nil
}
