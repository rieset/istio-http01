# Временные сертификаты и управление HSTS

Этот документ описывает механизм автоматического создания временных самоподписанных сертификатов и управления HSTS (HTTP Strict Transport Security) в операторе Istio HTTP01.

## Обзор

Оператор автоматически создает временные самоподписанные сертификаты для Gateway, у которых включен `httpsRedirect: true`, но основной сертификат еще не готов. Это позволяет сайту работать даже до получения валидного сертификата от Let's Encrypt.

## Проблема

Когда Gateway имеет `httpsRedirect: true`, но сертификат еще не готов:

1. **HTTP01 challenge не может пройти**: Let's Encrypt не может проверить домен через HTTP, так как все запросы перенаправляются на HTTPS
2. **HSTS блокирует доступ**: Если домен ранее использовал HSTS, браузеры (например, Chrome) блокируют доступ к сайту с самоподписанным сертификатом
3. **Сайт недоступен**: Пользователи не могут получить доступ к сайту до получения валидного сертификата

## Решение

Оператор автоматически:

1. **Создает временный самоподписанный сертификат** с теми же DNS именами, что и оригинальный сертификат, плюс все домены из связанных VirtualService
2. **Обновляет Gateway** для использования временного секрета
3. **Отключает `httpsRedirect`** на HTTP сервере (порт 80) для прохождения HTTP01 challenge
4. **Создает EnvoyFilter** для отключения HSTS заголовка через Lua фильтр
5. **Восстанавливает оригинальный сертификат** после его готовности

## Процесс работы

### Шаг 1: Обнаружение неготового сертификата

Оператор отслеживает Certificate ресурсы и определяет, когда сертификат не готов:

```go
isReady := r.isCertificateReady(cert)
// Проверяет условие Ready в статусе Certificate
```

### Шаг 2: Проверка Gateway

Если сертификат используется в Gateway с `httpsRedirect: true`:

```go
if r.hasHTTPSRedirect(gateway) {
    // Создаем временный сертификат и настраиваем Gateway
    r.ensureTemporaryCertificateSetup(ctx, cert, gateway)
}
```

### Шаг 3: Создание временного сертификата

Оператор создает:

1. **Временный Issuer** (self-signed):
   ```yaml
   apiVersion: cert-manager.io/v1
   kind: Issuer
   metadata:
     name: gateway-cert-beta8-temp-selfsigned-issuer
     namespace: istio-system
     labels:
       app.kubernetes.io/managed-by: istio-http01
       istio-http01.rieset.io/temp: "true"
   spec:
     selfSigned: {}
   ```

2. **Временный Certificate**:
   ```yaml
   apiVersion: cert-manager.io/v1
   kind: Certificate
   metadata:
     name: gateway-cert-beta8-temp-selfsigned
     namespace: istio-system
     labels:
       app.kubernetes.io/managed-by: istio-http01
       istio-http01.rieset.io/temp: "true"
       istio-http01.rieset.io/original-cert: gateway-cert-beta8
   spec:
     secretName: gateway-cert-secret-beta8-temp
     dnsNames:
       - beta8.b2c.pr0d.h3llo.dev
       - app.beta8.b2c.pr0d.h3llo.dev
     issuerRef:
       name: gateway-cert-beta8-temp-selfsigned-issuer
       kind: Issuer
   ```

**Важно**: DNS имена во временном сертификате объединяются из:
- DNS имен оригинального Certificate (`spec.dnsNames`)
- Доменов Gateway из связанных VirtualService

Это гарантирует, что временный сертификат покрывает все домены Gateway.

### Шаг 4: Обновление Gateway

После готовности временного сертификата оператор:

1. **Обновляет HTTPS сервер** (порт 443) для использования временного секрета:
   ```yaml
   spec:
     servers:
       - port:
           number: 443
         tls:
           credentialName: istio-system/gateway-cert-secret-beta8-temp
   ```

2. **Отключает `httpsRedirect`** на HTTP сервере (порт 80):
   ```yaml
   spec:
     servers:
       - port:
           number: 80
         tls:
           httpsRedirect: false  # Отключено для HTTP01 challenge
   ```

3. **Добавляет аннотации** для отслеживания оригинального состояния:
   ```yaml
   metadata:
     annotations:
       istio-http01.rieset.io/original-credential-name-gateway-cert-secret-beta8: "istio-system/gateway-cert-secret-beta8"
       istio-http01.rieset.io/original-https-redirect-gateway-cert-secret-beta8: "true"
   ```

### Шаг 5: Создание EnvoyFilter

**КРИТИЧЕСКИ ВАЖНО**: EnvoyFilter создается **сразу при создании временного сертификата**, ДО того как он станет готовым. Это предотвращает кеширование HSTS заголовка браузером при первом обращении к ресурсу.

Оператор создает EnvoyFilter для отключения HSTS заголовка:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: disable-hsts-h3gateway-beta8-h3gateway
  namespace: h3gateway-beta8
  labels:
    app.kubernetes.io/managed-by: istio-http01
    istio-http01.rieset.io/temp: "true"
    istio-http01.rieset.io/original-cert: gateway-cert-secret-beta8
