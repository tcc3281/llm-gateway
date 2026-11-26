# Architecture Code Review: `internal/plugin`
**Focus:** SOLID Principles Evaluation (ISP & DIP Deep Dive) & Design Pattern Improvements  
**Target Files:** `internal/plugin/plugin.go`, `internal/plugin/engine.go`, `internal/plugin/registry.go`, and `internal/plugin/builtin/*`  
**Date:** September 16, 2026  

---

## 1. Executive Summary

An architectural review of the `internal/plugin` subsystem was conducted to evaluate its adherence to Object-Oriented Programming (OOP) best practices and the **SOLID** design principles. 

While the system provides basic pluggability through lifecycle hooks (`PreAuth`, `PostAuth`, `PreRouting`, `PostUpstream`) and multi-tier scope resolution (`global`, `tenant`, `api_key`, etc.), the internal implementation exhibits **critical architectural coupling**. Most notably, it suffers from severe violations of the **Interface Segregation Principle (ISP)** and the **Dependency Inversion Principle (DIP)**.

### Summary Assessment

| Principle | Status | Severity | Primary Symptoms |
| :--- | :--- | :--- | :--- |
| **I** - Interface Segregation | **VIOLATED** | **Critical** | Monolithic `PluginContext` (20+ fields) is forced upon all plugin hooks regardless of phase; plugins access fields, DB handles, and transport writers they do not need. |
| **D** - Dependency Inversion | **VIOLATED** | **Critical** | Low-level concrete types (`*db.RedisClient`, `*db.PostgresDB`, `http.ResponseWriter`) are directly embedded in `PluginContext`; plugins instantiate concrete dependencies internally instead of using constructor injection. |
| **S** - Single Responsibility | **VIOLATED** | **High** | `PluginEngine` combines binding cache management, DB fallback, Redis pub/sub sync, async goroutine dispatch, and pipeline execution. Builtin plugins mix domain logic, storage access, and HTTP wire formatting. |
| **O** - Open/Closed | **VIOLATED** | **Medium/High** | Adding new lifecycle phases requires modifying `PluginEngine`, `BasePlugin`, and callers; hardcoded switch statements in scope specificity; untyped `map[string]any` configuration parsing. |
| **L** - Liskov Substitution | **VIOLATED** | **High** | `AsyncPostUpstreamPlugin` marker changes runtime execution semantics, swallows errors, and introduces data races; inconsistent halt contract (`pctx.Stop` vs. direct `ResponseWriter` writes vs. returning `error`). |

---

## 2. In-Depth SOLID Principle Violations

### 2.1. Interface Segregation Principle (ISP) — *Severity: Critical*
> *"Clients should not be forced to depend upon interfaces or data contracts that they do not use."*

