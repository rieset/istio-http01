/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) findVirtualServiceForDomain(ctx, gateway, domain, pod) (*VirtualService, error)
 *   Проверяет наличие VirtualService для Gateway и домена
 *
 * - (r *HTTP01SolverPodReconciler) createVirtualServiceForSolver(ctx, pod, gateway, domain) error
 *   Создает VirtualService для доступа к поду HTTP01 solver через Gateway
 *
 * - (r *HTTP01SolverPodReconciler) updateVirtualServiceForSolver(ctx, pod, existingVS) error
 *   Обновляет существующий VirtualService для нового пода HTTP01 solver
 *
 * - (r *HTTP01SolverPodReconciler) findVirtualServicesForPod(ctx, podName, podNamespace) ([]*VirtualService, error)
 *   Находит все VirtualService, связанные с указанным подом
 *
 * - (r *HTTP01SolverPodReconciler) deleteVirtualServicesForPod(ctx, podName, podNamespace) error
 *   Удаляет все VirtualService, связанные с указанным подом
 *
 * - (r *HTTP01SolverPodReconciler) cleanupOrphanedVirtualServices(ctx) error
 *   Удаляет VirtualService, которые ссылаются на несуществующие поды или сервисы
 *
 * - (r *HTTP01SolverPodReconciler) isVirtualServiceValid(ctx, vs) (bool, error)
 *   Проверяет актуальность VirtualService - существуют ли связанные под и сервис
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istioapinetworkingv1beta1 "istio.io/api/networking/v1beta1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

const (
	// http01SolverLabelValue is the value for the HTTP01 solver label
	http01SolverLabelValue = "true"
	// defaultCertManagerNamespace is the default namespace for cert-manager pods
	defaultCertManagerNamespace = "istio-system"
)

// findVirtualServiceForDomain проверяет наличие VirtualService для Gateway и домена
// Сначала ищет в namespace Gateway, затем во всех namespace
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
