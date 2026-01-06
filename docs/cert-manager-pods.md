# Структура cert-manager HTTP01 Challenge Pods

Этот документ описывает структуру подов, создаваемых cert-manager для обработки HTTP01 challenge от Let's Encrypt.

## Обзор

cert-manager создает временные поды для обработки HTTP01 challenge запросов. Эти поды должны быть доступны по HTTP на домене, для которого запрашивается сертификат. Оператор Istio Gas мониторит эти поды для обеспечения корректной работы процесса валидации.

## Идентификация подов

### Метки (Labels)

Поды HTTP01 solver имеют следующие метки, которые используются для их идентификации:

```yaml
labels:
  # Маркер, указывающий что это HTTP01 solver под
  acme.cert-manager.io/http01-solver: 'true'
  
  # Идентификатор домена для challenge
  acme.cert-manager.io/http-domain: '<domain-id>'
  
  # Токен для challenge
  acme.cert-manager.io/http-token: '<token-id>'
```

**Важно**: Метка `acme.cert-manager.io/http01-solver: 'true'` является основным идентификатором подов HTTP01 solver.

### Имя пода

Имена подов генерируются автоматически с префиксом:
```
cm-acme-http-solver-<random-suffix>
```

Пример: `cm-acme-http-solver-abc123`

### Owner References

Каждый под связан с Challenge ресурсом cert-manager через `ownerReferences`:

```yaml
ownerReferences:
  - apiVersion: acme.cert-manager.io/v1
    kind: Challenge
    name: <challenge-name>
    controller: true
    blockOwnerDeletion: true
```

Это позволяет:
- Отслеживать родительский Challenge ресурс
- Автоматически удалять поды при удалении Challenge
- Понимать контекст создания пода

## Важные поля для мониторинга

### Статус пода

#### Phase
Текущая фаза пода:
- `Pending` - под создан, но еще не запущен
- `Running` - под запущен и работает
- `Succeeded` - под успешно завершил работу
- `Failed` - под завершился с ошибкой
- `Unknown` - состояние неизвестно

#### Conditions

Ключевые условия для проверки:

```yaml
conditions:
  - type: Ready  # ВАЖНО: Под готов обрабатывать запросы
    status: 'True'
  - type: ContainersReady  # Контейнеры готовы
    status: 'True'
  - type: PodScheduled  # Под запланирован на ноду
    status: 'True'
```

**Критично**: Условие `Ready: True` означает, что под может обрабатывать HTTP01 challenge запросы.

#### Container Status

Статус контейнера `acmesolver`:

```yaml
containerStatuses:
  - name: acmesolver
    ready: true  # Контейнер готов
    state:
      running:
        startedAt: '<timestamp>'
    restartCount: 0  # Количество перезапусков
```

### Спецификация пода

#### Контейнер

Контейнер `acmesolver` запускается с аргументами:

```yaml
args:
  - '--listen-port=8089'  # Порт HTTP сервера
  - '--domain=<domain>'   # Домен для валидации
  - '--token=<token>'     # Токен challenge
  - '--key=<key>'         # Ключ для ответа
```

#### Порты

```yaml
ports:
  - name: http
    containerPort: 8089  # Порт для HTTP запросов
    protocol: TCP
```

#### Ресурсы

Типичные ограничения ресурсов:

```yaml
resources:
  limits:
    cpu: 100m
    memory: 64Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

## Безопасность

Поды HTTP01 solver имеют строгие настройки безопасности:

### Security Context контейнера

```yaml
securityContext:
  capabilities:
    drop:
      - ALL  # Удаление всех capabilities
  readOnlyRootFilesystem: true  # Только чтение
  allowPrivilegeEscalation: false  # Запрет повышения привилегий
```

### Security Context пода

```yaml
securityContext:
  runAsNonRoot: true  # Запуск от непривилегированного пользователя
  seccompProfile:
    type: RuntimeDefault  # Дефолтный seccomp профиль
```

## Жизненный цикл

1. **Создание**: cert-manager создает под при инициации HTTP01 challenge
2. **Запуск**: Под запускается и начинает слушать на порту 8089
3. **Валидация**: Let's Encrypt делает HTTP запрос к поду
4. **Завершение**: После успешной валидации под удаляется

## Мониторинг

Оператор Istio Gas должен отслеживать:

1. **Создание подов** с меткой `acme.cert-manager.io/http01-solver: 'true'`
2. **Статус готовности** (`Ready: True`)
3. **Состояние контейнера** (`ready: true`)
4. **Фазу пода** (`phase: Running`)
5. **Количество перезапусков** (`restartCount`)

## Пример

Полный пример пода см. в файле [cert-manager-pod-example.yaml](./cert-manager-pod-example.yaml)

## Селекторы для поиска

Для поиска всех HTTP01 solver подов используйте селектор:

```go
labels.SelectorFromSet(map[string]string{
    "acme.cert-manager.io/http01-solver": "true",
})
```

Или через метки:

```yaml
selector:
  matchLabels:
    acme.cert-manager.io/http01-solver: "true"
```

---

**Примечание**: Структура подов может изменяться в зависимости от версии cert-manager. Проверяйте актуальную документацию cert-manager при обновлении версий.

