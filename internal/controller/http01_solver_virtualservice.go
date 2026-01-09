/*
 * Функции VirtualService перенесены в отдельные модули:
 *
 * - http01_solver_vs_find.go - поиск VirtualService
 * - http01_solver_vs_create.go - создание VirtualService
 * - http01_solver_vs_update.go - обновление VirtualService
 * - http01_solver_vs_cleanup.go - удаление и очистка VirtualService
 * - http01_solver_vs_validation.go - валидация VirtualService
 */

package controller

const (
	// http01SolverLabelValue is the value for the HTTP01 solver label
	http01SolverLabelValue = "true"
	// defaultCertManagerNamespace is the default namespace for cert-manager pods
	defaultCertManagerNamespace = "istio-system"
)
