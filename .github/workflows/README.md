# GitHub Actions Workflows

Этот каталог содержит GitHub Actions workflows для автоматизации CI/CD процессов проекта.

## Доступные Workflows

### 1. Lint (`lint.yml`)
**Триггеры:**
- Push в ветки: `main`, `master`, `develop`
- Pull Request

**Что делает:**
- Запускает `golangci-lint` для проверки качества кода
- Использует версию линтера из `go.mod`

### 2. Tests (`test.yml`)
**Триггеры:**
- Push в ветки: `main`, `master`, `develop`
- Pull Request

**Что делает:**
- Запускает unit тесты через `make test`
- Проверяет, что код компилируется и тесты проходят

### 3. E2E Tests (`test-e2e.yml`)
**Триггеры:**
- Push в ветки: `main`, `master`, `develop`
- Pull Request

**Что делает:**
- Устанавливает Kind (Kubernetes in Docker)
- Запускает end-to-end тесты через `make test-e2e`
- Создает временный кластер для тестирования

### 4. Build and Push Docker Image (`build-image.yml`)
**Триггеры:**
- Push тегов вида `v*.*.*` (например, `v0.1.0`, `v1.2.3`)
- Ручной запуск через `workflow_dispatch`

**Что делает:**
- Собирает Docker образ для платформы `linux/amd64`
- Отправляет образ в Docker Hub (требует `DOCKER_USERNAME` и `DOCKER_PASSWORD` в secrets)
- Использует кэширование через GitHub Actions cache

**Переменные окружения:**
- `REGISTRY`: Docker registry (по умолчанию `rieset/istio-http01`)

**Secrets:**
- `DOCKER_USERNAME` - имя пользователя Docker Hub
- `DOCKER_PASSWORD` - пароль или токен доступа Docker Hub

**Ручной запуск:**
Можно запустить вручную через GitHub UI с параметрами:
- `image_tag`: Тег образа (например, `0.1.0`)
- `registry`: Docker registry (опционально, по умолчанию `rieset/istio-http01`)

**Пример использования:**
```bash
# При создании тега v0.1.0 автоматически соберется образ rieset/istio-http01:0.1.0
git tag v0.1.0
git push origin v0.1.0
```

### 5. Build and Package Helm Chart (`build-helm-chart.yml`)
**Триггеры:**
- Push тегов вида `v*.*.*` (например, `v0.1.0`, `v1.2.3`)
- Ручной запуск через `workflow_dispatch`

**Что делает:**
- Обновляет версию в `Chart.yaml` на основе тега
- Линтует Helm chart через `helm lint`
- Упаковывает chart в `.tgz` файл
- Загружает артефакт в GitHub Actions
- Создает GitHub Release с прикрепленным chart (при создании тега)
- **Публикует chart в GitHub Packages (OCI registry)**
- **Проверяет доступность опубликованного chart**

**Ручной запуск:**
Можно запустить вручную через GitHub UI с параметрами:
- `chart_version`: Версия chart (например, `0.1.0`)
- `app_version`: Версия приложения (опционально, по умолчанию равна `chart_version`)

**Пример использования:**
```bash
# При создании тега v0.1.0 автоматически:
# 1. Обновится Chart.yaml (version: 0.1.0, appVersion: "0.1.0")
# 2. Соберется chart istio-http01-0.1.0.tgz
# 3. Создастся GitHub Release с прикрепленным chart
# 4. Опубликуется в GitHub Packages: ghcr.io/OWNER/helm-charts/istio-http01:0.1.0
# 5. Проверится доступность chart в registry
git tag v0.1.0
git push origin v0.1.0
```

**Установка chart из GitHub Packages:**
```bash
# Авторизация в GitHub Container Registry (если требуется)
echo $GITHUB_TOKEN | helm registry login ghcr.io -u USERNAME --password-stdin

# Установка chart
helm install istio-http01 oci://ghcr.io/OWNER/helm-charts/istio-http01 --version 0.1.0
```

**Где найти опубликованный chart:**
- GitHub Packages: `https://github.com/OWNER?tab=packages`
- OCI registry: `ghcr.io/OWNER/helm-charts/istio-http01:VERSION`

## Настройка Secrets

Для работы workflows необходимо настроить следующие secrets в GitHub:

1. **DOCKER_USERNAME** - имя пользователя Docker Hub
2. **DOCKER_PASSWORD** - пароль или токен доступа Docker Hub

**Как настроить:**
1. Перейдите в репозиторий на GitHub
2. Откройте **Settings** → **Secrets and variables** → **Actions**
3. Нажмите **"New repository secret"**
4. Добавьте каждый secret:
   - **Name**: `DOCKER_USERNAME`
   - **Secret**: ваш username Docker Hub
   - **Name**: `DOCKER_PASSWORD`
   - **Secret**: ваш пароль или Personal Access Token (PAT) Docker Hub

**Важно:**
- Для безопасности рекомендуется использовать **Personal Access Token (PAT)** вместо пароля
- PAT можно создать в Docker Hub: Account Settings → Security → New Access Token
- После добавления secrets, workflows будут автоматически их использовать

**Проверка настройки:**
После настройки secrets, при запуске workflow `build-image.yml` вы должны увидеть:
```
✅ Docker Hub secrets are configured
```

Если secrets не настроены, вы получите понятное сообщение об ошибке с инструкциями.

## Процесс релиза

Для создания нового релиза:

1. **Обновите версию в коде** (если необходимо)
2. **Создайте тег:**
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

3. **Автоматически запустятся оба workflow параллельно:**
   - ✅ `build-image.yml` - соберет и отправит Docker образ в Docker Hub
   - ✅ `build-helm-chart.yml` - соберет Helm chart, опубликует в GitHub Packages и создаст GitHub Release

4. **Результат:**
   - ✅ Docker образ: `rieset/istio-http01:0.1.0` (в Docker Hub)
   - ✅ Helm chart: `istio-http01-0.1.0.tgz` (прикреплен к GitHub Release)
   - ✅ Helm chart в GitHub Packages: `ghcr.io/OWNER/helm-charts/istio-http01:0.1.0`

**Важно:**
- Оба workflow запускаются **автоматически** при пуше тега вида `v*.*.*`
- Workflows выполняются **параллельно** для ускорения процесса
- Все артефакты будут доступны после завершения обоих workflows

## Проверка статуса Workflows

Все workflows можно проверить на странице:
```
https://github.com/<owner>/<repo>/actions
```

## Troubleshooting

### Workflow не запускается
- Проверьте, что триггеры настроены правильно
- Убедитесь, что тег соответствует формату `v*.*.*`

### Ошибка авторизации Docker Hub
- Проверьте, что secrets `DOCKER_USERNAME` и `DOCKER_PASSWORD` настроены
- Убедитесь, что токен доступа имеет права на push в registry

### Ошибка создания GitHub Release
- Проверьте, что у workflow есть права `contents: write`
- Убедитесь, что тег существует в репозитории

