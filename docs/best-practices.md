# Best Practices для разработки на Go

Этот документ описывает лучшие практики разработки, используемые в проекте Istio Gas Operator.

## Общие принципы Go

### 1. Форматирование кода
- Всегда используйте `gofmt` для форматирования кода
- Используйте `goimports` для автоматического управления импортами
- Настройте IDE для автоматического форматирования при сохранении

```go
// Хорошо: правильное форматирование
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ...
}

// Плохо: неправильное форматирование
func(r *Reconciler)Reconcile(ctx context.Context,req ctrl.Request)(ctrl.Result,error){
    // ...
}
```

### 2. Обработка ошибок
- Всегда проверяйте ошибки явно
- Не игнорируйте ошибки с помощью `_`
- Предоставляйте контекст в сообщениях об ошибках
- Используйте `fmt.Errorf` с `%w` для обертывания ошибок

```go
// Хорошо: явная обработка ошибок
if err != nil {
    return fmt.Errorf("failed to get pod: %w", err)
}

// Плохо: игнорирование ошибок
result, _ = client.Get(ctx, key, obj)
```

### 3. Использование context.Context
- Всегда передавайте `context.Context` как первый параметр в функции, которые выполняют I/O операции
- Используйте контекст для отмены операций и таймаутов
- Не храните контекст в структурах, передавайте его явно

```go
// Хорошо: context как первый параметр
func (c *Client) GetPod(ctx context.Context, name string) (*v1.Pod, error) {
    return c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

// Плохо: отсутствие context
func (c *Client) GetPod(name string) (*v1.Pod, error) {
    return c.client.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
}
```

### 4. Именование
- Используйте короткие имена для локальных переменных
- Используйте длинные имена для экспортируемых функций и типов
- Следуйте Go naming conventions:
  - `Get`, `Set`, `Is`, `Has` для булевых методов
  - `New` для конструкторов
  - Интерфейсы могут заканчиваться на `-er` (например, `Reader`, `Writer`)

```go
// Хорошо: правильное именование
type PodMonitor struct {
    client client.Client
}

func (m *PodMonitor) IsPodReady(ctx context.Context, name string) (bool, error) {
    // ...
}

// Плохо: неправильное именование
type pm struct {
    c client.Client
}

func (m *pm) check(ctx context.Context, n string) (bool, error) {
    // ...
}
```

### 5. Документация
- Все экспортируемые функции, типы и переменные должны иметь комментарии
- Комментарии должны начинаться с имени сущности
- Используйте полные предложения

```go
// PodMonitor отслеживает состояние подов cert-manager для HTTP01 challenge.
type PodMonitor struct {
    client client.Client
}

// Reconcile выполняет цикл согласования для приведения состояния к желаемому.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ...
}
```

## Практики для Kubernetes операторов

### 1. Reconciliation Loop
- Всегда возвращайте `ctrl.Result{}` при успешном завершении
- Используйте `RequeueAfter` для периодических проверок
- Используйте `Requeue: true` только при временных ошибках
- Логируйте все важные действия

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    
    // Получение ресурса
    instance := &certv1.CertMonitor{}
    if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }
    
    // Выполнение логики
    if err := r.reconcilePods(ctx, instance); err != nil {
        log.Error(err, "failed to reconcile pods")
        return ctrl.Result{}, err
    }
    
    // Успешное завершение
    return ctrl.Result{}, nil
}
```

### 2. Работа с ресурсами
- Всегда проверяйте существование ресурсов перед использованием
- Используйте `client.ObjectKey` для создания ключей
- Обрабатывайте `IsNotFound` ошибки отдельно
- Используйте патчи вместо полного обновления, когда возможно

```go
// Хорошо: проверка существования
pod := &v1.Pod{}
key := client.ObjectKey{Namespace: namespace, Name: name}
if err := r.Get(ctx, key, pod); err != nil {
    if apierrors.IsNotFound(err) {
        // Ресурс не существует, создаем новый
        return r.createPod(ctx, pod)
    }
    return err
}
```

### 3. Finalizers
- Используйте finalizers для очистки ресурсов
- Всегда удаляйте finalizer после завершения очистки
- Обрабатывайте удаление ресурсов с finalizer в reconcile loop

```go
// Добавление finalizer
if !controllerutil.ContainsFinalizer(instance, finalizerName) {
    controllerutil.AddFinalizer(instance, finalizerName)
    if err := r.Update(ctx, instance); err != nil {
        return err
    }
}

// Обработка удаления
if !instance.GetDeletionTimestamp().IsZero() {
    // Выполнение очистки
    if err := r.cleanup(ctx, instance); err != nil {
        return err
    }
    // Удаление finalizer
    controllerutil.RemoveFinalizer(instance, finalizerName)
    return r.Update(ctx, instance)
}
```

### 4. Логирование
- Используйте структурированное логирование через `logr` (интегрирован в controller-runtime)
- Передавайте контекст через `context.Context`
- Используйте соответствующие уровни логирования:
  - `Error` - для ошибок, требующих внимания
  - `Info` - для важных событий
  - `Debug` - для отладочной информации
- Включайте контекстную информацию (имена ресурсов, namespace, host, domain, path)
- Избегайте автоматически добавляемых полей манифеста - используйте только явные поля

#### Настройка логирования

Проект использует `zap` через `controller-runtime` с улучшенной конфигурацией:

```go
opts := zap.Options{
    Development: true,
    EncoderConfigOptions: []zap.EncoderConfigOption{
        func(config *zap.EncoderConfig) {
            // Цветная подсветка уровней логирования
            config.EncodeLevel = zap.CapitalColorLevelEncoder
            // Формат времени ISO8601
            config.EncodeTime = zap.ISO8601TimeEncoder
            // Формат caller (файл:строка)
            config.EncodeCaller = zap.ShortCallerEncoder
        },
    },
}
```

#### Уровни логирования с цветовой подсветкой

- **ERROR** (красный) - критические ошибки
- **INFO** (синий) - информационные сообщения
- **DEBUG** (желтый) - отладочная информация

#### Примеры использования

```go
// Получение логгера из контекста
logger := log.FromContext(ctx)

