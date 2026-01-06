# Инструкция по установке Istio Gas Operator

## Предварительные требования

1. Kubernetes кластер (версия 1.19+)
2. cert-manager установлен в кластере
3. Istio установлен в кластере
4. Helm 3.x установлен
5. kubectl настроен для доступа к кластеру

## Сборка и отправка образа

Перед установкой оператора необходимо собрать и отправить Docker образ в registry.

### Быстрая сборка

```bash
# 1. Сборка образа для Linux amd64
make docker-build-amd64 IMG=rieset/istio-http01:0.1.0

# 2. Отправка образа в registry
docker push rieset/istio-http01:0.1.0

# 3. Деплой
helm upgrade --install istio-http01 ./helm/istio-http01 -n istio-system --create-namespace
```

**Важно**: 
- Убедитесь, что Docker BuildKit включен: `export DOCKER_BUILDKIT=1`
- Замените `rieset/istio-http01:0.1.0` на ваш registry и тег

Подробная инструкция: [docs/build.md](docs/build.md)

## Установка через Helm

### Вариант 1: Установка из GitHub Packages (рекомендуется)

Helm chart автоматически публикуется в GitHub Packages (OCI registry) при создании тегов версий (например, `v0.1.0`).

**⚠️ Важно:** Chart публикуется автоматически только при создании тега версии. Убедитесь, что нужная версия существует.

**Проверка доступных версий:**

```bash
# Просмотр информации о chart (замените 0.1.0 на актуальную версию)
helm show chart oci://ghcr.io/rieset/helm-charts/istio-http01 --version 0.1.0

# Или проверьте доступные версии на странице GitHub Packages:
# https://github.com/rieset?tab=packages&repo_name=istio-http01
```

**Быстрая установка:**

```bash
# Замените 0.1.0 на актуальную версию
helm install istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version 0.1.0 \
  -n istio-system \
  --create-namespace
```

**Установка с кастомными значениями:**

```bash
helm install istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version 0.1.0 \
  -n istio-system \
  --create-namespace \
  --set image.repository=rieset/istio-http01 \
  --set image.tag=0.1.0 \
  --set replicaCount=1
```

**Просмотр всех значений по умолчанию:**

```bash
helm show values oci://ghcr.io/rieset/helm-charts/istio-http01 --version 0.1.0
```

**Аутентификация (для приватных репозиториев или если chart не найден):**

Если chart не найден или репозиторий приватный, может потребоваться аутентификация:

```bash
# Создайте Personal Access Token (PAT) в GitHub:
# Settings -> Developer settings -> Personal access tokens -> Tokens (classic)
# Выберите права: read:packages
# Затем выполните:
helm registry login ghcr.io -u YOUR_GITHUB_USERNAME
# Введите PAT при запросе пароля
```

**Где найти актуальную версию:**

1. Перейдите на страницу пакета: https://github.com/rieset?tab=packages
2. Найдите пакет `helm-charts/istio-http01`
3. Выберите нужную версию из списка

**Если chart не найден:**

1. Убедитесь, что тег версии был создан (например, `git tag v0.1.0 && git push origin v0.1.0`)
2. Проверьте, что workflow `build-helm-chart.yml` успешно выполнился в GitHub Actions
3. Проверьте доступные версии на странице GitHub Packages
4. Если chart еще не опубликован, используйте локальную установку (см. Вариант 2)

### Вариант 2: Установка из локального chart

Если вы хотите установить chart из локальной директории:

**1. Подготовка зависимостей:**

```bash
go mod download
go mod tidy
```

**2. Обновление values.yaml:**

Обновите `helm/istio-http01/values.yaml` с правильным образом (если отличается от `rieset/istio-http01:0.1.0`):

```yaml
image:
  repository: rieset/istio-http01
  tag: "0.1.0"
```

**3. Установка:**

```bash
helm upgrade --install istio-http01 ./helm/istio-http01 -n istio-system --create-namespace
```

**Примечание**: Используется namespace `istio-system` для работы оператора в том же namespace, где установлен Istio.

### 5. Проверка установки

```bash
# Проверка подов
kubectl get pods -n istio-system

# Проверка логов
kubectl logs -n istio-system -l app.kubernetes.io/name=istio-http01 -f
```

## Обновление

### Обновление из GitHub Packages

```bash
# Обновление до новой версии (замените 0.1.0 на нужную версию)
helm upgrade istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version 0.1.0 \
  -n istio-system

# Обновление с кастомными значениями
helm upgrade istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version 0.1.0 \
  -n istio-system \
  --set image.tag=0.1.0 \
  --reuse-values
```

### Обновление из локального chart

```bash
# Обновление через Helm
helm upgrade istio-http01 ./helm/istio-http01 -n istio-system
```

## Удаление

```bash
# Удаление через Helm
helm uninstall istio-http01 -n istio-system
```

## Конфигурация

Основные параметры конфигурации в `helm/istio-http01/values.yaml`:

- `watchNamespace` - namespace для мониторинга Gateway (пустое = все namespace)
- `replicaCount` - количество реплик
- `leaderElection.enabled` - включение leader election (по умолчанию true)
- `resources` - ограничения ресурсов

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
kubectl logs -n istio-system -l app.kubernetes.io/name=istio-http01 -f
```

Вы должны увидеть логи вида:
- "Certificate detected" - при обнаружении сертификата
- "HTTP01 Solver Pod detected" - при обнаружении solver пода
- "Issuer detected" - при обнаружении Issuer
- "Istio Gateway detected" - при обнаружении Gateway
- "VirtualService host" - домены из VirtualService

## Разработка

### Локальный запуск

```bash
# Установка CRD (если есть)
make install

# Запуск оператора локально
make run
```

### Тестирование

```bash
# Запуск unit тестов
make test

# Запуск e2e тестов
make test-e2e
```

