# Индекс функций кода проекта

Этот документ содержит полный индекс всех функций, созданных Operator SDK и добавленных в проект. Функции организованы по файлам и модулям для удобной навигации.

## Структура документа

- [cmd/main.go](#cmdmaingo) - Точка входа приложения
- [internal/controller/](#internalcontroller) - Контроллеры оператора
  - [setup.go](#internalcontrollersetupgo) - Настройка контроллеров
  - [certificate_controller.go](#internalcontrollercertificate_controllergo) - Контроллер Certificate
  - [http01_solver_pod_controller.go](#internalcontrollerhttp01_solver_pod_controllergo) - Контроллер HTTP01 solver подов
  - [issuer_controller.go](#internalcontrollerissuer_controllergo) - Контроллер Issuer
  - [gateway_controller.go](#internalcontrollergateway_controllergo) - Контроллер Istio Gateway
- [test/utils/utils.go](#testutilsutilsgo) - Утилиты для тестирования
- [test/e2e/e2e_test.go](#teste2ee2e_testgo) - End-to-end тесты
- [test/e2e/e2e_suite_test.go](#teste2ee2e_suite_testgo) - Настройка e2e тестового окружения

---

## internal/controller/

**Описание**: Пакет содержит все контроллеры оператора для мониторинга различных ресурсов Kubernetes.

### setup.go

**Описание**: Настраивает и регистрирует все контроллеры оператора.

#### Функции

##### `SetupControllers(mgr ctrl.Manager) error`
- **Описание**: Настраивает все контроллеры оператора и регистрирует их в менеджере
- **Параметры**: 
  - `mgr ctrl.Manager` - менеджер контроллеров
- **Возвращает**: 
  - `error` - ошибка настройки
- **Регистрируемые контроллеры**:
  - CertificateReconciler
  - HTTP01SolverPodReconciler
  - IssuerReconciler
  - GatewayReconciler

### certificate_controller.go

**Описание**: Контроллер для мониторинга Certificate ресурсов cert-manager в своем namespace.

#### Типы

##### `CertificateReconciler`
- **Описание**: Структура контроллера для Certificate ресурсов
- **Поля**:
  - `Client client.Client` - Kubernetes клиент
  - `Scheme *runtime.Scheme` - runtime схема
  - `DebugMode bool` - режим отладки (задерживает восстановление сертификата на 5 минут)

#### Функции

##### `(r *CertificateReconciler) Reconcile(ctx, req) (ctrl.Result, error)`
- **Описание**: Обрабатывает изменения Certificate ресурсов и выводит информацию в логи
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `req ctrl.Request` - запрос на reconciliation
- **Возвращает**: 
  - `ctrl.Result` - результат reconciliation
  - `error` - ошибка обработки
- **Логируемая информация**:
  - Имя и namespace Certificate
  - DNS names
  - Issuer reference
  - Secret name
  - Условия статуса

##### `(r *CertificateReconciler) SetupWithManager(mgr) error`
- **Описание**: Настраивает контроллер для работы с менеджером
- **Параметры**: 
  - `mgr ctrl.Manager` - менеджер контроллеров
- **Возвращает**: 
  - `error` - ошибка настройки

##### `(r *CertificateReconciler) isCertificateReady(cert) bool`
- **Описание**: Проверяет, готов ли Certificate (выпущен ли сертификат)
- **Параметры**: 
  - `cert *certmanagerv1.Certificate` - Certificate ресурс
- **Возвращает**: 
  - `bool` - true если сертификат готов

##### `(r *CertificateReconciler) findGatewaysUsingCertificate(ctx, secretName, secretNamespace) ([]*Gateway, error)`
- **Описание**: Находит все Gateway, которые используют указанный сертификат (оригинальный или временный)
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `secretName string` - имя секрета
  - `secretNamespace string` - namespace секрета
- **Возвращает**: 
  - `[]*istionetworkingv1beta1.Gateway` - список Gateway
  - `error` - ошибка поиска

##### `(r *CertificateReconciler) hasHTTPSRedirect(gateway) bool`
- **Описание**: Проверяет, включен ли httpsRedirect в Gateway
- **Параметры**: 
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `bool` - true если httpsRedirect включен

##### `(r *CertificateReconciler) createSelfSignedCertificate(ctx, cert, gateway) error`
- **Описание**: Создает самоподписанный сертификат для Gateway когда основной не готов
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `cert *certmanagerv1.Certificate` - оригинальный Certificate
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `error` - ошибка создания

##### `(r *CertificateReconciler) updateGatewayWithTemporarySecret(ctx, gateway, cert, originalSecretName, tempSecretName, secretNamespace) error`
- **Описание**: Обновляет Gateway для использования временного секрета и отключает HSTS
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `cert *certmanagerv1.Certificate` - Certificate ресурс
  - `originalSecretName string` - имя оригинального секрета
  - `tempSecretName string` - имя временного секрета
  - `secretNamespace string` - namespace секретов
- **Возвращает**: 
  - `error` - ошибка обновления

##### `(r *CertificateReconciler) restoreGatewayOriginalSecret(ctx, gateway, originalSecretName, secretNamespace) error`
- **Описание**: Восстанавливает оригинальный секрет в Gateway и включает обратно HSTS
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `originalSecretName string` - имя оригинального секрета
  - `secretNamespace string` - namespace секрета
- **Возвращает**: 
  - `error` - ошибка восстановления

##### `(r *CertificateReconciler) deleteTemporarySelfSignedCertificate(ctx, cert) error`
- **Описание**: Удаляет временный самоподписанный сертификат и issuer
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `cert *certmanagerv1.Certificate` - оригинальный Certificate
- **Возвращает**: 
  - `error` - ошибка удаления

##### `(r *CertificateReconciler) createEnvoyFilterToDisableHSTS(ctx, gateway, originalSecretName) error`
- **Описание**: Создает EnvoyFilter для отключения HSTS заголовка
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `originalSecretName string` - имя оригинального секрета
- **Возвращает**: 
  - `error` - ошибка создания

##### `(r *CertificateReconciler) deleteEnvoyFilterForHSTS(ctx, gateway, originalSecretName) error`
- **Описание**: Удаляет EnvoyFilter для отключения HSTS
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `originalSecretName string` - имя оригинального секрета
- **Возвращает**: 
  - `error` - ошибка удаления

##### `(r *CertificateReconciler) ensureTemporaryCertificateSetup(ctx, cert, gateway) error`
- **Описание**: Проверяет и восстанавливает состояние временного сертификата, httpRedirect и EnvoyFilter
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `cert *certmanagerv1.Certificate` - Certificate ресурс
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `error` - ошибка проверки/восстановления

##### `(r *CertificateReconciler) getDomainsForGateway(ctx, gateway) ([]string, error)`
- **Описание**: Получает список доменов для Gateway из связанных VirtualService
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `[]string` - список доменов
  - `error` - ошибка получения

##### `(r *CertificateReconciler) getIngressGatewayIP(ctx, gateway) (string, error)`
- **Описание**: Получает IP адрес ingress gateway для Gateway
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `string` - IP адрес
  - `error` - ошибка получения

##### `(r *CertificateReconciler) verifyCertificateViaHTTPS(ctx, gateway, expectedDNSNames, ingressIP) error`
- **Описание**: Проверяет сертификат через HTTPS запрос и сравнивает домены
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `expectedDNSNames []string` - ожидаемые DNS имена
  - `ingressIP string` - IP адрес ingress gateway
- **Возвращает**: 
  - `error` - ошибка проверки

##### `(r *CertificateReconciler) verifyCertificateViaHTTP(ctx, gateway, ingressIP) error`
- **Описание**: Проверяет доступность через HTTP запрос
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
  - `ingressIP string` - IP адрес ingress gateway
- **Возвращает**: 
  - `error` - ошибка проверки

##### `(r *CertificateReconciler) findCertificateBySecretName(ctx, secretName, secretNamespace) *Certificate`
- **Описание**: Находит Certificate по имени секрета
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `secretName string` - имя секрета
  - `secretNamespace string` - namespace секрета
- **Возвращает**: 
  - `*certmanagerv1.Certificate` - найденный Certificate или nil

### http01_solver_pod_controller.go

**Описание**: Контроллер для мониторинга подов `cm-acme-http-solver-*` в своем namespace.

#### Типы

##### `HTTP01SolverPodReconciler`
- **Описание**: Структура контроллера для HTTP01 solver подов
- **Поля**:
  - `Client client.Client` - Kubernetes клиент
  - `Scheme *runtime.Scheme` - runtime схема

#### Функции

##### `(r *HTTP01SolverPodReconciler) Reconcile(ctx, req) (ctrl.Result, error)`
- **Описание**: Обрабатывает изменения HTTP01 solver подов и выводит информацию в логи
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `req ctrl.Request` - запрос на reconciliation
- **Возвращает**: 
  - `ctrl.Result` - результат reconciliation
  - `error` - ошибка обработки
- **Логируемая информация**:
  - Имя и namespace пода
  - Фаза и IP адреса
  - Метки (http-domain, http-token)
  - Информация о контейнере acmesolver
  - Домен из аргументов контейнера
  - Статус готовности
  - Owner references

##### `(r *HTTP01SolverPodReconciler) SetupWithManager(mgr) error`
- **Описание**: Настраивает контроллер с предикатом для фильтрации только HTTP01 solver подов
- **Параметры**: 
  - `mgr ctrl.Manager` - менеджер контроллеров
- **Возвращает**: 
  - `error` - ошибка настройки
- **Особенности**: Использует предикат для фильтрации подов по имени и метке

### issuer_controller.go

**Описание**: Контроллер для мониторинга Issuer ресурсов cert-manager в своем namespace.

#### Типы

##### `IssuerReconciler`
- **Описание**: Структура контроллера для Issuer ресурсов
- **Поля**:
  - `Client client.Client` - Kubernetes клиент
  - `Scheme *runtime.Scheme` - runtime схема

#### Функции

##### `(r *IssuerReconciler) Reconcile(ctx, req) (ctrl.Result, error)`
- **Описание**: Обрабатывает изменения Issuer ресурсов и выводит информацию в логи
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `req ctrl.Request` - запрос на reconciliation
- **Возвращает**: 
  - `ctrl.Result` - результат reconciliation
  - `error` - ошибка обработки
- **Логируемая информация**:
  - Имя и namespace Issuer
  - Тип Issuer (ACME, SelfSigned, CA, Vault)
  - Конфигурация ACME (server, email, solvers)
  - Условия статуса

##### `(r *IssuerReconciler) SetupWithManager(mgr) error`
- **Описание**: Настраивает контроллер для работы с менеджером
- **Параметры**: 
  - `mgr ctrl.Manager` - менеджер контроллеров
- **Возвращает**: 
  - `error` - ошибка настройки

### gateway_controller.go

**Описание**: Контроллер для мониторинга Istio Gateway ресурсов во всех namespace и связанных VirtualService.

#### Типы

##### `GatewayReconciler`
- **Описание**: Структура контроллера для Istio Gateway ресурсов
- **Поля**:
  - `Client client.Client` - Kubernetes клиент
  - `Scheme *runtime.Scheme` - runtime схема

#### Функции

##### `(r *GatewayReconciler) Reconcile(ctx, req) (ctrl.Result, error)`
- **Описание**: Обрабатывает изменения Istio Gateway ресурсов и выводит информацию в логи
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `req ctrl.Request` - запрос на reconciliation
- **Возвращает**: 
  - `ctrl.Result` - результат reconciliation
  - `error` - ошибка обработки
- **Логируемая информация**:
  - Имя и namespace Gateway
  - Информация о серверах (порты, протоколы, хосты)
  - TLS конфигурация
  - Связанные VirtualService
  - Домены из VirtualService

##### `(r *GatewayReconciler) getVirtualServicesForGateway(ctx, gateway) ([]VirtualService, error)`
- **Описание**: Получает все VirtualService, связанные с Gateway
- **Параметры**: 
  - `ctx context.Context` - контекст
  - `gateway *istionetworkingv1beta1.Gateway` - Gateway ресурс
- **Возвращает**: 
  - `[]istionetworkingv1beta1.VirtualService` - список связанных VirtualService
  - `error` - ошибка получения
- **Особенности**: 
  - Ищет VirtualService во всех namespace
  - Проверяет ссылки на Gateway в формате "gateway" или "namespace/gateway"

##### `(r *GatewayReconciler) SetupWithManager(mgr) error`
- **Описание**: Настраивает контроллер для работы с менеджером
- **Параметры**: 
  - `mgr ctrl.Manager` - менеджер контроллеров
- **Возвращает**: 
  - `error` - ошибка настройки

---

## cmd/main.go

**Описание**: Главный файл приложения, точка входа оператора. Инициализирует менеджер контроллеров, настраивает метрики, webhooks и health checks.

### Функции

#### `init()`
- **Описание**: Инициализирует runtime scheme, добавляя стандартные Kubernetes схемы
- **Параметры**: Нет
- **Возвращает**: Нет
- **Использование**: Вызывается автоматически при загрузке пакета

#### `main()`
- **Описание**: Главная функция приложения. Настраивает и запускает controller manager
- **Параметры**: Нет (использует флаги командной строки)
- **Возвращает**: Нет (завершает процесс при ошибке)
- **Флаги командной строки**:
  - `--metrics-bind-address`: Адрес для метрик (по умолчанию "0" - отключено)
  - `--health-probe-bind-address`: Адрес для health checks (по умолчанию ":8081")
  - `--leader-elect`: Включить leader election (по умолчанию false)
  - `--metrics-secure`: Использовать HTTPS для метрик (по умолчанию true)
  - `--webhook-cert-path`: Путь к сертификатам webhook
  - `--webhook-cert-name`: Имя файла сертификата webhook (по умолчанию "tls.crt")
  - `--webhook-cert-key`: Имя файла ключа webhook (по умолчанию "tls.key")
  - `--metrics-cert-path`: Путь к сертификатам метрик
  - `--metrics-cert-name`: Имя файла сертификата метрик (по умолчанию "tls.crt")
  - `--metrics-cert-key`: Имя файла ключа метрик (по умолчанию "tls.key")
  - `--enable-http2`: Включить HTTP/2 (по умолчанию false)
- **Основные действия**:
  1. Парсинг флагов командной строки
  2. Настройка логирования (zap)
  3. Настройка TLS для webhook и метрик
  4. Создание certificate watchers
  5. Инициализация controller manager
  6. Настройка health checks
  7. Запуск manager

---

## test/utils/utils.go

**Описание**: Утилиты для выполнения команд, установки зависимостей и работы с тестовым окружением.

### Константы

- `prometheusOperatorVersion`: Версия Prometheus Operator (v0.77.1)
- `prometheusOperatorURL`: URL для загрузки Prometheus Operator bundle
- `certmanagerVersion`: Версия cert-manager (v1.16.3)
- `certmanagerURLTmpl`: Шаблон URL для загрузки cert-manager

### Функции

#### `warnError(err error)`
- **Описание**: Выводит предупреждение об ошибке в GinkgoWriter
- **Параметры**: 
  - `err error` - ошибка для вывода
- **Возвращает**: Нет
- **Использование**: Вспомогательная функция для логирования ошибок в тестах

#### `Run(cmd *exec.Cmd) (string, error)`
- **Описание**: Выполняет команду в контексте проекта и возвращает вывод
- **Параметры**: 
  - `cmd *exec.Cmd` - команда для выполнения
- **Возвращает**: 
  - `string` - вывод команды
  - `error` - ошибка выполнения
- **Особенности**:
  - Устанавливает рабочую директорию в корень проекта
  - Добавляет переменную окружения `GO111MODULE=on`
  - Логирует выполняемую команду

#### `InstallPrometheusOperator() error`
- **Описание**: Устанавливает Prometheus Operator в кластер через kubectl
- **Параметры**: Нет
- **Возвращает**: 
  - `error` - ошибка установки
- **Использование**: В тестах для настройки метрик

#### `UninstallPrometheusOperator()`
- **Описание**: Удаляет Prometheus Operator из кластера
- **Параметры**: Нет
- **Возвращает**: Нет
- **Особенности**: Игнорирует ошибки (использует warnError)

#### `IsPrometheusCRDsInstalled() bool`
- **Описание**: Проверяет, установлены ли CRD Prometheus в кластере
- **Параметры**: Нет
- **Возвращает**: 
  - `bool` - true если CRD найдены
- **Проверяемые CRD**:
  - `prometheuses.monitoring.coreos.com`
  - `prometheusrules.monitoring.coreos.com`
  - `prometheusagents.monitoring.coreos.com`

#### `UninstallCertManager()`
- **Описание**: Удаляет cert-manager из кластера
- **Параметры**: Нет
- **Возвращает**: Нет
- **Особенности**: Игнорирует ошибки (использует warnError)

#### `InstallCertManager() error`
- **Описание**: Устанавливает cert-manager в кластер и ждет готовности webhook
- **Параметры**: Нет
- **Возвращает**: 
  - `error` - ошибка установки
- **Особенности**:
  - Применяет манифесты cert-manager
  - Ожидает готовности deployment `cert-manager-webhook` в namespace `cert-manager`
  - Таймаут ожидания: 5 минут

#### `IsCertManagerCRDsInstalled() bool`
- **Описание**: Проверяет, установлены ли CRD cert-manager в кластере
- **Параметры**: Нет
- **Возвращает**: 
  - `bool` - true если CRD найдены
- **Проверяемые CRD**:
  - `certificates.cert-manager.io`
  - `issuers.cert-manager.io`
  - `clusterissuers.cert-manager.io`
  - `certificaterequests.cert-manager.io`
  - `orders.acme.cert-manager.io`
  - `challenges.acme.cert-manager.io`

#### `LoadImageToKindClusterWithName(name string) error`
- **Описание**: Загружает Docker образ в Kind кластер
- **Параметры**: 
  - `name string` - имя образа для загрузки
- **Возвращает**: 
  - `error` - ошибка загрузки
- **Особенности**:
  - Использует переменную окружения `KIND_CLUSTER` (по умолчанию "kind")
  - Выполняет команду `kind load docker-image`

#### `GetNonEmptyLines(output string) []string`
- **Описание**: Преобразует вывод команды в массив непустых строк
- **Параметры**: 
  - `output string` - вывод команды
- **Возвращает**: 
  - `[]string` - массив непустых строк
- **Использование**: Для парсинга вывода kubectl команд

#### `GetProjectDir() (string, error)`
- **Описание**: Возвращает корневую директорию проекта
- **Параметры**: Нет
- **Возвращает**: 
  - `string` - путь к корню проекта
  - `error` - ошибка получения директории
- **Особенности**: Удаляет `/test/e2e` из пути, если присутствует

#### `UncommentCode(filename, target, prefix string) error`
- **Описание**: Раскомментирует код в файле, удаляя указанный префикс
- **Параметры**: 
  - `filename string` - путь к файлу
  - `target string` - целевой код для раскомментирования
  - `prefix string` - префикс комментария для удаления
- **Возвращает**: 
  - `error` - ошибка обработки файла
- **Использование**: Для автоматического раскомментирования кода в тестах

---

## test/e2e/e2e_test.go

**Описание**: End-to-end тесты для проверки работы оператора в реальном кластере.

### Константы

- `namespace`: Namespace для развертывания оператора ("example-system")
- `serviceAccountName`: Имя ServiceAccount ("example-controller-manager")
- `metricsServiceName`: Имя сервиса метрик ("example-controller-manager-metrics-service")
- `metricsRoleBindingName`: Имя RoleBinding для метрик ("example-metrics-binding")

### Функции

#### `TestE2E(t *testing.T)`
- **Описание**: Запускает e2e тестовый suite
- **Параметры**: 
  - `t *testing.T` - тестовый контекст
- **Возвращает**: Нет
- **Использование**: Точка входа для e2e тестов

#### `serviceAccountToken() (string, error)`
- **Описание**: Получает токен для ServiceAccount через Kubernetes TokenRequest API
- **Параметры**: Нет
- **Возвращает**: 
  - `string` - токен ServiceAccount
  - `error` - ошибка получения токена
- **Особенности**:
  - Использует kubectl create --raw для создания токена
  - Парсит JSON ответ для извлечения токена
  - Использует Eventually для повторных попыток

#### `getMetricsOutput() string`
- **Описание**: Получает логи из curl-metrics пода для доступа к метрикам
- **Параметры**: Нет
- **Возвращает**: 
  - `string` - вывод curl команды с метриками
- **Особенности**: Проверяет наличие HTTP 200 OK в выводе

### Типы

#### `tokenRequest`
- **Описание**: Упрощенное представление ответа Kubernetes TokenRequest API
- **Поля**:
  - `Status.Token string` - токен доступа

### Ginkgo тесты

#### `Describe("Manager", ...)`
- **Описание**: Основной блок e2e тестов для проверки работы manager
- **BeforeAll**: 
  - Создает namespace
  - Применяет restricted security policy
  - Устанавливает CRD
  - Развертывает controller-manager
- **AfterAll**: 
  - Удаляет curl-metrics pod
  - Удаляет controller-manager
  - Удаляет CRD
  - Удаляет namespace
- **AfterEach**: 
  - При ошибке собирает логи, события и описание подов
- **Тесты**:
  1. `It("should run successfully")` - проверяет, что controller-manager pod запущен и работает
  2. `It("should ensure the metrics endpoint is serving metrics")` - проверяет доступность метрик

---

## test/e2e/e2e_suite_test.go

**Описание**: Настройка e2e тестового окружения, установка зависимостей перед тестами.

### Переменные

- `skipCertManagerInstall`: Пропустить установку cert-manager (из переменной окружения `CERT_MANAGER_INSTALL_SKIP`)
- `isCertManagerAlreadyInstalled`: Флаг, указывающий что cert-manager уже установлен
- `projectImage`: Имя образа для тестирования ("example.com/example:v0.0.1")

### Функции

#### `TestE2E(t *testing.T)`
- **Описание**: Регистрирует fail handler и запускает Ginkgo suite
- **Параметры**: 
  - `t *testing.T` - тестовый контекст
- **Возвращает**: Нет

### Ginkgo хуки

#### `BeforeSuite`
- **Описание**: Выполняется один раз перед всеми тестами
- **Действия**:
  1. Собирает Docker образ оператора
  2. Загружает образ в Kind кластер
  3. Проверяет наличие cert-manager CRD
  4. Устанавливает cert-manager, если не установлен и не пропущен

#### `AfterSuite`
- **Описание**: Выполняется один раз после всех тестов
- **Действия**:
  1. Удаляет cert-manager, если был установлен в BeforeSuite

---

## Примечания

### Маркеры kubebuilder

В коде используются специальные маркеры kubebuilder для генерации кода:

- `// +kubebuilder:scaffold:imports` - место для добавления импортов
- `// +kubebuilder:scaffold:scheme` - место для добавления схем в runtime.Scheme
- `// +kubebuilder:scaffold:builder` - место для регистрации контроллеров
- `// +kubebuilder:scaffold:e2e-webhooks-checks` - место для добавления webhook тестов

### Структура проекта

```
.
├── cmd/
│   └── main.go              # Точка входа
├── internal/
│   └── controller/          # Контроллеры оператора
│       ├── setup.go         # Настройка контроллеров
│       ├── certificate_controller.go
│       ├── http01_solver_pod_controller.go
│       ├── issuer_controller.go
│       └── gateway_controller.go
├── api/                     # API определения (CRD)
├── controllers/             # Контроллеры (legacy)
├── test/
│   ├── utils/
│   │   └── utils.go         # Утилиты для тестов
│   └── e2e/
│       ├── e2e_suite_test.go # Настройка e2e
│       └── e2e_test.go      # E2E тесты
├── helm/                    # Helm chart
│   └── istio-http01/           # Chart оператора
└── config/                  # Kustomize конфигурации
```

### Обновление документации

При добавлении новых функций или изменении существующих, обновляйте этот документ для поддержания актуальности индекса.

---

**Последнее обновление**: 2026-01-10