#### The "God Context" Anti-Pattern: `PluginContext`
In [plugin.go](file:///data/per-pro/llm-gateway/internal/plugin/plugin.go#L23-L48), `PluginContext` acts as a monolithic data bucket holding every conceivable piece of state across the request lifecycle:

```go
type PluginContext struct {
    Ctx            context.Context
    APIKey         *model.APIKey
    User           *model.User
    Tenant         *model.Tenant
    CanReq         *canonical.CanonicalRequest
    RequestBody    []byte
    InboundFormat  string
    Model          string
    Provider       string
    ClientIP       string
    StartTime      time.Time
    Duration       time.Duration
    StatusCode     int
    ResponseWriter http.ResponseWriter
    Redis          *db.RedisClient
    Postgres       *db.PostgresDB

    // Mutable properties manipulated by plugins:
    ExtraHeaders   map[string]string
    RemoveHeaders  []string
    Stop           bool
    StopStatusCode int
    StopErrorBody  []byte
}
```

Every lifecycle interface requires this exact struct:
- `PreAuthPlugin.ExecPreAuth(pctx *PluginContext, cfg map[string]any) error`
- `PostAuthPlugin.ExecPostAuth(pctx *PluginContext, cfg map[string]any) error`
- `PreRoutingPlugin.ExecPreRouting(pctx *PluginContext, cfg map[string]any) error`
- `PostUpstreamPlugin.ExecPostUpstream(pctx *PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error`

#### Violation Analysis & Blast Radius
1. **Unnecessary Coupling & State Leaks:**
   - [ip_restriction.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/ip_restriction.go#L28-L66) (`ExecPreAuth`) only needs `ClientIP` and a way to block the request. Yet it receives full pointers to `Redis`, `Postgres`, `APIKey`, `User`, `Tenant`, and `ResponseWriter`.
   - [prompt_guard.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/prompt_guard.go#L42-L127) (`ExecPreRouting`) only inspects `CanReq.Messages` and `CanReq.SystemPrompt`. Yet it is exposed to internal database connections and raw request bodies.
   - [prometheus_metrics.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/prometheus_metrics.go#L75-L147) (`ExecPostUpstream`) only reads telemetry metadata, but receives mutable control-flow flags (`Stop`, `StopStatusCode`) and mutation fields (`ExtraHeaders`, `RemoveHeaders`).
2. **Invalid State Exposure Across Lifecycle Phases:**
   - In `PreAuth` plugins, `pctx.APIKey`, `pctx.User`, and `pctx.Tenant` are always `nil` because authentication hasn't occurred.
   - `pctx.Duration` and `pctx.StatusCode` are unpopulated during `PreAuth`, `PostAuth`, and `PreRouting`.
   - Forcing plugins to accept fields that are invalid or nil in their stage leads to defensive programming (`if pctx.CanReq == nil`, `if pctx.APIKey != nil`) and runtime `nil` pointer panics.
3. **Security & Integrity Risks:**
   - Any third-party or internal plugin has write access to mutate `pctx.Postgres` or rewrite `pctx.APIKey` permissions before routing occurs.

---

### 2.2. Dependency Inversion Principle (DIP) — *Severity: Critical*
> *"High-level modules should not depend on low-level modules. Both should depend on abstractions. Abstractions should not depend on details. Details should depend on abstractions."*

#### Concrete Infrastructure Leaks
The plugin abstraction layer (`internal/plugin`) directly imports and couples to concrete infrastructure packages:

```go
// internal/plugin/plugin.go
import (
    "llm-gateway/internal/db"     // Concrete DB package!
    "net/http"                    // Transport layer!
)

type PluginContext struct {
    ResponseWriter http.ResponseWriter // Concrete HTTP transport
    Redis          *db.RedisClient     // Concrete Redis struct
    Postgres       *db.PostgresDB      // Concrete Postgres struct
    ...
}
```

#### Code Evidence: Plugins Direct Access to Concrete DBs
1. **`RateLimitingPlugin` ([rate_limiting.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/rate_limiting.go#L39)):**
   ```go
   allowed, err := pctx.Redis.AllowRequest(pctx.Ctx, fmt.Sprintf("plugin:rate_limiting:%s", keyIdentifier), rpm)
   ```
   The plugin depends directly on `*db.RedisClient`. It cannot be used with an in-memory rate limiter, an Envoy/Redis cluster, or an alternative key-value store.
2. **`BudgetControlPlugin` ([budget_control.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/budget_control.go#L57-L67)):**
   ```go
   cachedSpent, cErr := pctx.Redis.GetCachedBudgetSpent(pctx.Ctx, keyID)
   ...
   spent, err = pctx.Postgres.GetSpentCostByKeyID(pctx.APIKey.ID)
   ...
   _ = pctx.Redis.SetCachedBudgetSpent(pctx.Ctx, keyID, spent, 60*time.Second)
   ```
   `BudgetControlPlugin` reaches directly into both concrete PostgreSQL and concrete Redis clients!
3. **`SemanticCachePlugin` ([semantic_cache.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/semantic_cache.go#L37,L63)):**
   ```go
   cachedData, err := pctx.Redis.GetRaw(pctx.Ctx, cacheKey)
   ...
   _ = pctx.Redis.SetRaw(pctx.Ctx, cacheKey, respBytes, time.Duration(ttlSeconds)*time.Second)
   ```

#### Consequence: Untestable Architecture
In [builtin_test.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/builtin_test.go), unit tests exist for `pii-masking`, `prompt-guard`, `header-transform`, `ip-restriction`, `canary`, and `watermarking`. 

However, **there are zero unit tests for `RateLimitingPlugin`, `SemanticCachePlugin`, `RetryCircuitBreakerPlugin`, or `BudgetControlPlugin` with real storage**. Because `pctx.Redis` and `pctx.Postgres` are concrete pointers (and not interfaces), writing mock unit tests without booting a Docker container or live database is virtually impossible.

#### Internal Dependency Instantiation (Hidden Dependencies)
In [audit_trail.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/audit_trail.go#L20-L22) and [webhook_notifier.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/webhook_notifier.go#L20-L22):
```go
func NewAuditTrailPlugin() *AuditTrailPlugin {
    return &AuditTrailPlugin{
        httpClient: &http.Client{Timeout: 5 * time.Second},
    }
}
```
Plugins instantiate hardcoded concrete `&http.Client{}` instances internally. They cannot be injected with custom transports, TLS configs, retry policies, or mock HTTP clients for testing.

---

### 2.3. Single Responsibility Principle (SRP) — *Severity: High*
> *"A module should have one, and only one, reason to change."*

1. **`PluginEngine` ([engine.go](file:///data/per-pro/llm-gateway/internal/plugin/engine.go)):**
   Accumulates at least 5 distinct concerns:
   - **Local In-Memory Cache Management:** `cachedBindings`, `lastLoaded`, `cacheTTL`, `ReloadBindings()`, `InvalidateCache()`.
   - **Background Synchronization:** `StartBackgroundSync()` runs a ticker routine.
   - **Distributed Event Messaging:** `StartRedisSubscriber()` listens on Redis Pub/Sub channels.
   - **Scope Resolution & Specificity Ranking:** Evaluates hierarchical rules in `ResolveEffectiveBindings()`.
   - **Hook Dispatch & Concurrency Control:** Iterates bindings, handles asynchronous goroutine dispatch for observability, and manages timeouts.
   *Impact:* Changes to caching policies, Redis pub/sub topics, scope hierarchy, or lifecycle hook dispatch all force modifications to `engine.go`.

2. **Builtin Plugins Violating SRP:**
   - **`SemanticCachePlugin` ([semantic_cache.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/semantic_cache.go#L42-L50)):**
     Instead of just checking a cache store, it formats HTTP headers (`Content-Type`, `X-Cache`), writes directly to `pctx.ResponseWriter`, writes status code `http.StatusOK`, and sets `pctx.Stop = true`. It conflates cache querying with HTTP transport output streaming.
   - **`BudgetControlPlugin` ([budget_control.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/budget_control.go#L50-L89)):**
     Evaluates tenant/key budget hierarchies, queries database and Redis cache, formats JSON HTTP error bodies, and writes warnings to HTTP headers.

---

### 2.4. Open/Closed Principle (OCP) — *Severity: Medium/High*
> *"Software entities should be open for extension, but closed for modification."*

1. **Rigid Lifecycle Pipeline:**
   `HookType` is closed: `HookPreAuth`, `HookPostAuth`, `HookPreRouting`, `HookPostUpstream`.
   `PluginEngine` contains hardcoded runner methods: `RunPreAuth`, `RunPostAuth`, `RunPreRouting`, `RunPostUpstream`.
   Adding a new stage (e.g. `PreUpstream` after load balancer selection, `OnStreamChunk` for streaming response tokens, `OnError` for fallback interceptors) requires:
   - Adding a new `HookType` constant.
   - Defining a new interface (`PreUpstreamPlugin`).
   - Adding a new runner method (`RunPreUpstream`) in `PluginEngine`.
   - Modifying `gateway.go` call sites.
   It cannot be extended without modifying core engine code.

2. **Hardcoded Scope Resolution ([engine.go](file:///data/per-pro/llm-gateway/internal/plugin/engine.go#L167-L175,L201-L228)):**
   Scope specificity ranks and scope matching logic are hardcoded via a static map (`scopeSpecificity`) and a rigid `switch b.ScopeLevel` statement. Adding a new organizational scope level (e.g., `workspace`, `organization`, `environment`) requires modifying `ResolveEffectiveBindings`.

3. **Untyped Dynamic Configurations (`map[string]any`):**
   Every plugin relies on [helpers.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/helpers.go) to dynamically unpack `cfg map[string]any` using type assertions (`getIntConfig`, `getStringConfig`). There is no typed configuration validation, schema introspection, or strongly typed configuration strategy.

---

### 2.5. Liskov Substitution Principle (LSP) — *Severity: High*
> *"Objects of a superclass should be replaceable with objects of a subclass without affecting the correctness of the program."*

#### 1. Inconsistent Halt and Termination Contracts
When a plugin decides to block or short-circuit a request, the mechanism is inconsistent across plugins:
- **`IPRestrictionPlugin` / `PromptGuardPlugin`:** Mutate `pctx.Stop = true`, `pctx.StopStatusCode = 400`, `pctx.StopErrorBody = []byte(...)`, and return `nil`.
- **`SemanticCachePlugin`:** Directly calls `pctx.ResponseWriter.WriteHeader(200)` and `pctx.ResponseWriter.Write(cachedData)`, sets `pctx.Stop = true`, and returns `nil`.
- **General Error Handling:** Other plugins might return a non-nil `error` from `Exec*`, which causes `PluginEngine` to immediately return that error.
If a caller substitutes one blocking plugin for another, the side effects on `http.ResponseWriter` and the gateway flow are radically different and unpredictable.

#### 2. `AsyncPostUpstreamPlugin` Behavioral Fracture ([plugin.go](file:///data/per-pro/llm-gateway/internal/plugin/plugin.go#L74-L79), [engine.go](file:///data/per-pro/llm-gateway/internal/plugin/engine.go#L368-L384))
```go
type AsyncPostUpstreamPlugin interface {
    IsAsyncPostUpstream() bool
}
```
`PluginEngine.RunPostUpstream` inspects whether the plugin implements `AsyncPostUpstreamPlugin`:
- If `true`: It executes `targetPlugin.ExecPostUpstream` inside a background goroutine, **completely ignores any returned error** (`_ = ...`), and returns immediately.
- If `false`: It executes synchronously in-line and aborts on error.

This violates LSP because implementing this interface breaks the contract of `PostUpstreamPlugin`:
1. The caller cannot rely on execution guarantees or error propagation.
2. Any mutations to `pctx` inside an async plugin are silently discarded.
3. **Data Race Danger:** The engine creates a shallow copy `pctxCopy := *pctx`. However, pointers (`pctxCopy.ExtraHeaders`, `pctxCopy.CanReq`, `pctxCopy.ResponseWriter`) point to the **same underlying memory** as the main request goroutine. If an async plugin reads/writes `ExtraHeaders` or `CanReq` while `gateway.go` is serializing the response, Go's race detector triggers data race errors.

---

## 3. Concurrency & Architectural Defects in Builtin Plugins

Beyond pure OOP principles, the review identified several critical implementation defects in `internal/plugin/builtin`:

1. **Race Condition in `CanaryTrafficSplitterPlugin` ([canary_traffic_splitter.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/canary_traffic_splitter.go#L13,L49)):**
   ```go
   type CanaryTrafficSplitterPlugin struct {
       rng *rand.Rand // NOT concurrency-safe!
   }
   func (p *CanaryTrafficSplitterPlugin) ExecPreRouting(...) error {
       roll := p.rng.Intn(100) // DATA RACE on concurrent requests!
   }
   ```
   `math/rand.Rand` is not thread-safe. Concurrent requests calling `ExecPreRouting` on the singleton plugin instance will corrupt internal RNG state or panic.
2. **Double Asynchronous Goroutine Spawning:**
   In [webhook_notifier.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/webhook_notifier.go#L33,L103) and [audit_trail.go](file:///data/per-pro/llm-gateway/internal/plugin/builtin/audit_trail.go#L33,L65):
   Both plugins implement `IsAsyncPostUpstream() bool { return true }` (so `PluginEngine` already spawns a background goroutine). Inside `ExecPostUpstream`, they execute `go func(...)` *again*! The outer goroutine finishes instantly and cancels its context (`defer cancel()`), while the un-tracked inner goroutine runs with a detached `context.Background()`.

---

## 4. Recommended Design Patterns & Target Architecture

To resolve the ISP and DIP violations while establishing a clean, extensible plugin architecture, four core design patterns should be introduced:

```
+-------------------------------------------------------------------------------+
|                             Gateway Request Pipeline                          |
+-------------------------------------------------------------------------------+
                                      |
                                      v
   +-------------------------------------------------------------------------+
   |             Pattern 1: Chain of Responsibility (Interceptor)           |
   |                                                                         |
   |   [AuthInterceptor] -> [RoutingInterceptor] -> [Upstream Execution]    |
   +-------------------------------------------------------------------------+
                                      |
             +------------------------+------------------------+
             |                                                 |
             v                                                 v
+-----------------------------+               +---------------------------------+
| Pattern 2: ISP Segregated   |               | Pattern 3: Dependency Inversion |
| Request & Filter Contexts   |               | & Strategy Pattern              |
|                             |               |                                 |
| - PreAuthContext (ClientIP) |               | - RateLimiter interface         |
| - PreRoutingContext (Prompt)|               | - CacheStore interface          |
| - Action: Continue / Halt   |               | - Injected via constructors     |
+-----------------------------+               +---------------------------------+
                                      |
                                      v
   +-------------------------------------------------------------------------+
   |                Pattern 4: Observer / Event Bus (Pub-Sub)                |
   |                                                                         |
   |              Publish Event: RequestCompletedEvent                       |
   |     +-------------------+-------------------+--------------------+      |
   |     v                   v                   v                    v      |
   | [PrometheusMetrics] [LangfuseTrace]  [AuditTrailLogger] [WebhookNotifier]|
   +-------------------------------------------------------------------------+
```

### Pattern 1: Chain of Responsibility (Interceptor Pipeline)
Instead of hardcoding phase runners (`RunPreAuth`, `RunPostAuth`), treat request filtering as an interceptor pipeline (similar to gRPC interceptors or HTTP middleware).
- Each interceptor decides whether to modify the request, halt execution cleanly via a standard `ExecutionResult`, or delegate to `next()`.

### Pattern 2: Role-Based Interface Segregation (ISP Solution)
Segregate `PluginContext` into phase-specific, read-only interfaces paired with narrow action controllers:
- `PreAuthContext`: Provides `ClientIP()` and `Header(key)`.
- `PreRoutingContext`: Provides `CanonicalRequest()`, `Model()`, `User()`, `Tenant()`.
- `ExecutionController`: Exposes `Halt(statusCode int, err error)` and `SetHeader(k, v string)`.
Plugins never receive database pointers or transport writer objects.

### Pattern 3: Dependency Inversion via Interfaces & Constructor Injection (DIP Solution)
Extract storage requirements into focused domain interfaces:
- `RateLimiter`: `Allow(ctx, key string, limit int) (bool, error)`
- `CacheStore`: `Get(ctx, key string) ([]byte, bool, error)` and `Set(ctx, key string, val []byte, ttl time.Duration) error`
- `CostCalculator`: `GetSpent(ctx, tenantID, keyID uuid.UUID) (float64, error)`
Inject these interfaces into the plugin structs when registering them in the registry:
`NewRateLimitingPlugin(limiter RateLimiter)`

### Pattern 4: Observer / Event-Driven Architecture (Observability Plugins)
Separate **mutating plugins** (which alter requests/responses) from **observability plugins** (metrics, logging, traces, webhooks).
- Observability plugins should not be interceptors. They are **Observers** that subscribe to `RequestCompletedEvent`.
- An in-memory or worker-pool `EventBus` broadcasts events asynchronously without hacking marker interfaces (`IsAsyncPostUpstream`) or risking data races on shared request pointers.

---

## 5. Concrete Refactoring Code Blueprints

### Refactoring 1: Decoupling Storage Dependencies (DIP)

#### Before: Concrete Redis coupling in `RateLimitingPlugin`
```go
// Direct dependency on internal concrete database package
type RateLimitingPlugin struct{}

func (p *RateLimitingPlugin) ExecPostAuth(pctx *plugin.PluginContext, cfg map[string]any) error {
    rpm := getIntConfig(cfg, "requests_per_minute", 0)
    // Concrete dependency embedded inside context:
    allowed, err := pctx.Redis.AllowRequest(pctx.Ctx, "key", rpm)
    ...
}
```

#### After: Inverted dependency using `RateLimiter` interface
```go
// 1. Domain abstraction owned by plugin layer
type RateLimiter interface {
    Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// 2. Plugin depends strictly on abstraction; injected via constructor
type RateLimitingPlugin struct {
    limiter RateLimiter
}

func NewRateLimitingPlugin(limiter RateLimiter) *RateLimitingPlugin {
    return &RateLimitingPlugin{limiter: limiter}
}

func (p *RateLimitingPlugin) Execute(ctx PreRoutingContext, ctrl ActionController, cfg RateLimitConfig) error {
    if cfg.RequestsPerMinute <= 0 {
        return nil
    }
    allowed, err := p.limiter.Allow(ctx.Context(), ctx.IdentityKey(), cfg.RequestsPerMinute, time.Minute)
    if err != nil {
        return nil // Fail open
    }
    if !allowed {
        ctrl.Halt(http.StatusTooManyRequests, errors.New("rate limit exceeded"))
    }
    return nil
}
```
*Benefits:* `RateLimitingPlugin` can be unit tested with a 5-line mock struct in `builtin_test.go` without running Redis.

---

### Refactoring 2: Segregating Plugin Contexts (ISP)

#### Before: Monolithic context passed to every hook
```go
// 20+ fields, mutable pointers, DB connections exposed to all
func (p *IPRestrictionPlugin) ExecPreAuth(pctx *plugin.PluginContext, cfg map[string]any) error {
    clientIP := pctx.ClientIP
    // pctx has pctx.Postgres, pctx.Redis, pctx.ResponseWriter, etc.
}
```

#### After: Role-focused Segregated Contexts
```go
// Read-only inspection interface for PreAuth phase
type PreAuthContext interface {
    Context() context.Context
    ClientIP() net.IP
    Header(key string) string
}

// Write-only / control interface for pipeline intervention
type PipelineController interface {
    HaltWithStatus(statusCode int, errorCode string, message string)
    AddHeader(key, value string)
    IsHalted() bool
}

// Segregated plugin contract
type PreAuthPlugin interface {
    Name() string
    ExecPreAuth(ctx PreAuthContext, ctrl PipelineController, cfg map[string]any) error
}

// Implementation only accesses what it declares:
func (p *IPRestrictionPlugin) ExecPreAuth(ctx PreAuthContext, ctrl PipelineController, cfg map[string]any) error {
    ip := ctx.ClientIP()
    if p.isBlocked(ip) {
        ctrl.HaltWithStatus(http.StatusForbidden, "ip_forbidden", "Access denied by IP policy")
    }
    return nil
}
```
*Benefits:* Plugins cannot accidentally modify database state, mutate response writers directly, or experience nil-pointer bugs from uninitialized downstream fields.

---

### Refactoring 3: Observer Pattern for Observability Plugins

#### Before: Marker interface and shallow-copy background goroutine
```go
// engine.go
if asyncP, isAsync := inst.(AsyncPostUpstreamPlugin); isAsync && asyncP.IsAsyncPostUpstream() {
    pctxCopy := *pctx // Shallow copy shares maps and pointers!
    go func(...) {
        _ = targetPlugin.ExecPostUpstream(&cCtx, rCopy, cConfig) // Error swallowed!
    }(...)
}
```

#### After: Clean Domain Event and Observer Subscription
```go
// Immutable Domain Event generated when an LLM request completes
type RequestCompletedEvent struct {
    Timestamp        time.Time
    Duration         time.Duration
    StatusCode       int
    Model            string
    Provider         string
    TenantID         string
    APIKeyPrefix     string
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
    Error            error
}

// Observer contract
type RequestObserver interface {
    OnRequestCompleted(event RequestCompletedEvent)
}

// Event Dispatcher (Worker Pool or Channel)
type EventBus struct {
    observers []RequestObserver
    eventChan chan RequestCompletedEvent
}

func (b *EventBus) Publish(event RequestCompletedEvent) {
    select {
    case b.eventChan <- event:
    default:
        // Prevent blocking hot-path if buffer full; log metric
    }
}

// Prometheus plugin is simply an observer
type PrometheusObserver struct{}

func (p *PrometheusObserver) OnRequestCompleted(e RequestCompletedEvent) {
    promRequestsTotal.WithLabelValues(e.TenantID, e.Model, e.Provider, strconv.Itoa(e.StatusCode)).Inc()
    promRequestDuration.WithLabelValues(e.TenantID, e.Model, e.Provider, strconv.Itoa(e.StatusCode)).Observe(e.Duration.Seconds())
}
```
*Benefits:* Eliminates all goroutine data races, eliminates double `go func` anti-patterns, decouples observability from the synchronous request path, and prevents metrics/webhook failures from impacting client traffic.

---

## 6. Actionable Implementation Roadmap

```
Phase 1: Urgent Bug Fixes (Sprint 1)
  ├── 1. Fix CanaryTrafficSplitterPlugin data race (replace rand.Rand with rand/v2 or sync.Mutex)
  └── 2. Remove double goroutine spawning in WebhookNotifier & AuditTrail

Phase 2: Dependency Inversion (Sprint 2)
  ├── 1. Define RateLimiter, CacheStore, and QuotaReader interfaces in internal/plugin
  ├── 2. Update Builtin Plugin constructors to accept these interfaces
  └── 3. Implement comprehensive unit tests in builtin_test.go using mocks

Phase 3: Interface Segregation & Context Refactoring (Sprint 3)
  ├── 1. Split PluginContext into PreAuthContext, PreRoutingContext, PostUpstreamContext
  ├── 2. Introduce PipelineController to replace mutable Stop/StopStatusCode/StopErrorBody
  └── 3. Remove concrete *db.RedisClient, *db.PostgresDB, and http.ResponseWriter from PluginContext

Phase 4: Event-Driven Observability (Sprint 4)
  ├── 1. Create RequestCompletedEvent and EventBus
  ├── 2. Migrate Prometheus, Langfuse, AuditTrail, and Webhook plugins to Event Observers
  └── 3. Deprecate AsyncPostUpstreamPlugin marker interface
```
