# Отслеживание HTTP01 Solver Pods и проверка VirtualService

Этот документ подробно описывает, как оператор Istio Gas отслеживает появление подов HTTP01 solver (`cm-acme-http-solver-*`) и проверяет наличие VirtualService для них в правильном Gateway.

## Обзор процесса

```
cert-manager создает Pod
    ↓
Controller-runtime Watch обнаруживает событие
    ↓
Predicate фильтрует только HTTP01 solver поды
    ↓
Reconcile вызывается для пода
    ↓
Извлечение домена из аргументов контейнера
    ↓
Поиск Gateway для домена
    ↓
Проверка наличия VirtualService
    ↓
Создание VirtualService (если отсутствует)
```

## Шаг 1: Регистрация контроллера

### 1.1 Инициализация в `cmd/main.go`

При запуске оператора вызывается `controller.SetupControllers(mgr)`, который регистрирует все контроллеры:

```go
// internal/controller/setup.go
func SetupControllers(mgr ctrl.Manager) error {
    // HTTP01 Solver Pod controller
    if err := (&HTTP01SolverPodReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        return err
    }
    // ... другие контроллеры
}
```

### 1.2 Настройка Watch в `SetupWithManager`

```go
// internal/controller/http01_solver_pod_controller.go:222-234
func (r *HTTP01SolverPodReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Предикат для фильтрации только HTTP01 solver подов
    http01SolverPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
        pod := obj.(*corev1.Pod)
        // Проверка имени и метки
        return strings.HasPrefix(pod.Name, "cm-acme-http-solver-") &&
            pod.Labels["acme.cert-manager.io/http01-solver"] == "true"
    })

    return ctrl.NewControllerManagedBy(mgr).
        For(&corev1.Pod{}).                    // Watch для всех Pod
        WithEventFilter(http01SolverPredicate). // Фильтр только HTTP01 solver
        Complete(r)
}
```

**Что происходит:**
- `For(&corev1.Pod{})` - контроллер подписывается на события всех Pod в кластере
- `WithEventFilter(http01SolverPredicate)` - применяется фильтр, который пропускает только поды:
  - С именем, начинающимся с `cm-acme-http-solver-`
  - С меткой `acme.cert-manager.io/http01-solver: "true"`

**RBAC права:**
```go
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
```

## Шаг 2: Обнаружение события

Когда cert-manager создает под `cm-acme-http-solver-abc123`:

1. **Kubernetes API Server** отправляет событие через Watch API
2. **Controller-runtime** получает событие (Create/Update/Delete)
3. **Predicate** проверяет, соответствует ли под критериям:
   - Имя: `cm-acme-http-solver-abc123` ✓ (начинается с `cm-acme-http-solver-`)
   - Метка: `acme.cert-manager.io/http01-solver: "true"` ✓
4. Если оба условия выполнены, событие передается в `Reconcile`

## Шаг 3: Reconcile - обработка пода

### 3.1 Получение пода

```go
// internal/controller/http01_solver_pod_controller.go:58-62
pod := &corev1.Pod{}
if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
    return ctrl.Result{}, client.IgnoreNotFound(err)
}
```

`req.NamespacedName` содержит:
- `Name`: `cm-acme-http-solver-abc123`
- `Namespace`: `istio-system`

### 3.2 Дополнительная проверка

```go
// internal/controller/http01_solver_pod_controller.go:64-67
if !strings.HasPrefix(pod.Name, "cm-acme-http-solver-") {
    return ctrl.Result{}, nil
}
```

Двойная проверка на случай, если predicate пропустил неподходящий под.

### 3.3 Извлечение домена

```go
// internal/controller/http01_solver_pod_controller.go:90-111
domain, err := r.extractDomainFromPod(pod)
```

**Функция `extractDomainFromPod`:**

