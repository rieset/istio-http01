/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) isVirtualServiceValid(ctx, vs) bool
 *   Проверяет актуальность VirtualService - существуют ли связанные под и сервис
 */

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// isVirtualServiceValid проверяет актуальность VirtualService - существуют ли связанные под и сервис
func (r *HTTP01SolverPodReconciler) isVirtualServiceValid(ctx context.Context, vs *istionetworkingv1beta1.VirtualService) bool {
	logger := log.FromContext(ctx)

	if vs.Labels == nil {
		return false
	}

	podName := vs.Labels["acme.cert-manager.io/solver-pod"]
	serviceName := vs.Labels["acme.cert-manager.io/solver-service"]
	if podName == "" || serviceName == "" {
		return false
	}

	// Определяем namespace пода из destination в VirtualService
	podNamespace := ""
	if len(vs.Spec.Http) > 0 && len(vs.Spec.Http[0].Route) > 0 {
		destination := vs.Spec.Http[0].Route[0].Destination
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

	// VirtualService валиден, если под и сервис существуют
	isValid := podExists && serviceExists

	if !isValid {
		logger.V(1).Info("VirtualService is not valid",
			"virtualService", vs.Name,
			"virtualServiceNamespace", vs.Namespace,
			"pod", podName,
			"podNamespace", podNamespace,
			"service", serviceName,
			"podExists", podExists,
			"serviceExists", serviceExists,
		)
	}

	return isValid
}
