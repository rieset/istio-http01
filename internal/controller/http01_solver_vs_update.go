/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) updateVirtualServiceForSolver(ctx, pod, existingVS) error
 *   Обновляет существующий VirtualService для нового пода HTTP01 solver
 */

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istioapinetworkingv1beta1 "istio.io/api/networking/v1beta1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// updateVirtualServiceForSolver обновляет существующий VirtualService для нового пода HTTP01 solver
func (r *HTTP01SolverPodReconciler) updateVirtualServiceForSolver(ctx context.Context, pod *corev1.Pod, existingVS *istionetworkingv1beta1.VirtualService) error {
	logger := log.FromContext(ctx)

	// Поиск Service для этого пода
	service, err := r.findServiceForPod(ctx, pod)
	if err != nil {
		logger.Error(err, "Service not found for solver pod",
			"pod", pod.Name,
			"podNamespace", pod.Namespace,
		)
		return err
	}

	// Определение порта из Service
	solverPort := uint32(8089) // Порт по умолчанию
	if len(service.Spec.Ports) > 0 {
		solverPort = uint32(service.Spec.Ports[0].Port)
	}

	// Обновление меток VirtualService
	if existingVS.Labels == nil {
		existingVS.Labels = make(map[string]string)
	}
	existingVS.Labels["acme.cert-manager.io/solver-pod"] = pod.Name
	existingVS.Labels["acme.cert-manager.io/solver-service"] = service.Name

	// Обновление owner reference только если под и VirtualService в одном namespace
	// Kubernetes не позволяет cross-namespace owner references
	if pod.Namespace == existingVS.Namespace {
		existingVS.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       pod.Name,
				UID:        pod.UID,
				Controller: func() *bool { b := true; return &b }(),
			},
		}
	} else {
		// Если в разных namespace, удаляем owner reference если он был установлен
		existingVS.OwnerReferences = nil
	}

	// Обновление destination в HTTP route
	if len(existingVS.Spec.Http) > 0 && len(existingVS.Spec.Http[0].Route) > 0 {
		existingVS.Spec.Http[0].Route[0].Destination = &istioapinetworkingv1beta1.Destination{
			Host: fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace),
			Port: &istioapinetworkingv1beta1.PortSelector{
				Number: solverPort,
			},
		}
	}

	// Обновление VirtualService
	if err := r.Update(ctx, existingVS); err != nil {
		return fmt.Errorf("failed to update VirtualService: %w", err)
	}

	logger.Info("Updated VirtualService for HTTP01 solver",
		"virtualService", existingVS.Name,
		"path", "/.well-known/acme-challenge/",
		"solverService", service.Name,
		"solverPort", solverPort,
	)

	return nil
}
