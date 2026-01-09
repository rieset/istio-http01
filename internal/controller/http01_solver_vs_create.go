/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) createVirtualServiceForSolver(ctx, pod, gateway, domain) error
 *   Создает VirtualService для доступа к поду HTTP01 solver через Gateway
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istioapinetworkingv1beta1 "istio.io/api/networking/v1beta1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// createVirtualServiceForSolver создает VirtualService для доступа к поду HTTP01 solver через Gateway
func (r *HTTP01SolverPodReconciler) createVirtualServiceForSolver(ctx context.Context, pod *corev1.Pod, gateway *istionetworkingv1beta1.Gateway, domain string) error {
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

	// Service найден, продолжаем создание VirtualService

	// Имя VirtualService на основе домена и пода
	vsName := fmt.Sprintf("http01-solver-%s", strings.ReplaceAll(strings.ReplaceAll(domain, ".", "-"), "*", "wildcard"))
	if len(vsName) > 63 {
		// Kubernetes имена ограничены 63 символами
		vsName = vsName[:63]
	}

	// Gateway reference
	gatewayRef := gateway.Name
	if gateway.Namespace != pod.Namespace {
		gatewayRef = fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name)
	}

	// Создание VirtualService
	virtualService := &istionetworkingv1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vsName,
			Namespace: gateway.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":        "istio-http01",
				"acme.cert-manager.io/http01-solver":  http01SolverLabelValue,
				"acme.cert-manager.io/solver-pod":     pod.Name,
				"acme.cert-manager.io/solver-service": service.Name,
			},
		},
		Spec: istioapinetworkingv1beta1.VirtualService{
			Hosts:    []string{domain},
			Gateways: []string{gatewayRef},
			Http: []*istioapinetworkingv1beta1.HTTPRoute{
				{
					Match: []*istioapinetworkingv1beta1.HTTPMatchRequest{
						{
							Uri: &istioapinetworkingv1beta1.StringMatch{
								MatchType: &istioapinetworkingv1beta1.StringMatch_Prefix{
									Prefix: "/.well-known/acme-challenge/",
								},
							},
						},
					},
					Route: []*istioapinetworkingv1beta1.HTTPRouteDestination{
						{
							Destination: &istioapinetworkingv1beta1.Destination{
								Host: fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace),
								Port: &istioapinetworkingv1beta1.PortSelector{
									Number: solverPort,
								},
							},
						},
					},
				},
			},
		},
	}

	// Установка owner reference только если под и VirtualService в одном namespace
	// Kubernetes не позволяет cross-namespace owner references
	if pod.Namespace == gateway.Namespace {
		if err := ctrl.SetControllerReference(pod, virtualService, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}

	// Создание VirtualService
	if err := r.Create(ctx, virtualService); err != nil {
		return fmt.Errorf("failed to create VirtualService: %w", err)
	}

	logger.Info("Created VirtualService for HTTP01 solver",
		"virtualService", virtualService.Name,
		"path", "/.well-known/acme-challenge/",
		"solverService", service.Name,
		"solverPort", solverPort,
	)

	return nil
}