```go
// internal/controller/http01_solver_pod_controller.go:237-248
func (r *HTTP01SolverPodReconciler) extractDomainFromPod(pod *corev1.Pod) (string, error) {
    for _, container := range pod.Spec.Containers {
        if container.Name == "acmesolver" {
            for _, arg := range container.Args {
                if strings.HasPrefix(arg, "--domain=") {
                    return strings.TrimPrefix(arg, "--domain="), nil
                }
            }
        }
    }
    return "", nil
}
```

**Пример аргументов контейнера:**
```yaml
containers:
  - name: acmesolver
    args:
      - '--listen-port=8089'
      - '--domain=app.example.com'  # ← Извлекается этот домен
      - '--token=UiXCAkqSyCLNsMaWc5POhWknQi1o9FEjKixVVxkyv0k'
      - '--key=...'
```

**Результат:** `domain = "app.example.com"`

## Шаг 4: Поиск Gateway для домена

### 4.1 Функция `findGatewayForDomain`

```go
// internal/controller/http01_solver_pod_controller.go:114-132
gateway, err := r.findGatewayForDomain(ctx, domain)
```

### 4.2 Алгоритм поиска (двухэтапный)

#### Этап 1: Прямой поиск по hosts в Gateway

```go
// internal/controller/http01_solver_pod_controller.go:256-278
// Получение всех Gateway во всех namespace
gatewayList := &istionetworkingv1beta1.GatewayList{}
r.List(ctx, gatewayList, client.InNamespace(""))

// Проверка каждого Gateway
for i := range gatewayList.Items {
    gateway := gatewayList.Items[i]
    for _, server := range gateway.Spec.Servers {
        for _, host := range server.Hosts {
            // Проверка точного совпадения или wildcard
            if host == domain || host == "*" || 
               strings.HasSuffix(domain, "."+strings.TrimPrefix(host, "*.")) {
                return gateway, nil
            }
        }
    }
}
```

**Пример Gateway:**
```yaml
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: example-gateway
  namespace: example-gateway-ns
spec:
  servers:
    - port:
        number: 80
        protocol: HTTP
      hosts:
        - "*"  # ← Подходит для любого домена
```

**Результат:** Gateway найден, если:
- `host == domain` (точное совпадение)
- `host == "*"` (wildcard)
- `domain` заканчивается на `host` (например, `host = "*.example.com"`, `domain = "app.example.com"`)

#### Этап 2: Поиск через VirtualService

Если Gateway не найден напрямую, ищем через VirtualService:

```go
// internal/controller/http01_solver_pod_controller.go:281-319
virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
r.List(ctx, virtualServiceList, client.InNamespace(""))

// Для каждого Gateway проверяем связанные VirtualService
for i := range gatewayList.Items {
    gateway := gatewayList.Items[i]
    for j := range virtualServiceList.Items {
        vs := virtualServiceList.Items[j]
        // Проверка, ссылается ли VirtualService на этот Gateway
        for _, gw := range vs.Spec.Gateways {
            if gw == gatewayName || gw == gatewayRef {
                // Проверка, содержит ли VirtualService этот домен в hosts
                for _, host := range vs.Spec.Hosts {
                    if host == domain || 
                       strings.HasSuffix(domain, "."+host) || 
                       strings.HasSuffix(host, "."+domain) {
                        return gateway, nil
                    }
                }
            }
        }
    }
}
```

**Пример VirtualService:**
```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: app-vs
spec:
  gateways:
    - example-gateway-ns/example-gateway  # ← Ссылка на Gateway
  hosts:
    - app.example.com  # ← Домен совпадает
```

**Результат:** Gateway найден через VirtualService, если:
- VirtualService ссылается на Gateway
- VirtualService содержит домен в `hosts`

## Шаг 5: Проверка наличия VirtualService

### 5.1 Функция `findVirtualServiceForDomain`

```go
// internal/controller/http01_solver_pod_controller.go:142-160
existingVS, err := r.findVirtualServiceForDomain(ctx, gateway, domain)
```

