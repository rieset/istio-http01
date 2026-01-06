# Istio Gas Helm Chart

Helm chart для установки Istio Gas оператора в Kubernetes кластер.

## Установка

```bash
helm install istio-http01 ./helm/istio-http01 -n istio-http01 --create-namespace
```

## Конфигурация

Основные параметры конфигурации в `values.yaml`:

- `watchNamespace` - namespace для мониторинга Gateway (пустое значение = все namespace)
- `replicaCount` - количество реплик (1 при включенном leader election)
- `leaderElection.enabled` - включение leader election (по умолчанию true)
- `resources` - ограничения ресурсов
- `image.repository` и `image.tag` - образ оператора

## Требования

- Kubernetes 1.19+
- cert-manager установлен в кластере
- Istio установлен в кластере

## Что мониторит оператор

Оператор выводит в логи информацию о:

1. **Certificate** (cert-manager.io) - в своем namespace
2. **HTTP01 Solver Pods** (cm-acme-http-solver-*) - в своем namespace
3. **Issuer** (cert-manager.io) - в своем namespace
4. **Istio Gateway** (networking.istio.io) - во всех namespace
5. **VirtualService** (networking.istio.io) - связанные с Gateway, включая домены

## Проверка работы

После установки оператор начнет логировать информацию о найденных ресурсах:

```bash
kubectl logs -n istio-http01 -l app.kubernetes.io/name=istio-http01 -f
```

Вы должны увидеть логи вида:
- "Certificate detected" - при обнаружении сертификата
- "HTTP01 Solver Pod detected" - при обнаружении solver пода
- "Issuer detected" - при обнаружении Issuer
- "Istio Gateway detected" - при обнаружении Gateway
- "VirtualService host" - домены из VirtualService

