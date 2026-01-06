/*
 * Функции, определенные в этом файле:
 *
 * - (r *IssuerReconciler) Reconcile(ctx, req) (ctrl.Result, error)
 *   Обрабатывает изменения Issuer ресурсов и выводит информацию в логи
 *
 * - (r *IssuerReconciler) SetupWithManager(mgr) error
 *   Настраивает контроллер для работы с менеджером
 */

package controller

import (
	"context"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IssuerReconciler реконсилирует Issuer ресурсы
type IssuerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers/status,verbs=get

// Reconcile обрабатывает Issuer ресурсы
func (r *IssuerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Получение Issuer
	issuer := &certmanagerv1.Issuer{}
	if err := r.Get(ctx, req.NamespacedName, issuer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Вывод информации о Issuer
	logger.Info("Issuer detected",
		"issuerName", issuer.Name,
		"issuerNamespace", issuer.Namespace,
	)

	// Информация о типе Issuer
	if issuer.Spec.ACME != nil {
		logger.Info("ACME Issuer",
			"issuerName", issuer.Name,
			"issuerNamespace", issuer.Namespace,
			"server", issuer.Spec.ACME.Server,
			"email", issuer.Spec.ACME.Email,
		)
		if issuer.Spec.ACME.Solvers != nil {
			for i, solver := range issuer.Spec.ACME.Solvers {
				if solver.HTTP01 != nil {
					logger.Info("HTTP01 Solver configured",
						"issuerName", issuer.Name,
						"issuerNamespace", issuer.Namespace,
						"solverIndex", i,
					)
					if solver.HTTP01.Ingress != nil {
						if solver.HTTP01.Ingress.Class != nil {
							logger.Info("HTTP01 Ingress class",
								"issuerName", issuer.Name,
								"issuerNamespace", issuer.Namespace,
								"ingressClass", *solver.HTTP01.Ingress.Class,
							)
						}
						if solver.HTTP01.Ingress.Name != "" {
							logger.Info("HTTP01 Ingress name",
								"issuerName", issuer.Name,
								"issuerNamespace", issuer.Namespace,
								"ingressName", solver.HTTP01.Ingress.Name,
							)
						}
					}
				}
			}
		}
	}

	if issuer.Spec.SelfSigned != nil {
		logger.Info("SelfSigned Issuer",
			"issuerName", issuer.Name,
			"issuerNamespace", issuer.Namespace,
		)
	}

	if issuer.Spec.CA != nil {
		logger.Info("CA Issuer",
			"issuerName", issuer.Name,
			"issuerNamespace", issuer.Namespace,
			"secretName", issuer.Spec.CA.SecretName,
		)
	}

	if issuer.Spec.Vault != nil {
		logger.Info("Vault Issuer",
			"issuerName", issuer.Name,
			"issuerNamespace", issuer.Namespace,
			"server", issuer.Spec.Vault.Server,
			"path", issuer.Spec.Vault.Path,
		)
	}

	// Статус Issuer
	if len(issuer.Status.Conditions) > 0 {
		for _, condition := range issuer.Status.Conditions {
			logger.Info("Issuer condition",
				"issuerName", issuer.Name,
				"issuerNamespace", issuer.Namespace,
				"type", condition.Type,
				"status", condition.Status,
				"reason", condition.Reason,
				"message", condition.Message,
			)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager настраивает контроллер
func (r *IssuerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certmanagerv1.Issuer{}).
		Complete(r)
}