// Логирование с контекстной информацией
logger.Info("HTTP01 Solver Pod detected",
    "podName", pod.Name,
    "podNamespace", pod.Namespace,
    "host", domain,
    "path", "/.well-known/acme-challenge/",
)

// Логирование ошибок
logger.Error(err, "failed to create VirtualService",
    "domain", domain,
    "gateway", gateway.Name,
    "solverPod", pod.Name,
)
```

#### Типизация логов

Для лучшей идентификации типов логов используйте стандартные поля:
- `host` - домен или хост
- `path` - путь URI
- `gatewayName`, `gatewayNamespace` - информация о Gateway
- `virtualServiceName`, `virtualServiceNamespace` - информация о VirtualService
- `podName`, `podNamespace` - информация о Pod
- `certificateName`, `certificateNamespace` - информация о Certificate
- `issuerName`, `issuerNamespace` - информация о Issuer

### 5. Тестирование
- Пишите unit тесты для всей бизнес-логики
- Используйте `envtest` для тестирования контроллеров
- Мокайте внешние зависимости
- Используйте табличные тесты для множественных сценариев

```go
func TestReconcile(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(*testing.T, client.Client)
        wantErr bool
    }{
        {
            name: "successful reconciliation",
            setup: func(t *testing.T, c client.Client) {
                // Настройка тестовых данных
            },
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Выполнение теста
        })
    }
}
```

## Структура кода

### 1. Размер файлов
- **КРИТИЧЕСКОЕ ТРЕБОВАНИЕ: Файлы должны быть менее 250 строк**
- Если файл превышает 250 строк, **ОБЯЗАТЕЛЬНО** требуется рефакторинг
- Разбивайте большие файлы на меньшие, сфокусированные модули
- Каждый файл должен иметь одну четкую ответственность
- При приближении к лимиту (200+ строк) начинайте планировать разделение

```go
// Плохо: файл с 300+ строками
// certmonitor_controller.go (300 строк)
// - Reconcile
// - reconcilePods
// - findChallengePods
// - updatePodStatus
// - cleanup
// - validateResource
// - processEvents
// - handleErrors
// ... и еще много функций

// Хорошо: разделение на несколько файлов
// certmonitor_controller.go (150 строк)
// - Reconcile
// - SetupWithManager
//
// certmonitor_reconciler.go (120 строк)
// - reconcilePods
// - findChallengePods
// - updatePodStatus
//
// certmonitor_cleanup.go (80 строк)
// - cleanup
// - removeFinalizers
```

### 2. Организация пакетов
- Группируйте связанный код в пакеты
- Избегайте циклических зависимостей
- Используйте интерфейсы для абстракции

```
controllers/
  ├── certmonitor_controller.go  # Основной контроллер
  └── pod_monitor.go             # Логика мониторинга подов

internal/
  ├── client/                    # Клиенты для внешних сервисов
  └── utils/                     # Утилиты
```

### 3. Интерфейсы
- Используйте интерфейсы для тестируемости
- Определяйте интерфейсы в месте использования, а не реализации
- Делайте интерфейсы небольшими и сфокусированными

```go
// Интерфейс для мониторинга подов
type PodMonitor interface {
    IsPodReady(ctx context.Context, name string) (bool, error)
    GetPodStatus(ctx context.Context, name string) (*PodStatus, error)
}
```

### 4. Обработка зависимостей
- Используйте dependency injection
- Передавайте зависимости через конструкторы
- Избегайте глобальных переменных

```go
// Хорошо: dependency injection
type Reconciler struct {
    client    client.Client
    podMonitor PodMonitor
}

func NewReconciler(client client.Client, podMonitor PodMonitor) *Reconciler {
    return &Reconciler{
        client:     client,
        podMonitor: podMonitor,
    }
}
```

## Производительность

### 1. Кэширование
- Используйте кэширование для часто запрашиваемых данных
- Настройте правильные индексы для поиска
- Очищайте кэш при необходимости

### 2. Batch операции
- Группируйте операции, когда возможно
- Используйте `List` вместо множественных `Get` запросов

### 3. Асинхронные операции
- Используйте goroutines для независимых операций
- Управляйте жизненным циклом goroutines через context

## Безопасность

### 1. RBAC
- Соблюдайте принцип наименьших привилегий
- Запрашивайте только необходимые права
- Документируйте требуемые права

### 2. Валидация входных данных
- Всегда валидируйте входные данные
- Используйте webhooks для валидации CR
- Проверяйте права доступа перед операциями

### 3. Секреты
- Никогда не логируйте секреты
- Используйте Kubernetes Secrets для хранения чувствительных данных
- Ротация секретов должна быть автоматизирована

---

**Примечание**: Этот документ должен обновляться по мере развития проекта и появления новых практик.

