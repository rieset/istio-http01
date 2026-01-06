/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) getDomainsForGateway(ctx, gateway) ([]string, error)
 *   Получает список доменов, за которые отвечает Gateway на основе связанных VirtualService, исключая созданные оператором istio-http01
 *
 * - (r *HTTP01SolverPodReconciler) findGatewayForDomain(ctx, domain) (*Gateway, error)
 *   Находит Gateway, который резолвит указанный домен через домены, закрепленные за Gateway через VirtualService.
 *   НЕ использует поле hosts в Gateway, так как оно содержит данные для внутренней сети, а не внешние домены.
 */

package controller

import (
	"context"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// getDomainsForGateway получает список доменов, за которые отвечает Gateway на основе связанных VirtualService
// Исключает VirtualService, созданные оператором istio-http01
func (r *HTTP01SolverPodReconciler) getDomainsForGateway(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) ([]string, error) {
	// Получение всех VirtualService во всех namespace
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace("")); err != nil {
		return nil, err
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
				vs.Labels["acme.cert-manager.io/http01-solver"] == "true" {
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

	return domains, nil
}

// findGatewayForDomain находит Gateway, который резолвит указанный домен
// Определяет Gateway только через домены, закрепленные за Gateway через VirtualService
// НЕ использует поле hosts в Gateway, так как оно содержит данные для внутренней сети, а не внешние домены
func (r *HTTP01SolverPodReconciler) findGatewayForDomain(ctx context.Context, domain string) (*istionetworkingv1beta1.Gateway, error) {
	// Получение всех Gateway во всех namespace
	gatewayList := &istionetworkingv1beta1.GatewayList{}
	if err := r.List(ctx, gatewayList, client.InNamespace("")); err != nil {
		return nil, err
	}

	ctrl.Log.Info("Determining Gateway for domain",
		"domain", domain,
		"gatewayCount", len(gatewayList.Items),
	)

	// Проверяем домены, закрепленные за Gateway через VirtualService
	for i := range gatewayList.Items {
		gateway := gatewayList.Items[i]

		// Получаем домены, закрепленные за этим Gateway
		domains, err := r.getDomainsForGateway(ctx, gateway)
		if err != nil {
			continue
		}

		ctrl.Log.Info("Checking Gateway for domain match",
			"domain", domain,
			"gateway", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"gatewayDomains", domains,
		)

		// Проверяем, есть ли искомый домен в списке доменов Gateway
		// Используем только точное совпадение, чтобы избежать неправильного сопоставления
		// (например, app-gamma.example.com не должен совпадать с app-alpha.example.com)
		for _, gatewayDomain := range domains {
			// Проверка точного совпадения
			if gatewayDomain == domain {
				ctrl.Log.Info("Gateway found via VirtualService domain match",
					"domain", domain,
					"gateway", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
					"matchedDomain", gatewayDomain,
					"method", "virtualservice_domain_match",
				)
				return gateway, nil
			}
			// Проверка wildcard (только если gatewayDomain == "*")
			if gatewayDomain == "*" {
				ctrl.Log.Info("Gateway found via VirtualService wildcard",
					"domain", domain,
					"gateway", gateway.Name,
					"gatewayNamespace", gateway.Namespace,
					"method", "virtualservice_wildcard",
				)
				return gateway, nil
			}
		}
	}

	ctrl.Log.Info("No Gateway found for domain",
		"domain", domain,
	)
	return nil, nil
}