### 5.2 Алгоритм проверки

```go
// internal/controller/http01_solver_pod_controller.go:325-378
func (r *HTTP01SolverPodReconciler) findVirtualServiceForDomain(
    ctx context.Context, 
    gateway *istionetworkingv1beta1.Gateway, 
    domain string,
) (*istionetworkingv1beta1.VirtualService, error) {
    // Получение всех VirtualService
    virtualServiceList := &istionetworkingv1beta1.VirtualServiceList{}
    r.List(ctx, virtualServiceList, client.InNamespace(""))

    gatewayRef := gateway.Name
    if gateway.Namespace != "" {
        gatewayRef = gateway.Namespace + "/" + gateway.Name
    }

    // Поиск VirtualService, который:
    // 1. Связан с этим Gateway
    // 2. Содержит этот домен в hosts
    // 3. Имеет маршрут к поду HTTP01 solver (проверяем по имени)
    for i := range virtualServiceList.Items {
        vs := virtualServiceList.Items[i]
        
        // Проверка связи с Gateway
        linkedToGateway := false
        for _, gw := range vs.Spec.Gateways {
            if gw == gatewayName || gw == gatewayRef {
                linkedToGateway = true
                break
            }
        }

        if !linkedToGateway {
            continue
        }

        // Проверка домена в hosts
        for _, host := range vs.Spec.Hosts {
            if host == domain {
                // Проверка, что это VirtualService для HTTP01 solver
                if strings.Contains(vs.Name, "http01-solver") || 
                   strings.Contains(vs.Name, "acme-solver") {
                    return vs, nil  // VirtualService найден
                }
            }
        }
    }

    return nil, nil  // VirtualService не найден
}
```

**Критерии поиска VirtualService:**
1. ✅ Связан с Gateway (в `spec.gateways` есть ссылка на найденный Gateway)
2. ✅ Содержит домен в `spec.hosts`
3. ✅ Имя содержит `"http01-solver"` или `"acme-solver"` (идентификатор для HTTP01 solver)

**Пример найденного VirtualService:**
```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: acme-solver-cm-acme-http-solver-abc123  # ← Содержит "acme-solver"
spec:
  gateways:
    - example-gateway-ns/example-gateway  # ← Связан с Gateway
  hosts:
    - app.example.com  # ← Домен совпадает
  http:
    - match:
        - uri:
            prefix: "/.well-known/acme-challenge/"
      route:
        - destination:
            host: cm-acme-http-solver-abc123.istio-system.svc.cluster.local
            port:
              number: 8089
```

## Шаг 6: Создание VirtualService (если отсутствует)

### 6.1 Проверка результата поиска

```go
// internal/controller/http01_solver_pod_controller.go:152-160
if existingVS != nil {
    logger.Info("VirtualService already exists for HTTP01 solver",
        "domain", domain,
        "gateway", gateway.Name,
        "virtualService", existingVS.Name,
        "virtualServiceNamespace", existingVS.Namespace,
        "solverPod", pod.Name,
    )
    return ctrl.Result{}, nil  // VirtualService уже существует, ничего не делаем
}
```

Если VirtualService найден - процесс завершается.

### 6.2 Создание VirtualService

Если VirtualService не найден, вызывается `createVirtualServiceForSolver`:

```go
// internal/controller/http01_solver_pod_controller.go:163-170
if err := r.createVirtualServiceForSolver(ctx, pod, gateway, domain); err != nil {
    logger.Error(err, "failed to create VirtualService for solver",
        "domain", domain,
        "gateway", gateway.Name,
        "solverPod", pod.Name,
    )
    return ctrl.Result{}, err
}
```

### 6.3 Процесс создания VirtualService

#### 6.3.1 Поиск Service для пода

