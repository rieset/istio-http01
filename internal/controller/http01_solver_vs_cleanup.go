/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) deleteVirtualServicesForPod(ctx, podName, podNamespace) error
 *   Удаляет все VirtualService, связанные с указанным подом
 *
 * - (r *HTTP01SolverPodReconciler) cleanupOrphanedVirtualServices(ctx) error
 *   Удаляет VirtualService, которые ссылаются на несуществующие поды или сервисы
 *
 * - (r *HTTP01SolverPodReconciler) cleanupOrphanedVirtualServicesInNamespace(ctx, namespace) error
 *   Удаляет неактуальные VirtualService оператора в указанном namespace
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// deleteVirtualServicesForPod удаляет все VirtualService, связанные с указанным подом
func (r *HTTP01SolverPodReconciler) deleteVirtualServicesForPod(ctx context.Context, podName, podNamespace string) error {
	logger := log.FromContext(ctx)

	virtualServices, err := r.findVirtualServicesForPod(ctx, podName, podNamespace)
	if err != nil {
		return fmt.Errorf("failed to find VirtualServices for pod: %w", err)
	}

	if len(virtualServices) == 0 {
		logger.V(1).Info("No VirtualServices found for pod",
			"pod", podName,
			"podNamespace", podNamespace,
		)
		return nil
	}

	for _, vs := range virtualServices {
		if err := r.Delete(ctx, vs); err != nil {
			logger.Error(err, "failed to delete VirtualService",
				"virtualService", vs.Name,
				"virtualServiceNamespace", vs.Namespace,
				"pod", podName,
			)
			// Продолжаем удаление остальных, даже если один не удалился
			continue
		}

		logger.Info("Deleted VirtualService for removed pod",
			"virtualService", vs.Name,
			"virtualServiceNamespace", vs.Namespace,
			"pod", podName,
			"podNamespace", podNamespace,
		)
	}

	return nil
}

// cleanupOrphanedVirtualServicesInNamespace удаляет неактуальные VirtualService оператора в указанном namespace
func (r *HTTP01SolverPodReconciler) cleanupOrphanedVirtualServicesInNamespace(ctx context.Context, namespace string) error {
	logger := log.FromContext(ctx)

	// Поиск всех VirtualService оператора в указанном namespace
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.InNamespace(namespace), client.MatchingLabels{
		"app.kubernetes.io/managed-by": "istio-http01",
	}); err != nil {
		return fmt.Errorf("failed to list VirtualServices in namespace: %w", err)
	}

	var orphanedVS []*istionetworkingv1beta1.VirtualService

	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService
		// Проверяем только VirtualService для HTTP01 solver
		if vsItem.Labels == nil || vsItem.Labels["acme.cert-manager.io/http01-solver"] != http01SolverLabelValue {
			continue
		}

		// Проверяем актуальность VirtualService
		isValid := r.isVirtualServiceValid(ctx, vsItem)

		if !isValid {
			orphanedVS = append(orphanedVS, vsItem)
			logger.Info("Found orphaned VirtualService in namespace",
				"virtualService", vsItem.Name,
				"virtualServiceNamespace", vsItem.Namespace,
				"namespace", namespace,
			)
		}
	}

	// Удаляем orphaned VirtualService
	for _, vs := range orphanedVS {
		if err := r.Delete(ctx, vs); err != nil {
			logger.Error(err, "failed to delete orphaned VirtualService",
				"virtualService", vs.Name,
				"virtualServiceNamespace", vs.Namespace,
			)
			continue
		}

		logger.Info("Deleted orphaned VirtualService in namespace",
			"virtualService", vs.Name,
			"virtualServiceNamespace", vs.Namespace,
			"namespace", namespace,
		)
	}

	if len(orphanedVS) > 0 {
		logger.Info("Cleaned up orphaned VirtualServices in namespace",
			"namespace", namespace,
			"count", len(orphanedVS),
		)
	}

	return nil
}

// cleanupOrphanedVirtualServices удаляет VirtualService, которые ссылаются на несуществующие поды или сервисы
func (r *HTTP01SolverPodReconciler) cleanupOrphanedVirtualServices(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Поиск всех VirtualService, созданных оператором
	virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
	if err := r.List(ctx, virtualServiceList, client.MatchingLabels{
		"app.kubernetes.io/managed-by":       "istio-http01",
		"acme.cert-manager.io/http01-solver": http01SolverLabelValue,
	}); err != nil {
		return fmt.Errorf("failed to list VirtualServices: %w", err)
	}

	var orphanedVS []*istionetworkingv1beta1.VirtualService

	for i := range virtualServiceList.Items {
		vsItem := virtualServiceList.Items[i] // vsItem это *VirtualService
		if vsItem.Labels == nil {
			continue
		}

		podName := vsItem.Labels["acme.cert-manager.io/solver-pod"]
		serviceName := vsItem.Labels["acme.cert-manager.io/solver-service"]
		if podName == "" || serviceName == "" {
			continue
		}

		// Определяем namespace пода из destination в VirtualService
		podNamespace := ""
		if len(vsItem.Spec.Http) > 0 && len(vsItem.Spec.Http[0].Route) > 0 {
			destination := vsItem.Spec.Http[0].Route[0].Destination
			if destination != nil && destination.Host != "" {
				// Парсим host вида "service-name.namespace.svc.cluster.local"
				parts := strings.Split(destination.Host, ".")
				if len(parts) >= 2 {
					podNamespace = parts[1]
				}
			}
		}

		// Если не удалось определить namespace из destination, используем istio-system по умолчанию
		if podNamespace == "" {
			podNamespace = defaultCertManagerNamespace // По умолчанию для cert-manager
		}

		// Проверяем существование пода
		pod := &corev1.Pod{}
		podKey := client.ObjectKey{
			Name:      podName,
			Namespace: podNamespace,
		}
		podExists := r.Get(ctx, podKey, pod) == nil

		// Проверяем существование сервиса
		service := &corev1.Service{}
		serviceKey := client.ObjectKey{
			Name:      serviceName,
			Namespace: podNamespace,
		}
		serviceExists := r.Get(ctx, serviceKey, service) == nil

		// Если под и сервис не существуют, VirtualService считается orphaned
		if !podExists && !serviceExists {
			orphanedVS = append(orphanedVS, vsItem)
			logger.Info("Found orphaned VirtualService",
				"virtualService", vsItem.Name,
				"virtualServiceNamespace", vsItem.Namespace,
				"pod", podName,
				"podNamespace", podNamespace,
				"service", serviceName,
			)
		}
	}

	// Удаляем orphaned VirtualService
	for _, vs := range orphanedVS {
		if err := r.Delete(ctx, vs); err != nil {
			logger.Error(err, "failed to delete orphaned VirtualService",
				"virtualService", vs.Name,
				"virtualServiceNamespace", vs.Namespace,
			)
			continue
		}

		logger.Info("Deleted orphaned VirtualService",
			"virtualService", vs.Name,
			"virtualServiceNamespace", vs.Namespace,
		)
	}

	if len(orphanedVS) > 0 {
		logger.Info("Cleaned up orphaned VirtualServices",
			"count", len(orphanedVS),
		)
	}

	return nil
}
