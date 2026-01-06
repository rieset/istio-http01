/*
 * Функции, определенные в этом файле:
 *
 * - (r *GatewayReconciler) getVirtualServicesForGateway(ctx, gateway) ([]istionetworkingv1beta1.VirtualService, error)
 *   Получает все VirtualService, связанные с Gateway, исключая созданные оператором istio-http01
 *
 * - (r *GatewayReconciler) getDomainsForGateway(ctx, gateway) ([]string, error)
 *   Получает список доменов, за которые отвечает Gateway на основе связанных VirtualService
 */

package controller

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// getVirtualServicesForGateway получает все VirtualService, связанные с Gateway
func (r *GatewayReconciler) getVirtualServicesForGateway(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) ([]*istionetworkingv1beta1.VirtualService, error) {
	// Получение всех VirtualService во всех namespace
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace("")); err != nil {
		return nil, err
	}

	var matchingVS []*istionetworkingv1beta1.VirtualService
	gatewayName := gateway.Name
	gatewayNamespace := gateway.Namespace

	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService

		// Исключаем VirtualService, созданные оператором istio-http01
		if vsItem.Labels != nil {
			if vsItem.Labels["app.kubernetes.io/managed-by"] == "istio-http01" ||
				vsItem.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue {
				continue
			}
		}
		// Также исключаем VirtualService по имени (если содержат http01-solver или acme-solver)
		if strings.Contains(vsItem.Name, "http01-solver") || strings.Contains(vsItem.Name, "acme-solver") {
			continue
		}

		// Проверка, ссылается ли VirtualService на этот Gateway
		for _, gw := range vsItem.Spec.Gateways {
			// Gateway может быть указан как "namespace/gateway" или просто "gateway"
			if gw == gatewayName || gw == gatewayNamespace+"/"+gatewayName {
				// Добавляем указатель (не копируем структуру с мьютексом)
				matchingVS = append(matchingVS, vsItem)
				// Убрали избыточное логирование - информация о VirtualService будет в основном логе
				break
			}
		}
	}

	return matchingVS, nil
}

// getDomainsForGateway получает список доменов, за которые отвечает Gateway на основе связанных VirtualService
func (r *GatewayReconciler) getDomainsForGateway(ctx context.Context, gateway *istionetworkingv1beta1.Gateway) ([]string, error) {
	virtualServices, err := r.getVirtualServicesForGateway(ctx, gateway)
	if err != nil {
		return nil, err
	}

	// Используем map для исключения дубликатов доменов
	domainMap := make(map[string]bool)
	for _, vs := range virtualServices {
		for _, host := range vs.Spec.Hosts {
			domainMap[host] = true
		}
	}

	// Преобразуем map в slice
	domains := make([]string, 0, len(domainMap))
	for domain := range domainMap {
		domains = append(domains, domain)
	}

	return domains, nil
}
