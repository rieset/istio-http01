# Логирование в Istio Gas Operator

Этот документ описывает систему логирования, используемую в проекте Istio Gas Operator.

## Обзор

Проект использует структурированное логирование через библиотеку `zap` (go.uber.org/zap), интегрированную в `controller-runtime`. Логирование настроено для удобной разработки с цветной подсветкой уровней и читаемым форматом вывода.

## Конфигурация

### Настройка в `cmd/main.go`

```go
import (
    "go.uber.org/zap/zapcore"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

opts := zap.Options{
    Development: true,
    EncoderConfigOptions: []zap.EncoderConfigOption{
        func(config *zapcore.EncoderConfig) {
            // Цветная подсветка уровней логирования
            config.EncodeLevel = zapcore.CapitalColorLevelEncoder
            // Формат времени ISO8601
            config.EncodeTime = zapcore.ISO8601TimeEncoder
            // Формат caller (файл:строка)
            config.EncodeCaller = zapcore.ShortCallerEncoder
        },
    },
}
```

### Флаги командной строки

Zap предоставляет следующие флаги для настройки логирования:

- `--zap-devel` - включить development режим (цветная подсветка, читаемый формат)
- `--zap-encoder` - формат энкодера (`json` или `console`)
- `--zap-log-level` - минимальный уровень логирования (`debug`, `info`, `warn`, `error`)
- `--zap-stacktrace-level` - уровень для вывода stack trace (`error`, `panic`, `fatal`)

## Уровни логирования

### ERROR (красный)
Используется для критических ошибок, требующих немедленного внимания.

```go
logger.Error(err, "failed to create VirtualService",
    "domain", domain,
    "gateway", gateway.Name,
)
```

### INFO (синий)
Используется для важных событий и информационных сообщений.

```go
logger.Info("HTTP01 Solver Pod detected",
    "podName", pod.Name,
    "podNamespace", pod.Namespace,
    "host", domain,
)
```

### DEBUG (желтый)
Используется для отладочной информации (включается через `--zap-log-level=debug`).

```go
logger.V(1).Info("Processing HTTP01 challenge",
    "podName", pod.Name,
    "domain", domain,
)
```

## Типизация логов

Для лучшей идентификации и фильтрации логов используются стандартные поля:

### Поля для ресурсов

- `name`, `namespace` - базовые поля для Gateway
- `gatewayName`, `gatewayNamespace` - информация о Gateway
- `virtualServiceName`, `virtualServiceNamespace` - информация о VirtualService
- `podName`, `podNamespace` - информация о Pod
- `certificateName`, `certificateNamespace` - информация о Certificate
- `issuerName`, `issuerNamespace` - информация о Issuer

### Поля для сетевых ресурсов

- `host` - домен или хост (например, `"app.example.com"`)
- `path` - путь URI (например, `"/.well-known/acme-challenge/"`)
- `uri` - полная информация о URI match (например, `"prefix:/.well-known/acme-challenge/"`)
- `destinationHost` - хост назначения
- `destinationPort` - порт назначения

### Поля для HTTP01 Solver

- `domain` - домен для валидации
- `solverPod` - имя пода HTTP01 solver
- `solverService` - имя Service для HTTP01 solver
- `solverPort` - порт HTTP01 solver

## Примеры логов

### Gateway обнаружен

```
INFO    Istio Gateway detected    {"name": "example-gateway", "namespace": "example-gateway-ns"}
```

### Gateway server с host

```
INFO    Gateway server    {"name": "example-gateway", "namespace": "example-gateway-ns", "index": 0, "port": 80, "protocol": "HTTP", "host": "*"}
```

### HTTP01 Solver Pod обнаружен

```
INFO    HTTP01 Solver Pod detected    {"podName": "cm-acme-http-solver-abc123", "podNamespace": "istio-system", "host": "app.example.com", "path": "/.well-known/acme-challenge/"}
```

### VirtualService создан

```
INFO    Created VirtualService for HTTP01 solver    {"virtualService": "http01-solver-app-example-com", "namespace": "istio-system", "host": "app.example.com", "path": "/.well-known/acme-challenge/", "gateway": "example-gateway", "solverPod": "cm-acme-http-solver-abc123"}
```

## Фильтрация логов

### По типу ресурса

```bash
# Логи Gateway
kubectl logs -n istio-system deployment/istio-http01 | grep "Gateway"

# Логи HTTP01 Solver Pod
kubectl logs -n istio-system deployment/istio-http01 | grep "HTTP01 Solver Pod"

# Логи VirtualService
kubectl logs -n istio-system deployment/istio-http01 | grep "VirtualService"
```

### По host/domain

```bash
# Логи для конкретного домена
kubectl logs -n istio-system deployment/istio-http01 | grep "host.*app.example.com"
```

### По уровню

```bash
# Только ошибки
kubectl logs -n istio-system deployment/istio-http01 | grep "ERROR"

# Только информационные сообщения
kubectl logs -n istio-system deployment/istio-http01 | grep "INFO"
```

## Production режим

Для production рекомендуется использовать JSON формат:

```bash
# Запуск с JSON форматом
./manager --zap-encoder=json --zap-log-level=info
```

JSON формат удобен для парсинга системами мониторинга и логирования (ELK, Loki, etc.).

## Best Practices

1. **Всегда включайте контекстную информацию**: имена ресурсов, namespace, host, domain
2. **Используйте правильные уровни**: Error для ошибок, Info для важных событий
3. **Избегайте дублирования**: не логируйте автоматически добавляемые поля манифеста
4. **Типизируйте логи**: используйте стандартные поля для лучшей фильтрации
5. **Не логируйте секреты**: никогда не выводите в логи чувствительные данные

## См. также

- [Best Practices для логирования](./best-practices.md#4-логирование)
- [Zap Documentation](https://pkg.go.dev/go.uber.org/zap)
- [Controller Runtime Logging](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/log)