```go
// internal/controller/http01_solver_pod_controller.go:385-447
// Сначала пытаемся найти Service по имени пода
serviceName := pod.Name
serviceKey := client.ObjectKey{
    Name:      serviceName,
    Namespace: pod.Namespace,
}
serviceCandidate := &corev1.Service{}
if err := r.Get(ctx, serviceKey, serviceCandidate); err == nil {
    service = serviceCandidate
} else {
    // Если не нашли по имени, ищем по меткам и ownerReferences
    for i := range serviceList.Items {
        svc := &serviceList.Items[i]
        if svc.Labels["acme.cert-manager.io/http01-solver"] == "true" {
            // Проверка ownerReferences - должны совпадать с подом
            for _, podOwner := range pod.OwnerReferences {
                for _, svcOwner := range svc.OwnerReferences {
                    if podOwner.Kind == svcOwner.Kind &&
                       podOwner.Name == svcOwner.Name &&
                       podOwner.APIVersion == svcOwner.APIVersion {
                        service = svc
                        break
                    }
                }
            }
        }
    }
}
```

**Важно:** cert-manager может создать Service с другим именем, поэтому поиск выполняется:
1. По имени пода (если совпадает)
2. По меткам и ownerReferences (если имя не совпадает)

#### 6.3.2 Определение порта

```go
// internal/controller/http01_solver_pod_controller.go:449-453
solverPort := uint32(8089) // Порт по умолчанию
if len(service.Spec.Ports) > 0 {
    solverPort = uint32(service.Spec.Ports[0].Port)
}
```

#### 6.3.3 Формирование имени VirtualService

```go
// internal/controller/http01_solver_pod_controller.go:461-466
vsName := fmt.Sprintf("http01-solver-%s", 
    strings.ReplaceAll(strings.ReplaceAll(domain, ".", "-"), "*", "wildcard"))
if len(vsName) > 63 {
    vsName = vsName[:63]  // Kubernetes имена ограничены 63 символами
}
```

**Пример:** `domain = "app.example.com"` → `vsName = "http01-solver-app-example-com"`

#### 6.3.4 Формирование Gateway reference

```go
// internal/controller/http01_solver_pod_controller.go:468-472
gatewayRef := gateway.Name
if gateway.Namespace != pod.Namespace {
    gatewayRef = fmt.Sprintf("%s/%s", gateway.Namespace, gateway.Name)
}
```

**Пример:** 
- Gateway в том же namespace: `gatewayRef = "example-gateway"`
- Gateway в другом namespace: `gatewayRef = "example-gateway-ns/example-gateway"`

#### 6.3.5 Создание VirtualService ресурса

```go
// internal/controller/http01_solver_pod_controller.go:474-522
virtualService := &istionetworkingv1beta1.VirtualService{
    ObjectMeta: metav1.ObjectMeta{
        Name:      vsName,
        Namespace: pod.Namespace,
        Labels: map[string]string{
            "app.kubernetes.io/managed-by":        "istio-http01",
            "acme.cert-manager.io/http01-solver":  "true",
            "acme.cert-manager.io/solver-pod":     pod.Name,
            "acme.cert-manager.io/solver-service": service.Name,
        },
        OwnerReferences: []metav1.OwnerReference{
            {
                APIVersion: "v1",
                Kind:       "Pod",
                Name:       pod.Name,
                UID:        pod.UID,
                Controller: func() *bool { b := true; return &b }(),
            },
        },
    },
    Spec: istioapinetworkingv1beta1.VirtualService{
        Hosts:    []string{domain},
        Gateways: []string{gatewayRef},
        Http: []*istioapinetworkingv1beta1.HTTPRoute{
            {
                Match: []*istioapinetworkingv1beta1.HTTPMatchRequest{
                    {
                        Uri: &istioapinetworkingv1beta1.StringMatch{
                            MatchType: &istioapinetworkingv1beta1.StringMatch_Prefix{
                                Prefix: "/.well-known/acme-challenge/",
                            },
                        },
                    },
                },
                Route: []*istioapinetworkingv1beta1.HTTPRouteDestination{
                    {
                        Destination: &istioapinetworkingv1beta1.Destination{
                            Host: fmt.Sprintf("%s.%s.svc.cluster.local", 
                                service.Name, service.Namespace),
                            Port: &istioapinetworkingv1beta1.PortSelector{
                                Number: solverPort,
                            },
                        },
                    },
                },
            },
        },
    },
}
```

