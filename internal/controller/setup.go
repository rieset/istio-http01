/*
 * Функции, определенные в этом файле:
 *
 * - SetupControllers(mgr) error
 *   Настраивает и регистрирует все контроллеры оператора
 */

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupControllers настраивает все контроллеры оператора
func SetupControllers(mgr ctrl.Manager) error {
	// Certificate controller
	if err := (&CertificateReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// HTTP01 Solver Pod controller
	if err := (&HTTP01SolverPodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Issuer controller
	if err := (&IssuerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// Istio Gateway controller
	if err := (&GatewayReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}
