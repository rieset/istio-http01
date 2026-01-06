# Istio HTTP01 Operator

Kubernetes operator для мониторинга подов cert-manager, созданных для проверки домена через Let's Encrypt по методу HTTP01 challenge.

## Описание

Этот оператор отслеживает созданные cert-manager поды, которые используются для HTTP01 challenge при получении SSL/TLS сертификатов от Let's Encrypt. Оператор обеспечивает мониторинг состояния этих подов и может выполнять дополнительные действия для обеспечения корректной работы процесса валидации домена.

## Требования

- Kubernetes кластер (версия 1.19+)
- cert-manager установлен в кластере
- Helm 3.0+ (для установки оператора)

## Развертывание

### Получение списка доступных версий

Перед установкой определите доступную версию чарта:

```bash
# Получение списка всех доступных версий через Helm
helm search repo oci://ghcr.io/rieset/helm-charts/istio-http01 --versions

# Или проверьте доступные версии на странице GitHub Packages:
# https://github.com/rieset?tab=packages&repo_name=istio-http01

# Или используйте GitHub API (требует аутентификации для приватных репозиториев):
# curl -H "Authorization: token YOUR_TOKEN" https://api.github.com/orgs/rieset/packages/container/istio-http01/versions
```

**⚠️ Важно:** Замените `<version>` в командах ниже на актуальную версию из списка выше.

### Через Helm из GitHub Packages (рекомендуется)

Helm chart публикуется в GitHub Packages (OCI registry) при создании тегов версий (например, `v<version>`).

**⚠️ Важно:** Chart публикуется автоматически только при создании тега версии. Убедитесь, что нужная версия существует.

**Установка из GitHub Packages:**

```bash
# Замените <version> на актуальную версию из списка выше
# ⚠️ ВАЖНО: Версия образа должна совпадать с версией чарта
helm install istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version <version> \
  -n istio-system \
  --create-namespace \
  --set image.repository=rieset/istio-http01 \
  --set image.tag=<version>
```

**Примечание:** Версия Docker образа (`image.tag`) должна совпадать с версией Helm чарта (`--version`). Это гарантирует, что будет использован правильный образ для данной версии оператора.

**Обновление:**

```bash
# Обновление до новой версии (замените <version> на нужную версию)
# ⚠️ ВАЖНО: Версия образа должна совпадать с версией чарта
helm upgrade istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version <version> \
  -n istio-system \
  --set image.repository=rieset/istio-http01 \
  --set image.tag=<version>
```

**Просмотр информации о chart:**

```bash
# Просмотр метаданных chart
helm show chart oci://ghcr.io/rieset/helm-charts/istio-http01 --version <version>

# Просмотр всех значений по умолчанию
helm show values oci://ghcr.io/rieset/helm-charts/istio-http01 --version <version>
```

**Аутентификация (для приватных репозиториев или если chart не найден):**

Если chart не найден или репозиторий приватный, может потребоваться аутентификация:

```bash
# Создайте Personal Access Token (PAT) в GitHub с правами read:packages
# Settings -> Developer settings -> Personal access tokens -> Tokens (classic)
# Затем выполните:
helm registry login ghcr.io -u YOUR_GITHUB_USERNAME
# Введите PAT при запросе пароля
```

**Если chart не найден:**

1. Убедитесь, что тег версии был создан (например, `v<version>`)
2. Проверьте, что workflow `build-helm-chart.yml` успешно выполнился
3. Проверьте доступные версии на странице GitHub Packages
4. Если chart еще не опубликован, используйте локальную установку (см. ниже)

### Через Helm из локального chart

```bash
# Обновить образ в values.yaml
# Затем установить
helm install istio-http01 ./helm/istio-http01 -n istio-system --create-namespace
```

### Через Makefile

При использовании Makefile версия автоматически синхронизируется между Docker образом и Helm чартом:

```bash
# Установить версию (по умолчанию используется VERSION=0.1.0 из Makefile)
export VERSION=0.1.0

# Собрать образ, отправить в registry и развернуть через Helm
# Версия образа и чарта будет автоматически синхронизирована
make build-push-deploy IMG=rieset/istio-http01:$(VERSION)
```