**Структура создаваемого VirtualService:**
- **Hosts:** `["app.example.com"]` - домен для валидации
- **Gateways:** `["example-gateway-ns/example-gateway"]` - ссылка на Gateway
- **HTTP Route:**
  - **Match:** URI с префиксом `/.well-known/acme-challenge/`
  - **Route:** Направление на Service пода solver

#### 6.3.6 Установка Owner Reference

```go
// internal/controller/http01_solver_pod_controller.go:524-527
if err := ctrl.SetControllerReference(pod, virtualService, r.Scheme); err != nil {
    return fmt.Errorf("failed to set controller reference: %w", err)
}
```

**Зачем:** При удалении пода, VirtualService будет автоматически удален (garbage collection).

#### 6.3.7 Создание в Kubernetes

```go
// internal/controller/http01_solver_pod_controller.go:529-532
if err := r.Create(ctx, virtualService); err != nil {
    return fmt.Errorf("failed to create VirtualService: %w", err)
}
```

## Полный пример потока

### Сценарий: cert-manager создает под для валидации домена

1. **cert-manager создает под:**
   ```yaml
   name: cm-acme-http-solver-abc123
   namespace: istio-system
   labels:
     acme.cert-manager.io/http01-solver: "true"
   containers:
     - name: acmesolver
       args:
         - '--domain=app.example.com'
   ```

2. **Controller-runtime получает событие Create**

3. **Predicate проверяет:**
   - Имя начинается с `cm-acme-http-solver-` ✓
   - Метка `acme.cert-manager.io/http01-solver: "true"` ✓
   - Событие передается в Reconcile

4. **Reconcile извлекает домен:**
   - `domain = "app.example.com"`

5. **Поиск Gateway:**
   - Проверка всех Gateway по hosts
   - Найден Gateway `example-gateway` в namespace `example-gateway-ns` с host `"*"`

6. **Проверка VirtualService:**
   - Поиск VirtualService, связанного с Gateway и содержащего домен
   - VirtualService не найден (или найден, но не для HTTP01 solver)

7. **Создание VirtualService:**
   - Поиск Service для пода (по имени или ownerReferences)
   - Создание VirtualService с маршрутом на Service пода
   - VirtualService создан: `http01-solver-app-example-com`

8. **Результат:**
   - VirtualService создан и связан с Gateway
   - Трафик на `http://app.example.com/.well-known/acme-challenge/...` 
     будет направлен на под HTTP01 solver
   - Let's Encrypt может проверить домен

## Важные моменты

1. **Watch работает на уровне Kubernetes API:** Controller-runtime использует Watch API для получения событий в реальном времени
2. **Predicate фильтрует на уровне контроллера:** Только подходящие поды обрабатываются
3. **Поиск Gateway двухэтапный:** Сначала напрямую, затем через VirtualService
4. **Проверка VirtualService:** Ищется только VirtualService, специально созданный для HTTP01 solver
5. **Owner Reference:** VirtualService автоматически удаляется при удалении пода
6. **Service поиск:** Учитывает, что cert-manager может создать Service с другим именем

## Логирование

На каждом этапе оператор логирует важные события:

```go
logger.Info("HTTP01 Solver Pod detected", ...)
logger.Info("Challenge domain extracted", ...)
logger.Info("Found Gateway for HTTP01 solver", ...)
logger.Info("VirtualService already exists for HTTP01 solver", ...)
logger.Info("Created VirtualService for HTTP01 solver", ...)
```

Это позволяет отслеживать весь процесс в логах оператора.

