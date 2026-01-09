/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) findVirtualServiceForDomain(ctx, gateway, domain) (*VirtualService, error)
 *   Проверяет наличие VirtualService для Gateway и домена
 *
 * - (r *HTTP01SolverPodReconciler) findVirtualServicesForPod(ctx, podName, podNamespace) ([]*VirtualService, error)
 *   Находит все VirtualService, связанные с указанным подом
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// findVirtualServiceForDomain проверяет наличие VirtualService для Gateway и домена
func (r *HTTP01SolverPodReconciler) findVirtualServiceForDomain(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, domain string) (*istionetworkingv1beta1.VirtualService, error) {
	// Сначала ищем в namespace Gateway (где создаются VirtualService)
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace(gateway.Namespace)); err != nil {
		return nil, err
	}

	// Поиск VirtualService в namespace Gateway
	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService

		// Проверка домена в hosts
		hasDomain := false
		for _, host := range vsItem.Spec.Hosts {
			if host == domain {
				hasDomain = true
				break
			}
		}

		if !hasDomain {
			continue
		}

		// Проверка, что это VirtualService для HTTP01 solver (по меткам или имени)
		if strings.Contains(vsItem.Name, "http01-solver") || strings.Contains(vsItem.Name, "acme-solver") ||
			(vsItem.Labels != nil && vsItem.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue) {
			// VirtualService найден - возвращаем без логирования (будет залогировано в Reconcile)
			return vsItem, nil
		}
	}

	// Если не найдено в namespace Gateway, ищем во всех namespace (для обратной совместимости)
	virtualServiceList = &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace("")); err != nil {
		return nil, err
	}

	// Поиск VirtualService во всех namespace
	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService

		// Пропускаем VirtualService из namespace Gateway (уже проверили)
		if vsItem.Namespace == gateway.Namespace {
			continue
		}

		// Проверка домена в hosts
		hasDomain := false
		for _, host := range vsItem.Spec.Hosts {
			if host == domain {
				hasDomain = true
				break
			}
		}

		if !hasDomain {
			continue
		}

		// Проверка, что это VirtualService для HTTP01 solver (по меткам или имени)
		if strings.Contains(vsItem.Name, "http01-solver") || strings.Contains(vsItem.Name, "acme-solver") ||
			(vsItem.Labels != nil && vsItem.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue) {
			// VirtualService найден - возвращаем без логирования (будет залогировано в Reconcile)
			return vsItem, nil
		}
	}

	return nil, nil
}

// findVirtualServicesForPod находит все VirtualService, связанные с указанным подом
func (r *HTTP01SolverPodReconciler) findVirtualServicesForPod(ctx context.Context, podName, podNamespace string) ([]*istionetworkingv1beta1.VirtualService, error) {
	logger := log.FromContext(ctx)

	// Поиск всех VirtualService во всех namespace с меткой solver-pod
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.MatchingLabels{
		"acme.cert-manager.io/solver-pod": podName,
		"app.kubernetes.io/managed-by":    "istio-http01",
	}); err != nil {
		return nil, fmt.Errorf("failed to list VirtualServices: %w", err)
	}

	var matchingVS []*istionetworkingv1beta1.VirtualService
	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService
		// Проверяем, что это VirtualService для HTTP01 solver
		if vsItem.Labels != nil && vsItem.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue {
			// Проверяем, что метка solver-pod совпадает
			if vsItem.Labels["acme.cert-manager.io/solver-pod"] == podName {
				matchingVS = append(matchingVS, vsItem)
			}
		}
	}

	logger.V(1).Info("Found VirtualServices for pod",
		"pod", podName,
		"podNamespace", podNamespace,
		"count", len(matchingVS),
	)

	return matchingVS, nil
}
