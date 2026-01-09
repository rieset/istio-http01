/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) Reconcile(ctx, req) (ctrl.Result, error)
 *   Обрабатывает изменения подов cm-acme-http-solver-* и выводит информацию в логи
 *
 * - (r *HTTP01SolverPodReconciler) extractDomainFromPod(pod) (string, error)
 *   Извлекает домен из аргументов контейнера acmesolver
 *
 * - (r *HTTP01SolverPodReconciler) SetupWithManager(mgr) error
 *   Настраивает контроллер для работы с менеджером
 *
 * Дополнительные функции находятся в:
 * - http01_solver_gateway.go - поиск Gateway для домена
 * - http01_solver_virtualservice.go - работа с VirtualService
 * - http01_solver_service.go - поиск Service для пода
 */

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// HTTP01SolverPodReconciler реконсилирует HTTP01 solver поды
type HTTP01SolverPodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;patch;update;delete

// Reconcile обрабатывает HTTP01 solver поды
func (r *HTTP01SolverPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Получение Pod
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		// Если под не найден (удален), удаляем связанные VirtualService
		if client.IgnoreNotFound(err) == nil {
			// Под был удален - удаляем связанные VirtualService
			if err := r.deleteVirtualServicesForPod(ctx, req.Name, req.Namespace); err != nil {
				ctrl.Log.Error(err, "failed to delete VirtualServices for removed pod",
					"pod", req.Name,
					"namespace", req.Namespace,
				)
				// Продолжаем выполнение, даже если не удалось удалить VirtualService
			}
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Проверка, что это HTTP01 solver под
	if !strings.HasPrefix(pod.Name, "cm-acme-http-solver-") {
		return ctrl.Result{}, nil
	}

	// Извлечение домена из аргументов контейнера
	domain := r.extractDomainFromPod(pod)

	if domain == "" {
		return ctrl.Result{}, nil
	}

	// Поиск Gateway для этого домена
	gateway, err := r.findGatewayForDomain(ctx, domain)
	if err != nil {
		ctrl.Log.Error(err, "failed to find Gateway for domain",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"domain", domain,
		)
		return ctrl.Result{}, nil
	}

	if gateway == nil {
		err := fmt.Errorf("no Gateway found for domain %s", domain)
		ctrl.Log.Error(err, "Gateway not found for HTTP01 solver domain",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"domain", domain,
		)
		return ctrl.Result{}, err
	}

	// Проверка наличия VirtualService для этого домена и Gateway
	existingVS, err := r.findVirtualServiceForDomain(ctx, gateway, domain)
	if err != nil {
		ctrl.Log.Error(err, "failed to check for existing VirtualService",
			"pod", pod.Name,
			"domain", domain,
			"gateway", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		return ctrl.Result{}, nil
	}

	// Если VirtualService не найден по домену, проверяем все VirtualService оператора в namespace Gateway
	// на актуальность и удаляем неактуальные
	if existingVS == nil {
		if err := r.cleanupOrphanedVirtualServicesInNamespace(ctx, gateway.Namespace); err != nil {
			ctrl.Log.Error(err, "failed to cleanup orphaned VirtualServices in namespace",
				"namespace", gateway.Namespace,
			)
			// Продолжаем выполнение, даже если не удалось очистить
		}
	}

	if existingVS != nil {
		// Проверяем актуальность VirtualService - существуют ли связанные под и сервис
		isValid := r.isVirtualServiceValid(ctx, existingVS)
		if !isValid {
			// VirtualService неактуален - удаляем его и создадим новый
			ctrl.Log.Info("VirtualService is not valid, deleting it",
				"virtualService", existingVS.Name,
				"virtualServiceNamespace", existingVS.Namespace,
				"pod", pod.Name,
			)
			if err := r.Delete(ctx, existingVS); err != nil {
				ctrl.Log.Error(err, "failed to delete invalid VirtualService",
					"virtualService", existingVS.Name,
					"virtualServiceNamespace", existingVS.Namespace,
				)
				// Продолжаем выполнение, попробуем создать новый VirtualService
			} else {
				ctrl.Log.Info("Deleted invalid VirtualService",
					"virtualService", existingVS.Name,
					"virtualServiceNamespace", existingVS.Namespace,
				)
				// Продолжаем выполнение, чтобы создать новый VirtualService
				existingVS = nil
			}
		}

		// Если VirtualService был удален как неактуальный, создаем новый
		if existingVS == nil {
			// Продолжаем выполнение для создания нового VirtualService
		} else {
			// Проверяем, что VirtualService принадлежит текущему поду
			vsPodName := existingVS.Labels["acme.cert-manager.io/solver-pod"]
			if vsPodName != pod.Name {
				// Обновляем VirtualService для нового пода
				if err := r.updateVirtualServiceForSolver(ctx, pod, existingVS); err != nil {
					ctrl.Log.Error(err, "failed to update VirtualService for solver",
						"pod", pod.Name,
						"domain", domain,
						"gateway", gateway.Name,
						"virtualService", existingVS.Name,
					)
					return ctrl.Result{}, err
				}
				ctrl.Log.Info("Updated VirtualService for HTTP01 solver",
					"pod", pod.Name,
					"domain", domain,
					"gateway", gateway.Name,
					"virtualService", existingVS.Name,
				)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}

			// VirtualService уже существует и принадлежит текущему поду
			// Периодически проверяем и очищаем orphaned VirtualService
			if err := r.cleanupOrphanedVirtualServices(ctx); err != nil {
				ctrl.Log.Error(err, "failed to cleanup orphaned VirtualServices")
				// Продолжаем выполнение, даже если не удалось очистить
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Создание VirtualService для доступа к поду солвера
	if err := r.createVirtualServiceForSolver(ctx, pod, gateway, domain); err != nil {
		ctrl.Log.Error(err, "failed to create VirtualService for solver",
			"pod", pod.Name,
			"domain", domain,
			"gateway", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
		return ctrl.Result{}, err
	}

	ctrl.Log.Info("Created VirtualService for HTTP01 solver",
		"pod", pod.Name,
		"domain", domain,
		"gateway", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
	)

	// Периодически проверяем и очищаем orphaned VirtualService
	if err := r.cleanupOrphanedVirtualServices(ctx); err != nil {
		ctrl.Log.Error(err, "failed to cleanup orphaned VirtualServices")
		// Продолжаем выполнение, даже если не удалось очистить
	}

	// Перепроверка через 30 секунд для убеждения что VirtualService существует
	// и на случай если он был удален пользователем
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager настраивает контроллер
func (r *HTTP01SolverPodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Предикат для фильтрации только HTTP01 solver подов
	http01SolverPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod := obj.(*corev1.Pod)
		// Проверка имени и метки
		return strings.HasPrefix(pod.Name, "cm-acme-http-solver-") &&
			pod.Labels["acme.cert-manager.io/http01-solver"] == http01SolverLabelValue
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(http01SolverPredicate).
		Complete(r)
}

// extractDomainFromPod перенесена в http01_solver_pod_extract.go