Или используйте переменную VERSION напрямую:

```bash
# Сборка и деплой с указанной версией
make build-push-deploy VERSION=0.1.0 IMG=rieset/istio-http01:0.1.0
```

**Примечание:** Команда `make deploy-helm` автоматически обновляет `Chart.yaml` и `values.yaml` с версией из переменной `VERSION` перед деплоем.

Подробная инструкция по установке: [INSTALL.md](INSTALL.md)

## Случай использования

Оператор разработан для работы в следующей архитектуре:

### Архитектура

- **Несколько Gateway в разных namespace**: В кластере может быть несколько Istio Gateway, каждый в своем namespace (например, `example-gateway-alpha`, `example-gateway-gamma` и т.д.)

- **Общий IP для Ingress**: Все Gateway привязаны к одному внешнему IP адресу через Istio Ingress Gateway

- **Hosts в Gateway для внутренней маршрутизации**: Поле `hosts` в Gateway используется для внутренней маршрутизации в service mesh и содержит namespace или внутренние имена, а **не** внешние домены

- **Внешние домены через VirtualService**: Внешние домены определяются через VirtualService ресурсы, которые связаны с Gateway через поле `spec.gateways`

- **Непересекающиеся домены**: Домены для разных namespace не пересекаются (например, `app-alpha.example.com` и `app-gamma.example.com`)

- **Размещение оператора**: Оператор должен быть установлен в том же namespace, что и Istio (обычно `istio-system`)

### Как это работает

1. **cert-manager создает HTTP01 solver под** для валидации домена (например, `cm-acme-http-solver-abc123`)

2. **Оператор обнаруживает под** и извлекает домен из аргументов контейнера

3. **Оператор ищет Gateway** для этого домена:
   - Просматривает все VirtualService в кластере
   - Находит VirtualService, который содержит нужный домен в `spec.hosts`
   - Определяет, какой Gateway связан с этим VirtualService через `spec.gateways`
   - **Важно**: Оператор НЕ использует поле `hosts` в Gateway для определения домена, так как оно содержит внутренние данные

4. **Оператор создает VirtualService** для HTTP01 challenge:
   - VirtualService создается в namespace Gateway (не в namespace пода)
   - Маршрутизирует трафик `/.well-known/acme-challenge/*` на под HTTP01 solver
   - Связан с найденным Gateway

5. **Автоматическая очистка**: Оператор периодически проверяет и удаляет VirtualService, которые ссылаются на несуществующие поды или сервисы

### Пример конфигурации

