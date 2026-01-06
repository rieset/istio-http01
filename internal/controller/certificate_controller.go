/*
 * Функции, определенные в этом файле:
 *
 * - (r *CertificateReconciler) Reconcile(ctx, req) (ctrl.Result, error)
 *   Обрабатывает изменения Certificate ресурсов и выводит информацию в логи
 *
 * - (r *CertificateReconciler) SetupWithManager(mgr) error
 *   Настраивает контроллер для работы с менеджером
 */

package controller

import (
	"context"
	"fmt"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CertificateReconciler реконсилирует Certificate ресурсы
type CertificateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates/status,verbs=get

// Reconcile обрабатывает Certificate ресурсы
func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Получение Certificate
	cert := &certmanagerv1.Certificate{}
	if err := r.Get(ctx, req.NamespacedName, cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Вывод информации о Certificate
	logger.Info("Certificate detected",
		"certificateName", cert.Name,
		"certificateNamespace", cert.Namespace,
		"dnsNames", cert.Spec.DNSNames,
		"issuerRef", fmt.Sprintf("%s/%s", cert.Spec.IssuerRef.Kind, cert.Spec.IssuerRef.Name),
		"secretName", cert.Spec.SecretName,
	)

	// Дополнительная информация о статусе
	if len(cert.Status.Conditions) > 0 {
		for _, condition := range cert.Status.Conditions {
			logger.Info("Certificate condition",
				"certificateName", cert.Name,
				"certificateNamespace", cert.Namespace,
				"type", condition.Type,
				"status", condition.Status,
				"reason", condition.Reason,
				"message", condition.Message,
			)
		}
	}

	// Информация о DNS names
	if len(cert.Spec.DNSNames) > 0 {
		logger.Info("Certificate DNS names",
			"certificateName", cert.Name,
			"certificateNamespace", cert.Namespace,
			"dnsNames", cert.Spec.DNSNames,
		)
	}

	// Информация о Common Name
	if cert.Spec.CommonName != "" {
		logger.Info("Certificate common name",
			"certificateName", cert.Name,
			"certificateNamespace", cert.Namespace,
			"commonName", cert.Spec.CommonName,
		)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager настраивает контроллер
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certmanagerv1.Certificate{}).
		Complete(r)
}
