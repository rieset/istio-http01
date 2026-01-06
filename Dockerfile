# =============================================================================
# Stage 1: Builder - сборка приложения
# =============================================================================
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Копируем файлы зависимостей для кэширования слоя
COPY go.mod go.mod
COPY go.sum go.sum

# Загружаем зависимости (кэшируется если go.mod/go.sum не изменились)
RUN go mod download

# Копируем исходный код
COPY cmd/ cmd/
COPY internal/ internal/

# Собираем бинарник
# GOARCH не имеет значения по умолчанию, чтобы бинарник собирался согласно платформе хоста
# Например, при вызове make docker-build на Apple Silicon M1 BUILDPLATFORM будет linux/arm64,
# а для Apple x86 будет linux/amd64. Оставляя пустым, мы гарантируем, что контейнер и бинарник
# будут иметь одинаковую платформу.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager ./cmd

# =============================================================================
# Stage 2: Runtime - финальный минимальный образ только с бинарником
# =============================================================================
# Используем distroless как минимальный базовый образ для упаковки бинарника
# Подробнее: https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Копируем только собранный бинарник из стадии builder
COPY --from=builder /workspace/manager /manager

# Устанавливаем пользователя без root прав
USER 65532:65532

# Точка входа - запуск менеджера
ENTRYPOINT ["/manager"]
