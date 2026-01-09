/*
 * Функции, определенные в этом файле:
 *
 * - (r *HTTP01SolverPodReconciler) extractDomainFromPod(pod) string
 *   Извлекает домен из аргументов контейнера acmesolver
 */

package controller

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// extractDomainFromPod извлекает домен из аргументов контейнера acmesolver
func (r *HTTP01SolverPodReconciler) extractDomainFromPod(pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		if container.Name == "acmesolver" {
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--domain=") {
					return strings.TrimPrefix(arg, "--domain=")
				}
			}
		}
	}
	return ""
}