spec:
  workloadSelector:
    labels:
      istio: h3-cluster-istio  # Селектор из Gateway
  configPatches:
  - applyTo: HTTP_FILTER
    match:
      context: GATEWAY
      proxy:
        proxyVersion: ".*"
      listener:
        filterChain:
          filter:
            name: envoy.filters.network.http_connection_manager
    patch:
      operation: INSERT_BEFORE
      value:
        name: envoy.filters.http.lua
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
          inline_code: |
            function envoy_on_response(response_handle)
              response_handle:headers():remove("strict-transport-security")
            end
```

**Важно**: 
- EnvoyFilter использует селектор из Gateway (`spec.selector`), а не фиксированный `istio: ingressgateway`, что позволяет правильно применять фильтр к нужным подам.
- EnvoyFilter создается **сразу при создании временного сертификата**, чтобы предотвратить кеширование HSTS браузером. Подробнее см. [Временные аспекты HSTS и EnvoyFilter](hsts-timing.md).

### Шаг 6: Восстановление после готовности

Когда основной сертификат становится готовым:

1. **В debug режиме**: Оператор проверяет, прошло ли 5 минут с момента создания временного сертификата
2. **Восстановление Gateway**:
   - Восстанавливает оригинальный секрет в HTTPS сервере
   - Включает обратно `httpsRedirect` на HTTP сервере
   - Удаляет аннотации
3. **Удаление EnvoyFilter**: HSTS включается обратно
4. **Удаление временных ресурсов**: Временный Certificate и Issuer удаляются

## Debug режим

Для тестирования временных сертификатов доступен debug режим, который задерживает восстановление оригинального сертификата на 5 минут.

### Включение

В `values.yaml`:
```yaml
debug: true
```

Или при установке через Helm:
```bash
helm install istio-http01 \
  oci://ghcr.io/rieset/helm-charts/istio-http01 \
  --version <version> \
  -n istio-system \
  --create-namespace \
  --set debug=true
```

### Как работает

- Когда основной сертификат становится готовым, оператор проверяет время создания временного сертификата
- Если прошло менее 5 минут, восстановление откладывается
- В логах оператора отображается оставшееся время до восстановления
- После истечения 5 минут оператор автоматически восстанавливает оригинальный сертификат

## Периодическая проверка

Оператор периодически (каждые 30 секунд) проверяет состояние временных сертификатов через функцию `ensureTemporaryCertificateSetup`:

1. Проверяет наличие временного сертификата
2. Проверяет готовность временного сертификата
3. Проверяет, использует ли Gateway временный секрет
4. Проверяет, отключен ли `httpsRedirect`
5. Проверяет наличие EnvoyFilter

Если какой-то компонент отсутствует, оператор автоматически создает его.

## Проверка сертификатов

Оператор выполняет проверку сертификатов через HTTPS запросы:

1. **Получение доменов Gateway** из связанных VirtualService
2. **Получение IP адреса ingress gateway** по селектору Gateway
3. **HTTPS запрос** к домену через ingress IP
4. **Проверка DNS имен** в сертификате на соответствие доменам Gateway и Certificate

Это позволяет убедиться, что Istio использует правильный сертификат.

## Аннотации Gateway

Оператор добавляет следующие аннотации к Gateway:

- `istio-http01.rieset.io/original-credential-name-<secretName>`: Хранит оригинальное имя секрета для восстановления
- `istio-http01.rieset.io/original-https-redirect-<secretName>`: Хранит оригинальное значение `httpsRedirect` для восстановления

Эти аннотации автоматически удаляются при восстановлении оригинального сертификата.

## Логирование

Оператор логирует важные события:

- Создание временного сертификата
- Обновление Gateway для использования временного секрета
- Создание EnvoyFilter
- Восстановление оригинального сертификата
- Удаление временных ресурсов
- В debug режиме: оставшееся время до восстановления

## Важные моменты

1. **Временные сертификаты покрывают все домены Gateway**: DNS имена объединяются из Certificate и VirtualService
2. **EnvoyFilter использует селектор Gateway**: Правильно применяется к нужным подам
3. **Автоматическое восстановление**: После готовности основного сертификата все временные ресурсы автоматически удаляются
4. **Периодическая проверка**: Оператор автоматически восстанавливает недостающие компоненты
5. **Debug режим**: Позволяет тестировать временные сертификаты без необходимости ждать готовности основного сертификата

## Примеры использования

### Проверка временного сертификата

```bash
# Проверка DNS имен во временном сертификате
kubectl get certificate gateway-cert-beta8-temp-selfsigned -n istio-system \
  -o jsonpath='{.spec.dnsNames[*]}'

# Проверка EnvoyFilter
kubectl get envoyfilter disable-hsts-h3gateway-beta8-h3gateway -n h3gateway-beta8

# Проверка Gateway
kubectl get gateway h3gateway -n h3gateway-beta8 \
  -o jsonpath='{.spec.servers[?(@.port.number==443)].tls.credentialName}'
```

### Проверка HSTS заголовка

```bash
# Проверка, что HSTS заголовок удаляется
curl -I -k https://beta8.b2c.pr0d.h3llo.dev/echo | grep -i "strict-transport-security"
# Должно быть пусто (заголовок удален)
```

## См. также

- [README.md](../README.md) - Основная документация оператора
- [code-index.md](code-index.md) - Индекс функций кода
- [glossary.md](glossary.md) - Глоссарий терминов

