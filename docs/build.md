# Инструкция по сборке образа оператора

Этот документ описывает процесс сборки Docker образа для оператора Istio Gas.

## Быстрый старт

Для сборки образа для Linux amd64 выполните следующие команды:

```bash
# 1. Сборка образа
make docker-build-amd64 IMG=example-registry/istio-http01:0.1.0

# 2. Отправка образа в registry
docker push example-registry/istio-http01:0.1.0

# 3. Установка/обновление оператора через Helm
helm upgrade --install istio-http01 ./helm/istio-http01 -n istio-system --create-namespace
```

**Или используйте одну команду Makefile:**

```bash
# Сборка, отправка и установка одной командой
make build-push-deploy IMG=example-registry/istio-http01:0.1.0
```

## Подробная инструкция

### Предварительные требования

1. **Docker** установлен и запущен
2. **Docker BuildKit** включен:
   ```bash
   export DOCKER_BUILDKIT=1
   ```
   Или включите BuildKit в настройках Docker Desktop

3. **Make** установлен (обычно предустановлен на macOS/Linux)

### Сборка образа для Linux amd64

Используйте специальную цель Makefile для сборки образа под Linux amd64:

```bash
make docker-build-amd64 IMG=example-registry/istio-http01:0.1.0
```

Эта команда:
- Собирает образ для платформы `linux/amd64`
- Передает правильные build-args (`TARGETOS=linux`, `TARGETARCH=amd64`)
- Использует флаг `--platform linux/amd64` для Docker

### Отправка образа в registry

После успешной сборки отправьте образ в ваш Docker registry:

```bash
docker push example-registry/istio-http01:0.1.0
```

**Важно**: Убедитесь, что вы авторизованы в registry:
```bash
docker login your-registry.com
```

### Полная последовательность команд

```bash
# 1. Включить BuildKit (если еще не включен)
export DOCKER_BUILDKIT=1

# 2. Сборка образа для Linux amd64
make docker-build-amd64 IMG=example-registry/istio-http01:0.1.0

# 3. Проверка образа (опционально)
docker images | grep example-registry/istio-http01

# 4. Отправка образа в registry
docker push example-registry/istio-http01:0.1.0

# 5. Установка/обновление оператора через Helm
helm upgrade --install istio-http01 ./helm/istio-http01 -n istio-system --create-namespace
```

**Альтернатива: использование Makefile команд**

```bash
# Сборка и отправка образа
make docker-build-push IMG=example-registry/istio-http01:0.1.0

# Или все сразу: сборка, отправка и установка
make build-push-deploy IMG=example-registry/istio-http01:0.1.0
```

## Альтернативные варианты сборки

### Сборка для текущей платформы

Если вы собираете образ на той же платформе, где будете его использовать:

```bash
make docker-build IMG=example-registry/istio-http01:0.1.0
```

### Сборка для нескольких платформ (multi-arch)

Для сборки образа, поддерживающего несколько архитектур:

```bash
# Создать builder (один раз)
docker buildx create --name istio-http01-builder --use

# Собрать для нескольких платформ
make docker-buildx IMG=example-registry/istio-http01:0.1.0 PLATFORMS=linux/amd64,linux/arm64
```

Или напрямую через docker buildx:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t example-registry/istio-http01:0.1.0 \
  --push .
```

### Прямой вызов docker build

Если нужно собрать образ без Makefile:

```bash
docker build --platform linux/amd64 \
  -t example-registry/istio-http01:0.1.0 \
  --build-arg TARGETOS=linux \
  --build-arg TARGETARCH=amd64 \
  .
```

## Проверка образа

После сборки можно проверить платформу образа:

```bash
docker inspect example-registry/istio-http01:0.1.0 | grep -A 5 "Architecture"
```

Или запустить тестовый контейнер:

```bash
docker run --rm example-registry/istio-http01:0.1.0 --version
```

## Обновление версии

При обновлении версии образа:

1. Обновите тег в команде сборки:
   ```bash
   make docker-build-amd64 IMG=example-registry/istio-http01:0.2.0
   ```

2. Обновите версию в `helm/istio-http01/values.yaml`:
   ```yaml
   image:
     tag: "0.2.0"
   ```

3. Отправьте новый образ:
   ```bash
   docker push example-registry/istio-http01:0.2.0
   ```

## Устранение проблем

### Ошибка: "buildx is not available"

Установите Docker Buildx или используйте обычную сборку:
```bash
make docker-build IMG=example-registry/istio-http01:0.1.0
```

### Ошибка: "failed to solve"

Убедитесь, что:
- Docker BuildKit включен: `export DOCKER_BUILDKIT=1`
- Все зависимости загружены: `go mod download`
- Dockerfile находится в корне проекта

### Медленная сборка на macOS с Apple Silicon

При сборке образа для amd64 на Apple Silicon используется эмуляция, что может быть медленнее. Это нормально.

Для ускорения можно использовать:
- Docker Desktop с включенной опцией "Use Rosetta for x86/amd64 emulation"
- Или собрать образ на Linux машине с amd64

## См. также

- [INSTALL.md](../INSTALL.md) - Полная инструкция по установке
- [README.md](../README.md) - Общая информация о проекте