```yaml
# Gateway в namespace example-gateway-alpha
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: example-gateway
  namespace: example-gateway-alpha
  labels:
    app.kubernetes.io/managed-by: Helm  # Пример метки от Helm
spec:
  selector:
    istio: ingressgateway  # Селектор для выбора Istio Ingress Gateway
  servers:
    # HTTP сервер (порт 80)
    - hosts:
        # ВАЖНО: hosts в Gateway используются для внутренней маршрутизации
        # Это НЕ внешние домены, а внутренние имена для service mesh
        # (например, namespace или внутренние сервисы)
        - example-app-ns-1/*      # Внутреннее имя namespace 1
        - example-app-ns-2/*     # Внутреннее имя namespace 2
        - example-gateway-alpha/*  # Внутреннее имя namespace Gateway
      name: http-example-gateway
      port:
        name: http-example-gateway # Уникальное имя для каждого неймспейса
        number: 80
        protocol: HTTP2
      tls:
        httpsRedirect: true  # перенаправлять на HTTPS
    # HTTPS сервер (порт 443)
    - hosts:
        # Те же внутренние имена для HTTPS
        - example-app-ns-1/*
        - example-app-ns-2/*
        - example-gateway-alpha/*
      name: https-example-gateway
      port:
        name: https-example-gateway # Уникальное имя для каждого неймспейса
        number: 443
        protocol: HTTPS
      tls:
        credentialName: example-gateway-cert-secret  # Имя секрета с TLS сертификатом
        mode: SIMPLE  # Режим TLS (SIMPLE, MUTUAL, ISTIO_MUTUAL)
---
# VirtualService для внешнего домена
# ВАЖНО: Внешние домены определяются через VirtualService, а не через Gateway
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: app-alpha-vs
  namespace: example-app-ns-1  # VirtualService в том же namespace, что и Gateway
spec:
  gateways:
    # Связь с Gateway (может быть указан как "namespace/name" или просто "name")
    - example-gateway-alpha/example-gateway
  hosts:
    # ВАЖНО: Здесь указываются ВНЕШНИЕ домены для сертификатов
    # Оператор использует эти домены для поиска Gateway
    - app-alpha.example.com  # Внешний домен для приложения
  http:
    - route:
        - destination:
            host: app-alpha-service  # Внутренний сервис
            port:
              number: 8080
---
# Certificate для создания TLS сертификата
# ВАЖНО: Certificate создает секрет (secretName), который используется в Gateway (credentialName)
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-gateway-cert
  namespace: istio-system  # Certificate обычно создается в namespace Istio
  labels:
    app.kubernetes.io/managed-by: Helm  # Пример метки от Helm
spec:
  # DNS имена, для которых выдается сертификат
  # ВАЖНО: Эти домены должны совпадать с доменами в VirtualService
  dnsNames:
    - app-alpha.example.com      # Основной домен
    - app.alpha.example.com      # Поддомен
  # Имя секрета, который будет создан cert-manager
  # ВАЖНО: Это имя должно совпадать с credentialName в Gateway
  secretName: example-gateway-cert-secret
  # Ссылка на Issuer, который будет выдавать сертификат
  issuerRef:
    group: cert-manager.io
    kind: Issuer
    name: example-issuer  # Имя Issuer ресурса
  # Дополнительные настройки сертификата
  duration: 2160h  # Срок действия сертификата (90 дней)
  renewBefore: 360h  # Обновить за 15 дней до истечения
  privateKey:
    algorithm: RSA
    size: 2048
    rotationPolicy: Always  # Всегда ротировать ключ при обновлении
  # Дополнительный формат вывода (опционально)
  additionalOutputFormats:
    - type: CombinedPEM  # Объединенный формат PEM
  # Информация о субъекте сертификата (опционально)
  subject:
    organizations:
      - example.com  # Организация
```

**Связь между ресурсами:**

1. **Certificate** создает секрет `example-gateway-cert-secret` с TLS сертификатом
2. **Gateway** использует этот секрет через `credentialName: example-gateway-cert-secret` в HTTPS сервере
3. **VirtualService** определяет внешние домены (`app-alpha.example.com`), которые должны совпадать с `dnsNames` в Certificate
4. **Оператор** находит Gateway для домена через VirtualService и создает временный VirtualService для HTTP01 challenge

### ⚠️ Важные требования к конфигурации Gateway

**КРИТИЧЕСКИ ВАЖНО** при настройке Gateway для корректной работы оператора:

1. **Уникальные имена портов для каждого namespace**:
   - Имя порта (`spec.servers[].port.name`) должно быть уникальным для каждого namespace Gateway
   - Это необходимо для правильного построения маршрутов в Istio
   - Пример: `http-example-gateway-alpha`, `https-example-gateway-alpha` для namespace `example-gateway-alpha`
   - ❌ **НЕ используйте одинаковые имена портов** в разных namespace, даже если это разные Gateway

2. **Правильное заполнение hosts**:
   - В поле `spec.servers[].hosts` должны быть перечислены **все namespace приложений**, которые относятся к этому Gateway
   - В `hosts` **обязательно должен быть указан namespace самого Gateway**
   - Формат: `namespace-name/*` (например, `example-app-ns-1/*`, `example-gateway-alpha/*`)
   - Пример правильной конфигурации:
     ```yaml
     hosts:
       - example-app-ns-1/*      # Namespace приложения 1
       - example-app-ns-2/*      # Namespace приложения 2
       - example-gateway-alpha/*  # Namespace самого Gateway (обязательно!)
     ```

3. **Непересекающиеся namespace**:
   - Namespace, указанные в `hosts` разных Gateway, **не должны пересекаться**
   - Каждый namespace приложения должен быть связан только с одним Gateway
   - Это гарантирует правильную маршрутизацию и предотвращает конфликты

