/*
 * Функции, определенные в этом файле:
 *
 * - (r *CertificateReconciler) createEnvoyFilterToDisableHSTS(ctx, gateway, originalSecretName) error
 *   Создает EnvoyFilter для отключения HSTS заголовка
 *
 * - (r *CertificateReconciler) deleteEnvoyFilterForHSTS(ctx, gateway, originalSecretName) error
 *   Удаляет EnvoyFilter для отключения HSTS
 */

package controller

import (
	"context"
	"fmt"
	"strings"

	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createEnvoyFilterToDisableHSTS создает EnvoyFilter для отключения HSTS заголовка
func (r *CertificateReconciler) createEnvoyFilterToDisableHSTS(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, originalSecretName string) error {
	logger := log.FromContext(ctx)

	envoyFilterName := fmt.Sprintf("disable-hsts-%s-%s", gateway.Namespace, gateway.Name)
	envoyFilterNamespace := gateway.Namespace

	// Проверяем, не создан ли уже EnvoyFilter
	existingFilter := &istionetworkingv1alpha3.EnvoyFilter{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      envoyFilterName,
		Namespace: envoyFilterNamespace,
	}, existingFilter); err == nil {
		// EnvoyFilter уже существует
		logger.V(1).Info("EnvoyFilter to disable HSTS already exists",
			"envoyFilterName", envoyFilterName,
			"namespace", envoyFilterNamespace,
		)
		return nil
	}

	// Создаем EnvoyFilter для отключения HSTS заголовка через unstructured
	// Используем HTTP_FILTER для удаления заголовка через Lua filter
	envoyFilter := &unstructured.Unstructured{}
	envoyFilter.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1alpha3",
		Kind:    "EnvoyFilter",
	})
	envoyFilter.SetName(envoyFilterName)
	envoyFilter.SetNamespace(envoyFilterNamespace)
	envoyFilter.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by":         "istio-http01",
		"istio-http01.rieset.io/temp":          tempLabelValue,
		"istio-http01.rieset.io/original-cert": originalSecretName,
	})

	// Устанавливаем spec через unstructured
	// Получаем селектор из Gateway для правильного выбора workload
	workloadLabels := make(map[string]interface{})
	if len(gateway.Spec.Selector) > 0 {
		// Используем селектор из Gateway
		for key, value := range gateway.Spec.Selector {
			workloadLabels[key] = value
		}
	} else {
		// Если селектор не указан, используем стандартный istio ingressgateway
		workloadLabels["istio"] = "ingressgateway"
	}

	spec := map[string]interface{}{
		"workloadSelector": map[string]interface{}{
			"labels": workloadLabels,
		},
		"configPatches": []interface{}{
			map[string]interface{}{
				"applyTo": "HTTP_FILTER",
				"match": map[string]interface{}{
					"context": "GATEWAY",
					"proxy": map[string]interface{}{
						"proxyVersion": ".*",
					},
					"listener": map[string]interface{}{
						"filterChain": map[string]interface{}{
							"filter": map[string]interface{}{
								"name": "envoy.filters.network.http_connection_manager",
							},
						},
					},
				},
				"patch": map[string]interface{}{
					"operation": "INSERT_BEFORE",
					"value": map[string]interface{}{
						"name": "envoy.filters.http.lua",
						"typed_config": map[string]interface{}{
							"@type":       "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua",
							"inline_code": "function envoy_on_response(response_handle)\n  response_handle:headers():remove(\"strict-transport-security\")\nend\n",
						},
					},
				},
			},
		},
	}
	if err := unstructured.SetNestedMap(envoyFilter.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set EnvoyFilter spec: %w", err)
	}

	if err := r.Create(ctx, envoyFilter); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create EnvoyFilter to disable HSTS: %w", err)
		}
		logger.V(1).Info("EnvoyFilter to disable HSTS already exists",
			"envoyFilterName", envoyFilterName,
			"namespace", envoyFilterNamespace,
		)
	} else {
		logger.Info("Created EnvoyFilter to disable HSTS",
			"envoyFilterName", envoyFilterName,
			"gatewayName", gateway.Name,
			"gatewayNamespace", gateway.Namespace,
		)
	}

	return nil
}

// deleteEnvoyFilterForHSTS удаляет EnvoyFilter для отключения HSTS
func (r *CertificateReconciler) deleteEnvoyFilterForHSTS(ctx context.Context, gateway *istionetworkingv1beta1.Gateway, _ string) error {
	logger := log.FromContext(ctx)

	envoyFilterName := fmt.Sprintf("disable-hsts-%s-%s", gateway.Namespace, gateway.Name)
	envoyFilterNamespace := gateway.Namespace

	// Используем unstructured для получения EnvoyFilter, так как тип v1alpha3.EnvoyFilter не зарегистрирован в схеме
	envoyFilter := &unstructured.Unstructured{}
	envoyFilter.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1alpha3",
		Kind:    "EnvoyFilter",
	})
	envoyFilter.SetName(envoyFilterName)
	envoyFilter.SetNamespace(envoyFilterNamespace)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      envoyFilterName,
		Namespace: envoyFilterNamespace,
	}, envoyFilter); err != nil {
		// EnvoyFilter не найден, возможно уже удален
		return nil
	}

	// Проверяем, что это наш EnvoyFilter
	labels, found, err := unstructured.NestedStringMap(envoyFilter.Object, "metadata", "labels")
	if err != nil || !found || labels["istio-http01.rieset.io/temp"] != tempLabelValue {
		return nil
	}

	if err := r.Delete(ctx, envoyFilter); err != nil {
		logger.Error(err, "failed to delete EnvoyFilter for HSTS",
			"envoyFilterName", envoyFilterName,
		)
		return fmt.Errorf("failed to delete EnvoyFilter: %w", err)
	}

	logger.Info("EnvoyFilter для отключения HSTS удален (HSTS включен обратно)",
		"envoyFilterName", envoyFilterName,
		"gatewayName", gateway.Name,
		"gatewayNamespace", gateway.Namespace,
	)

	return nil
}
