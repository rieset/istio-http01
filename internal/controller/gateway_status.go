/*
 * Функции, определенные в этом файле:
 *
 * - (r *GatewayReconciler) getAllGatewaysWithDomains(ctx) (map[string][]string, error)
 *   Получает все Gateway и их домены для обновления статуса пода
 *
 * - (r *GatewayReconciler) updateOperatorPodStatus(ctx) error
 *   Обновляет статус пода оператора с информацией о Gateway и их доменах
 */

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

// getAllGatewaysWithDomains получает все Gateway и их домены для обновления статуса пода
func (r *GatewayReconciler) getAllGatewaysWithDomains(ctx context.Context) (map[string][]string, error) {
	// Получение всех Gateway во всех namespace
	gatewayList := &istionetworkingv1beta1.GatewayList{}
	if err := r.List(ctx, gatewayList, client.InNamespace("")); err != nil {
		return nil, err
	}

	gatewayDomains := make(map[string][]string)

	// Для каждого Gateway получаем домены
	for i := range gatewayList.Items {
		gateway := gatewayList.Items[i]
		domains, err := r.getDomainsForGateway(ctx, gateway)
		if err != nil {
			continue // Пропускаем Gateway с ошибками
		}
		// Используем формат "namespace/name" как ключ
		gatewayKey := fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name)
		gatewayDomains[gatewayKey] = domains
	}

	return gatewayDomains, nil
}

// updateOperatorPodStatus обновляет статус пода оператора с информацией о Gateway и их доменах
func (r *GatewayReconciler) updateOperatorPodStatus(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Получение namespace оператора из переменной окружения или использование istio-system по умолчанию
	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = defaultCertManagerNamespace
	}

	// Получение имени пода оператора из переменной окружения
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		// Если HOSTNAME не установлен, пытаемся найти под по лейблам
		podList := &corev1.PodList{}
		if err := r.List(ctx, podList, client.InNamespace(operatorNamespace), client.MatchingLabels{
			"app.kubernetes.io/name": "istio-http01",
			"control-plane":          "controller-manager",
		}); err != nil {
			return fmt.Errorf("failed to list operator pods: %w", err)
		}
		if len(podList.Items) == 0 {
			return fmt.Errorf("operator pod not found")
		}
		// Берем первый под (если leader election включен, будет только один)
		podName = podList.Items[0].Name
	}

	// Получение всех Gateway и их доменов
	gatewayDomains, err := r.getAllGatewaysWithDomains(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all gateways with domains: %w", err)
	}

	// Преобразование в JSON для хранения в аннотации
	gatewayData, err := json.MarshalIndent(gatewayDomains, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal gateway domains: %w", err)
	}

	// Получение пода оператора
	operatorPod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      podName,
		Namespace: operatorNamespace,
	}, operatorPod); err != nil {
		return fmt.Errorf("failed to get operator pod: %w", err)
	}

	// Обновление аннотаций пода (статус пода read-only, используем аннотации)
	if operatorPod.Annotations == nil {
		operatorPod.Annotations = make(map[string]string)
	}
	operatorPod.Annotations["istio-http01.rieset.io/gateway-domains"] = string(gatewayData)

	// Обновление пода
	if err := r.Update(ctx, operatorPod); err != nil {
		return fmt.Errorf("failed to update operator pod: %w", err)
	}

	logger.Info("Updated operator pod status with Gateway domains",
		"podName", podName,
		"namespace", operatorNamespace,
		"gatewayCount", len(gatewayDomains),
	)

	return nil
}