**Пример неправильной конфигурации:**
```yaml
# Gateway 1 в namespace example-gateway-alpha
hosts:
  - example-app-ns-1/*
  - example-gateway-alpha/*

# Gateway 2 в namespace example-gateway-gamma
hosts:
  - example-app-ns-1/*  # ❌ ОШИБКА: namespace пересекается с Gateway 1!
  - example-gateway-gamma/*
```

**Пример правильной конфигурации:**
```yaml
# Gateway 1 в namespace example-gateway-alpha
hosts:
  - example-app-ns-1/*      # Namespace приложения 1
  - example-app-ns-2/*      # Namespace приложения 2
  - example-gateway-alpha/*  # Namespace Gateway (обязательно!)

# Gateway 2 в namespace example-gateway-gamma
hosts:
  - example-app-ns-3/*      # Namespace приложения 3 (не пересекается!)
  - example-app-ns-4/*      # Namespace приложения 4 (не пересекается!)
  - example-gateway-gamma/*  # Namespace Gateway (обязательно!)
```

### Важные особенности

- **Оператор определяет Gateway по доменам из VirtualService**, а не по `hosts` в Gateway
- **VirtualService создаются в namespace Gateway**, что позволяет изолировать конфигурацию по namespace
- **Оператор автоматически очищает устаревшие VirtualService** после успешной валидации домена
- **Поддержка cross-namespace**: Оператор корректно работает, когда Gateway и поды находятся в разных namespace

## Технологии

- **Go** - основной язык разработки
- **Operator SDK** - фреймворк для создания Kubernetes операторов
  - Документация: https://sdk.operatorframework.io/docs/installation/
  - Версия: v1.42.0+

## Установка Operator SDK

### macOS (Homebrew)
```bash
brew install operator-sdk
```

### Linux/Windows
См. официальную документацию: https://sdk.operatorframework.io/docs/installation/

## Структура проекта

```
.
├── .cursorrules          # Правила для AI ассистента
├── README.md             # Этот файл
├── docs/                         # Документация
│   ├── code-index.md             # Индекс функций кода
│   ├── operator-sdk-go-guide.md   # Руководство по созданию оператора
│   ├── glossary.md               # Глоссарий терминов
│   ├── best-practices.md         # Best practices для Go
│   ├── modules.md                # Документация по модулям
│   ├── cert-manager-pods.md      # Структура cert-manager HTTP01 подов
│   └── cert-manager-pod-example.yaml  # Пример пода cert-manager
├── cmd/                  # Точка входа приложения
│   └── main.go          # Главный файл оператора
├── internal/             # Внутренние пакеты
│   └── controller/       # Контроллеры оператора
├── api/                  # API определения (CRDs)
├── controllers/          # Реализация контроллеров (legacy)
├── test/                 # Тесты
│   ├── utils/            # Утилиты для тестов
│   └── e2e/              # End-to-end тесты
├── helm/                 # Helm chart для установки
│   └── istio-http01/        # Chart оператора
└── config/               # Kustomize конфигурации
```

## Документация

Подробная документация находится в директории `docs/`:
- [Инструкция по сборке образа](docs/build.md) - как собрать и отправить Docker образ
- [Индекс функций кода](docs/code-index.md) - полный индекс всех функций проекта
- [Руководство по созданию Go оператора с Operator SDK](docs/operator-sdk-go-guide.md) - пошаговое руководство
- [Отслеживание HTTP01 Solver Pods](docs/http01-solver-tracking.md) - как оператор отслеживает поды и проверяет VirtualService
- [Логирование](docs/logging.md) - система логирования с цветной подсветкой
- [Глоссарий терминов](docs/glossary.md)
- [Best Practices для Go](docs/best-practices.md)
- [Документация по модулям](docs/modules.md)
- [Структура cert-manager HTTP01 подов](docs/cert-manager-pods.md)
- [Пример пода cert-manager](docs/cert-manager-pod-example.yaml)

## Лицензия

Этот проект лицензирован под лицензией MIT. См. файл [LICENSE](LICENSE) для подробностей.

## Авторы

- **Albert Iblyaminov** - создатель и основной разработчик

