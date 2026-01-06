# Руководство по созданию Go оператора с Operator SDK

Этот документ анализирует процесс создания Kubernetes оператора на Go с использованием Operator SDK на основе официальной документации.

**Официальная документация**: https://sdk.operatorframework.io/docs/building-operators/golang/

## Предварительные требования

Согласно [официальной документации](https://sdk.operatorframework.io/docs/building-operators/golang/installation/), для создания Go оператора требуется:

### Обязательные компоненты
- **git** - система контроля версий
- **Go версия 1.22+** - язык программирования
  - ⚠️ **Важно**: В проекте указана версия Go 1.23+, что соответствует требованиям
- **Docker версия 17.03+** - для сборки образов оператора
- **kubectl** - клиент для работы с Kubernetes
- **Доступ к Kubernetes кластеру** совместимой версии

### Operator SDK CLI
- Установленный `operator-sdk` CLI инструмент
- См. [руководство по установке](https://sdk.operatorframework.io/docs/installation/)

## Процесс создания оператора

### Шаг 1: Инициализация проекта

Команда для инициализации нового проекта:

```bash
operator-sdk init --domain <domain> --repo <repo>
```

**Параметры:**
- `--domain` - домен для группировки API (например, `example.com`)
- `--repo` - путь к репозиторию Go модуля (например, `github.com/example/memcached-operator`)

**Что создается:**
- Базовая структура проекта
- `go.mod` файл с зависимостями
- `Makefile` с командами для сборки и развертывания
- Конфигурация Kustomize
- Базовые файлы для тестирования

**Пример для нашего проекта:**
```bash
operator-sdk init --domain example.com --repo github.com/example/istio-http01
```

### Шаг 2: Создание API и контроллера

Команда для создания Custom Resource и контроллера:

```bash
operator-sdk create api --group <group> --version <version> --kind <Kind> --resource --controller
```

**Параметры:**
- `--group` - группа API (например, `cache`, `cert`)
- `--version` - версия API (например, `v1alpha1`, `v1`)
- `--kind` - тип ресурса (например, `Memcached`, `CertMonitor`)
- `--resource` - создать Custom Resource Definition
- `--controller` - создать контроллер

**Что создается:**
- CRD определение в `api/<version>/<kind>_types.go`
- Контроллер в `controllers/<kind>_controller.go`
- Базовые тесты в `controllers/<kind>_controller_test.go`
- RBAC манифесты для доступа к ресурсам

**Пример для нашего проекта:**
```bash
operator-sdk create api --group cert --version v1 --kind CertMonitor --resource --controller
```

### Шаг 3: Определение Custom Resource

После создания API необходимо определить структуру Custom Resource в файле `api/v1/certmonitor_types.go`:

```go
// CertMonitorSpec определяет желаемое состояние CertMonitor
type CertMonitorSpec struct {
    // Поля спецификации
    Domain string `json:"domain,omitempty"`
    // ...
}

// CertMonitorStatus определяет наблюдаемое состояние CertMonitor
type CertMonitorStatus struct {
    // Поля статуса
    PodsReady int `json:"podsReady,omitempty"`
    // ...
}

// CertMonitor - это Custom Resource для мониторинга cert-manager подов
type CertMonitor struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   CertMonitorSpec   `json:"spec,omitempty"`
    Status CertMonitorStatus `json:"status,omitempty"`
}
```

**Важно:**
- После изменения типов необходимо запустить `make generate` для обновления кода
- Запустить `make manifests` для генерации CRD манифестов

### Шаг 4: Реализация логики контроллера

Основная логика оператора реализуется в методе `Reconcile` контроллера:

```go
func (r *CertMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    
    // 1. Получение Custom Resource
    instance := &certv1.CertMonitor{}
    if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }
    
    // 2. Выполнение логики reconciliation
    // - Поиск подов cert-manager
    // - Проверка их состояния
    // - Обновление статуса
    
    // 3. Возврат результата
    return ctrl.Result{}, nil
}
```

**Ключевые моменты:**
- Использование `context.Context` для отмены операций
- Обработка ошибок `IsNotFound` для удаленных ресурсов
- Логирование через `log.FromContext(ctx)`
- Возврат `ctrl.Result{}` для успешного завершения

### Шаг 5: Настройка Watch для Pods

Для мониторинга подов cert-manager необходимо настроить watch:

```go
func (r *CertMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&certv1.CertMonitor{}).
        Owns(&corev1.Pod{}).  // Watch для подов
        WithOptions(controller.Options{
            MaxConcurrentReconciles: 1,
        }).
        Complete(r)
}
```

**Использование Predicates для фильтрации:**

```go
import "sigs.k8s.io/controller-runtime/pkg/predicate"

func (r *CertMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Предикат для фильтрации только HTTP01 solver подов
    http01SolverPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
        pod := obj.(*corev1.Pod)
        return pod.Labels["acme.cert-manager.io/http01-solver"] == "true"
    })
    
    return ctrl.NewControllerManagedBy(mgr).
        For(&certv1.CertMonitor{}).
        Owns(&corev1.Pod{}, builder.WithPredicates(http01SolverPredicate)).
        Complete(r)
}
```

### Шаг 6: Тестирование

Operator SDK использует `envtest` из controller-runtime для тестирования без реального кластера:

```go
// controllers/suite_test.go
var (
    cfg     *rest.Config
    k8sClient client.Client
    testEnv  *envtest.Environment
)

func TestMain(m *testing.M) {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }
    
    cfg, _ = testEnv.Start()
    k8sClient, _ = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    
    code := m.Run()
    testEnv.Stop()
    os.Exit(code)
}
```

**Тестирование контроллера:**

```go
func TestCertMonitorReconciler(t *testing.T) {
    // Настройка тестового окружения
    // Создание тестовых ресурсов
    // Вызов Reconcile
    // Проверка результатов
}
```

### Шаг 7: Генерация кода и манифестов

После изменения типов и контроллера:

```bash
# Генерация кода (deepcopy, client, etc.)
make generate

# Генерация CRD манифестов
make manifests

# Генерация всего (код + манифесты)
operator-sdk generate kustomize manifests
```

### Шаг 8: Локальный запуск

Для разработки и отладки:

```bash
# Запуск оператора локально (требует доступ к кластеру)
make run

# Или с явным указанием namespace
make run NAMESPACE=istio-system
```

**Требования:**
- Настроенный `kubectl` с доступом к кластеру
- Установленные CRD в кластере (`make install`)

### Шаг 9: Сборка образа

Сборка Docker образа оператора:

```bash
# Сборка образа
make docker-build IMG=<registry>/<name>:<tag>

# Пример
make docker-build IMG=quay.io/example/istio-http01:v0.1.0
```

**Что происходит:**
- Компиляция Go кода в бинарный файл
- Создание Docker образа с оператором
- Использование базового образа из `Dockerfile`

### Шаг 10: Развертывание

Развертывание оператора в кластер:

```bash
# Установка CRD
make install

# Развертывание оператора
make deploy IMG=<registry>/<name>:<tag>

# Пример
make deploy IMG=quay.io/example/istio-http01:v0.1.0
```

**Что создается:**
- Namespace для оператора
- ServiceAccount с RBAC правами
- Deployment с оператором
- Service (если требуется)

## Структура проекта после инициализации

```
.
├── api/                    # API определения
│   └── v1/
│       ├── certmonitor_types.go      # Определения типов
│       ├── certmonitor_types_test.go # Тесты типов
│       ├── groupversion_info.go      # Версия группы
│       └── zz_generated.deepcopy.go  # Автогенерируемый код
├── config/                  # Конфигурация Kustomize
│   ├── crd/                # CRD манифесты
│   ├── rbac/               # RBAC манифесты
│   ├── manager/            # Deployment оператора
│   └── samples/            # Примеры Custom Resources
├── controllers/            # Контроллеры
│   ├── certmonitor_controller.go     # Основной контроллер
│   ├── certmonitor_controller_test.go # Тесты контроллера
│   └── suite_test.go       # Настройка тестового окружения
├── main.go                 # Точка входа
├── Makefile                # Команды для сборки и развертывания
├── go.mod                  # Go модули
├── go.sum                  # Checksums зависимостей
└── Dockerfile              # Docker образ оператора
```

## Ключевые команды Makefile

| Команда | Описание |
|---------|----------|
| `make generate` | Генерация кода (deepcopy, client) |
| `make manifests` | Генерация манифестов (CRD, RBAC) |
| `make install` | Установка CRD в кластер |
| `make uninstall` | Удаление CRD из кластера |
| `make run` | Запуск оператора локально |
| `make docker-build` | Сборка Docker образа |
| `make docker-push` | Отправка образа в registry |
| `make deploy` | Развертывание оператора |
| `make undeploy` | Удаление оператора |
| `make test` | Запуск тестов |

## Best Practices из документации

### 1. Использование Finalizers
Для очистки ресурсов при удалении Custom Resource:

```go
const finalizerName = "certmonitor.example.com/finalizer"

if !controllerutil.ContainsFinalizer(instance, finalizerName) {
    controllerutil.AddFinalizer(instance, finalizerName)
    return r.Update(ctx, instance)
}

if !instance.GetDeletionTimestamp().IsZero() {
    // Выполнение очистки
    controllerutil.RemoveFinalizer(instance, finalizerName)
    return r.Update(ctx, instance)
}
```

### 2. Обновление статуса
Использование `Status().Update()` для обновления статуса:

```go
instance.Status.PodsReady = readyCount
instance.Status.Conditions = conditions
return r.Status().Update(ctx, instance)
```

### 3. Обработка ошибок
Правильная обработка различных типов ошибок:

```go
if err != nil {
    if apierrors.IsNotFound(err) {
        // Ресурс не найден - нормальная ситуация
        return ctrl.Result{}, nil
    }
    if apierrors.IsConflict(err) {
        // Конфликт версий - повторить попытку
        return ctrl.Result{Requeue: true}, nil
    }
    // Другие ошибки
    return ctrl.Result{}, err
}
```

### 4. Логирование
Структурированное логирование с контекстом:

```go
log := log.FromContext(ctx).WithValues(
    "certmonitor", req.NamespacedName,
    "namespace", req.Namespace,
)
log.Info("reconciling CertMonitor")
log.Error(err, "failed to reconcile")
```

## Ссылки на документацию

- [Установка Operator SDK](https://sdk.operatorframework.io/docs/building-operators/golang/installation/)
- [Quickstart Tutorial](https://sdk.operatorframework.io/docs/building-operators/golang/quickstart/)
- [Testing with EnvTest](https://sdk.operatorframework.io/docs/building-operators/golang/testing/)
- [Advanced Topics](https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/)
- [Controller Runtime Reference](https://sdk.operatorframework.io/docs/building-operators/golang/reference/)

---

**Примечание**: Этот документ основан на официальной документации Operator SDK v1.42.0. При обновлении версии проверяйте актуальную документацию.

