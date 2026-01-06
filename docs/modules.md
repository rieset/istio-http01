# Документация по используемым модулям

Этот документ описывает основные модули и библиотеки, используемые в проекте Istio Gas Operator.

## Содержание (Индекс)

### Основные модули
- **[Operator SDK](#operator-sdk)** - инструментарий для создания Kubernetes операторов
  - Описание, версия и установка
  - CLI инструменты и scaffolding
  - Использование в проекте
  - Преимущества
- **[Go (Golang)](#go-golang)** - язык программирования
  - Описание и особенности
  - Стандартная библиотека (context, fmt, errors)
  - Внешние зависимости (controller-runtime, k8s.io/*)

### Внешние зависимости Go
- **[sigs.k8s.io/controller-runtime](#sigsk8siocontroller-runtime)** - библиотека для контроллеров
- **[k8s.io/api](#k8sioapi)** - определения Kubernetes API ресурсов
- **[k8s.io/apimachinery](#k8sioapimachinery)** - утилиты для работы с Kubernetes API
- **[k8s.io/client-go](#k8sioclient-go)** - клиент для взаимодействия с Kubernetes API

### Управление проектом
- **[Управление зависимостями](#управление-зависимостями)** - go.mod и команды
- **[Версионирование](#версионирование)** - SemVer и Go модули
- **[Инструменты разработки](#инструменты-разработки)** - gofmt, goimports, golangci-lint, go test
- **[Рекомендации по обновлению](#рекомендации-по-обновлению)** - обновление Operator SDK, Go и зависимостей

---

## Operator SDK

### Описание
Operator SDK - это инструментарий для создания, тестирования и развертывания Kubernetes операторов. Он предоставляет высокоуровневые абстракции и инструменты для упрощения разработки операторов.

**Официальный сайт**: https://sdk.operatorframework.io/  
**Документация по установке**: https://sdk.operatorframework.io/docs/installation/  
**Репозиторий**: https://github.com/operator-framework/operator-sdk

### Версия
Рекомендуемая версия: v1.42.0 или выше

### Установка

#### macOS (Homebrew)
```bash
brew install operator-sdk
```

#### Linux/Windows
```bash
# Установка переменных окружения
export ARCH=$(case $(uname -m) in x86_64) echo -n amd64 ;; aarch64) echo -n arm64 ;; *) echo -n $(uname -m) ;; esac)
export OS=$(uname | awk '{print tolower($0)}')

# Скачивание бинарника
export OPERATOR_SDK_DL_URL=https://github.com/operator-framework/operator-sdk/releases/download/v1.42.0
curl -LO ${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH}

# Установка
chmod +x operator-sdk_${OS}_${ARCH} && sudo mv operator-sdk_${OS}_${ARCH} /usr/local/bin/operator-sdk
```

### Основные компоненты

#### 1. CLI инструменты
- `operator-sdk init` - инициализация нового проекта
- `operator-sdk create api` - создание API и контроллера
- `operator-sdk generate` - генерация кода и манифестов
- `operator-sdk build` - сборка образа оператора
- `operator-sdk run` - запуск оператора локально

#### 2. Scaffolding
Operator SDK автоматически генерирует:
- Структуру проекта
- CRD определения
- Контроллеры с базовой логикой
- RBAC манифесты
- Kustomize конфигурации

#### 3. Интеграция с controller-runtime
Operator SDK использует controller-runtime для работы с Kubernetes API.

### Использование в проекте

**Подробное руководство**: См. [Руководство по созданию Go оператора](operator-sdk-go-guide.md)

#### Инициализация проекта
```bash
operator-sdk init --domain example.com --repo github.com/example/istio-http01
```

#### Создание API
```bash
operator-sdk create api --group cert --version v1 --kind CertMonitor
```

#### Генерация кода
```bash
# Генерация CRD манифестов
operator-sdk generate kustomize manifests

# Генерация кода контроллера
operator-sdk generate
```

### Преимущества
- Автоматическая генерация boilerplate кода
- Интеграция с OLM (Operator Lifecycle Manager)
- Поддержка различных языков (Go, Ansible, Helm)
- Встроенные инструменты для тестирования
- Генерация манифестов для развертывания

## Go (Golang)

### Описание
Go - компилируемый язык программирования с открытым исходным кодом, разработанный Google. Используется как основной язык для разработки Kubernetes операторов.

**Официальный сайт**: https://go.dev/  
**Документация**: https://go.dev/doc/

### Версия
Требуемая версия: Go 1.22 или выше (в проекте используется Go 1.23+)

**Примечание**: Согласно [официальной документации Operator SDK](https://sdk.operatorframework.io/docs/building-operators/golang/installation/), минимальная версия Go - 1.22. Проект использует Go 1.23+ для доступа к новейшим возможностям языка.

### Установка
См. официальную документацию: https://go.dev/doc/install

### Основные особенности для операторов

#### 1. Статическая типизация
- Обеспечивает безопасность типов на этапе компиляции
- Упрощает рефакторинг и поддержку кода

#### 2. Простота и читаемость
- Минималистичный синтаксис
- Явная обработка ошибок
- Отсутствие неявных преобразований

#### 3. Производительность
- Компиляция в нативный код
- Эффективное использование памяти
- Быстрое время выполнения

#### 4. Concurrency
- Встроенная поддержка goroutines
- Каналы для коммуникации
- Отлично подходит для асинхронных операций

### Используемые пакеты стандартной библиотеки

#### context
- Управление жизненным циклом операций
- Отмена и таймауты
- Передача метаданных

#### fmt
- Форматирование строк
- Обработка ошибок с контекстом

#### errors
- Создание и обертывание ошибок
- Проверка типов ошибок

### Внешние зависимости

#### sigs.k8s.io/controller-runtime
Основная библиотека для создания контроллеров Kubernetes.

**Версия**: v0.18.0+

**Основные компоненты**:
- `manager.Manager` - управление контроллерами
- `controller.Controller` - базовый контроллер
- `client.Client` - клиент для работы с Kubernetes API
- `reconcile.Reconciler` - интерфейс для reconciliation

**Использование**:
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)
```

#### k8s.io/api
Определения Kubernetes API ресурсов.

**Версия**: v0.30.0+

**Использование**:
```go
import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

#### k8s.io/apimachinery
Утилиты для работы с Kubernetes API.

**Версия**: v0.30.0+

**Основные компоненты**:
- `runtime.Object` - интерфейс для Kubernetes объектов
- `meta/v1` - метаданные ресурсов
- `apis/meta/v1` - общие типы API

#### k8s.io/client-go
Клиент для взаимодействия с Kubernetes API.

**Версия**: v0.30.0+

**Использование**:
```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)
```

## Управление зависимостями

### go.mod
Файл `go.mod` определяет модуль и его зависимости.

```go
module github.com/example/istio-http01

go 1.23

require (
    sigs.k8s.io/controller-runtime v0.18.0
    k8s.io/api v0.30.0
    k8s.io/apimachinery v0.30.0
    k8s.io/client-go v0.30.0
)
```

### Команды для управления зависимостями

```bash
# Добавление новой зависимости
go get package@version

# Обновление зависимостей
go get -u ./...

# Очистка неиспользуемых зависимостей
go mod tidy

# Проверка зависимостей
go mod verify

# Загрузка зависимостей
go mod download
```

## Версионирование

### Семантическое версионирование
Проект следует семантическому версионированию (SemVer):
- `MAJOR.MINOR.PATCH`
- Пример: `v1.2.3`

### Go модули и версии
- Используется семантическое версионирование для зависимостей
- Теги версий должны начинаться с `v`
- `go.mod` автоматически управляет версиями

## Инструменты разработки

### gofmt
Форматирование кода:
```bash
gofmt -w .
```

### goimports
Управление импортами:
```bash
goimports -w .
```

### golangci-lint
Статический анализ кода:
```bash
golangci-lint run
```

### go test
Запуск тестов:
```bash
go test ./...
go test -v ./...
go test -cover ./...
```

## Рекомендации по обновлению

### Operator SDK
- Проверяйте changelog перед обновлением
- Тестируйте обновления в dev окружении
- Следуйте migration guide при major обновлениях

### Go
- Обновляйтесь до последней стабильной версии
- Проверяйте breaking changes в release notes
- Тестируйте совместимость зависимостей

### Зависимости
- Регулярно обновляйте зависимости для безопасности
- Используйте `go get -u` с осторожностью
- Тестируйте после обновления зависимостей

---

**Примечание**: Этот документ должен обновляться при изменении версий модулей или добавлении новых зависимостей.

