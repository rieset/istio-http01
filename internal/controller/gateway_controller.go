/*
 * Функции, определенные в этом файле:
 *
 * - (r *GatewayReconciler) Reconcile(ctx, req) (ctrl.Result, error)
 *   Обрабатывает изменения Istio Gateway ресурсов и выводит информацию в логи
 *
 * - (r *GatewayReconciler) SetupWithManager(mgr) error
 *   Настраивает контроллер для работы с менеджером
 *
 * Дополнительные функции находятся в:
 * - gateway_virtualservice.go - работа с VirtualService и доменами Gateway
 * - gateway_status.go - обновление статуса пода оператора
 */

package controller

import (
	"context"
	"time"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GatewayReconciler реконсилирует Istio Gateway ресурсы
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;update

// Reconcile обрабатывает Istio Gateway ресурсы
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Получение Gateway
	gateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Создаем контекстный логгер с информацией о Gateway
	logger = logger.WithValues(
		"gateway", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
	)

	logger.Info("Istio Gateway detected")

	// Информация о серверах (логируем только сводку)
	if len(gateway.Spec.Servers) > 0 {
		serverCount := len(gateway.Spec.Servers)
		totalHosts := 0
		for _, server := range gateway.Spec.Servers {
			totalHosts += len(server.Hosts)
		}
		logger.Info("Gateway servers",
			"serverCount", serverCount,
			"totalHosts", totalHosts,
		)
	}

	// Получение связанных VirtualService
	virtualServices, err := r.getVirtualServicesForGateway(ctx, gateway)
	if err != nil {
		logger.Error(err, "failed to get VirtualServices for Gateway")
	} else {
		logger.Info("Found VirtualServices",
			"count", len(virtualServices),
		)

		// Логируем сводку по VirtualService хостам (без дублирования)
		if len(virtualServices) > 0 {
			allHosts := make([]string, 0)
			for _, vs := range virtualServices {
				allHosts = append(allHosts, vs.Spec.Hosts...)
			}
			ctrl.Log.Info("VirtualService hosts",
				"Gateway", map[string]string{
					"name":      gateway.Name,
					"namespace": gateway.Namespace,
				},
				"hosts", allHosts,
			)
		}
	}

	// Получение доменов, за которые отвечает Gateway на основе VirtualService
	domains, err := r.getDomainsForGateway(ctx, gateway)
	if err != nil {
		logger.Error(err, "failed to get domains for Gateway")
	} else {
		// Логирование доменов Gateway на основе VirtualService
		ctrl.Log.WithValues(
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
			"domains", domains,
		).Info("Gateway domains from VirtualService")
	}

	// Обновление статуса пода оператора с информацией о Gateway и их доменах
	if err := r.updateOperatorPodStatus(ctx); err != nil {
		logger.Error(err, "failed to update operator pod status")
	}

	// Периодическая проверка ответственности Gateway за домены VirtualService
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// SetupWithManager настраивает контроллер
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&istionetworkingv1beta1.Gateway{}).
		Complete(r)
}
