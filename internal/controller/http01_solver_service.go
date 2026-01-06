/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) findServiceForPod(ctx, pod) (*Service, error)
 *   Находит Service для HTTP01 solver пода по имени или по селектору (http-domain и http-token метки)
 */

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// findServiceForPod находит Service для HTTP01 solver пода
// Сначала пытается найти по имени пода, затем по меткам и селектору
func (r *HTTP01SolverPodReconciler) findServiceForPod(ctx context.Context, pod *corev1.Pod) (*corev1.Service, error) {
	logger := log.FromContext(ctx)

	// Поиск Service для этого пода (cert-manager создает Service для HTTP01 solver)
	// Service может иметь другое имя, поэтому ищем по меткам и ownerReferences
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(pod.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var service *corev1.Service
	// Сначала пытаемся найти Service по имени пода (если совпадает)
	serviceName := pod.Name
	serviceKey := client.ObjectKey{
		Name:      serviceName,
		Namespace: pod.Namespace,
	}
	serviceCandidate := &corev1.Service{}
	if err := r.Get(ctx, serviceKey, serviceCandidate); err == nil {
		service = serviceCandidate
		logger.Info("Found Service by pod name",
			"serviceName", service.Name,
			"pod", pod.Name,
		)
		return service, nil
	}

	// Если не нашли по имени, ищем по меткам и селектору
	// cert-manager создает Service с меткой acme.cert-manager.io/http01-solver: "true"
	// и селектором по меткам http-domain и http-token пода
	for i := range serviceList.Items {
		svc := &serviceList.Items[i]
		// Проверка метки
		if svc.Labels["acme.cert-manager.io/http01-solver"] == "true" {
			// Проверка селектора - должен соответствовать меткам пода
			if svc.Spec.Selector != nil {
				podDomain := pod.Labels["acme.cert-manager.io/http-domain"]
				podToken := pod.Labels["acme.cert-manager.io/http-token"]

				svcDomain := svc.Spec.Selector["acme.cert-manager.io/http-domain"]
				svcToken := svc.Spec.Selector["acme.cert-manager.io/http-token"]

				// Service должен иметь селектор по http-domain и http-token
				// и они должны совпадать с метками пода
				if podDomain != "" && podToken != "" &&
					svcDomain == podDomain && svcToken == podToken {
					service = svc
					logger.Info("Found Service by selector match",
						"serviceName", service.Name,
						"pod", pod.Name,
						"httpDomain", podDomain,
						"httpToken", podToken,
					)
					return service, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("service not found for solver pod %s: cert-manager may not have created the service yet", pod.Name)
}
