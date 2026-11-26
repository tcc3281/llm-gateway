# Code Review Report: SOLID & Design Patterns

## Scope
- **Target Package:** `internal/plugin` and builtin plugins in `internal/plugin/builtin`
- **Language:** Go (1.22+)
- **Existing Architectural Patterns Noted:**
  - **Registry Pattern:** In-memory registration system (`Registry`, `defaultRegistry`, `RegisterGlobal`)
  - **Lifecycle Hook Abstraction:** Phase-specific plugin interfaces (`PreAuthPlugin`, `PostAuthPlugin`, `PreRoutingPlugin`, `PostUpstreamPlugin`)
  - **Single-Run Override Engine:** Specificity-based resolution for multi-scope bindings (`ResolveEffectiveBindings`)
  - **Hybrid In-Memory Cache:** Double-checked locking cache with Redis PubSub invalidation embedded inside `PluginEngine`

---

## SOLID Audit

### ⚠️ S — Single Responsibility Principle

#### 1. `PluginEngine` is a God Class with Mixed Infrastructure and Orchestration Concerns
`internal/plugin/engine.go:24-165, 294-396`

`PluginEngine` currently has at least five distinct reasons to change:
1. **Cache storage & invalidation policy:** Managing `cachedBindings`, `lastLoaded`, `cacheTTL`, and double-checked mutex locking.
2. **Background sync & polling:** Managing a background ticker loop in `StartBackgroundSync`.
3. **Redis PubSub message consumption:** Managing distributed cache eviction in `StartRedisSubscriber`.
4. **Scope resolution & precedence calculation:** Managing binding filtering and precedence sorting in `ResolveEffectiveBindings`.
5. **Pipeline execution & concurrency management:** Iterating through hooks, executing plugins, short-circuiting on `pctx.Stop`, and spawning asynchronous background goroutines with detached contexts for observability plugins.

**Before (`internal/plugin/engine.go:24-48`):**
```go
// ❌ Violates SRP: Engine mixes caching, pubsub subscription, database querying,
// and pipeline execution into one monolithic struct.
type PluginEngine struct {
	pg             PostgresReader
	registry       *Registry
	mu             sync.RWMutex
	cachedBindings []model.PluginBinding
	lastLoaded     time.Time
	cacheTTL       time.Duration
}

func (e *PluginEngine) ReloadBindings(ctx context.Context) error { ... }
func (e *PluginEngine) GetActiveBindings(ctx context.Context) ([]model.PluginBinding, error) { ... }
func (e *PluginEngine) StartBackgroundSync(ctx context.Context, interval time.Duration) { ... }
func (e *PluginEngine) StartRedisSubscriber(ctx context.Context, redisClient *db.RedisClient) { ... }
func (e *PluginEngine) RunPreAuth(pctx *PluginContext, bindings []model.PluginBinding) error { ... }
func (e *PluginEngine) RunPostUpstream(pctx *PluginContext, resp *canonical.CanonicalResponse, bindings []model.PluginBinding) error { ... }
```

**After:**
```go
// ✅ SRP Refactor: Separate storage/caching from execution orchestration.

// 1. BindingRepository handles retrieval and caching
type BindingRepository interface {
	GetActiveBindings(ctx context.Context) ([]model.PluginBinding, error)
	Reload(ctx context.Context) error
}

// 2. ScopeResolver handles rule evaluation and specificity ranking
type ScopeResolver interface {
	ResolveEffective(all []model.PluginBinding, req *RequestContext) []model.PluginBinding
}

// 3. PluginEngine focuses strictly on pipeline execution
type PluginEngine struct {
	repo     BindingRepository
	registry PluginRegistry
	resolver ScopeResolver
}

func (e *PluginEngine) ExecutePreAuth(ctx context.Context, pctx *PluginContext) error {
	// Single responsibility: execute plugins matching resolved bindings
	bindings, err := e.repo.GetActiveBindings(ctx)
	if err != nil {
		return err
	}
	effective := e.resolver.ResolveEffective(bindings, pctx.ToRequestContext())
	return e.runHook(pctx, effective, HookPreAuth)
}
```

#### 2. `PluginContext` is a Monolithic God Object
`internal/plugin/plugin.go:24-48`

`PluginContext` mixes low-level transport handles (`http.ResponseWriter`), infrastructure database drivers (`*db.RedisClient`, `*db.PostgresDB`), IAM identity (`*model.User`, `*model.Tenant`, `*model.APIKey`), canonical request payloads, performance metrics (`StartTime`, `Duration`), and pipeline control flags (`Stop`, `StopStatusCode`, `StopErrorBody`). Any change to auth, routing, telemetry, or storage forces modifications to `PluginContext`.

---

### ⚠️ O — Open/Closed Principle

#### 1. Scope Specificity and Matching Are Hardcoded in Switch and Map
`internal/plugin/engine.go:167-175, 201-229`

The binding resolution logic in `ResolveEffectiveBindings` evaluates scopes using a hardcoded `switch b.ScopeLevel` and a static `scopeSpecificity` map. Adding a new scope level (e.g. `workspace`, `client_ip_range`, `tag`, or `organization`) requires modifying `PluginEngine` directly instead of extending it.

**Before (`internal/plugin/engine.go:167-175, 201-217`):**
```go
// ❌ Violates OCP: Hardcoded map and switch statement. Adding new scopes requires modifying engine.go.
var scopeSpecificity = map[string]int{
	"global":   1,
	"provider": 2,
	"model":    3,
	"tenant":   4,
	"role":     5,
	"user":     6,
	"api_key":  7,
}

switch b.ScopeLevel {
case "global":
	matched = true
case "provider":
	if b.ScopeProvider != nil && provider != "" && *b.ScopeProvider == provider {
		matched = true
	}
case "tenant":
	if b.ScopeTenantID != nil && tenantID != nil && *b.ScopeTenantID == *tenantID {
		matched = true
	}
// ...
}
```

**After:**
```go
// ✅ OCP Refactor: Strategy Pattern for Scope Evaluation.
type ScopeEvaluator interface {
	ScopeLevel() string
	Specificity() int
	Matches(binding model.PluginBinding, req *RequestContext) bool
}

type ScopeRegistry struct {
	evaluators map[string]ScopeEvaluator
}

func (r *ScopeRegistry) Register(evaluator ScopeEvaluator) {
	r.evaluators[evaluator.ScopeLevel()] = evaluator
}

func (r *ScopeRegistry) Evaluate(b model.PluginBinding, req *RequestContext) (bool, int) {
	eval, ok := r.evaluators[b.ScopeLevel]
	if !ok {
		return false, 0
	}
	return eval.Matches(b, req), eval.Specificity()
}
// ℹ️ New scopes (e.g., WorkspaceEvaluator) can now be added without editing core engine code.
```

#### 2. Duplicated Hook Execution Loops Across 4 Hardcoded Lifecycle Methods
`internal/plugin/engine.go:294-396`

`RunPreAuth`, `RunPostAuth`, `RunPreRouting`, and `RunPostUpstream` repeat the same loop structure: fetch plugin by name, assert hook interface, parse JSON configuration, call execution method, and inspect `pctx.Stop`. Adding a new lifecycle hook (such as `OnStreamChunk`, `OnError`, or `PreFallback`) requires modifying `PluginEngine` with yet another duplicate loop method.

---

### ⚠️ L — Liskov Substitution Principle

#### 1. Untyped Registry Accepts Arbitrary Types Without Contract Validation
`internal/plugin/registry.go:11, 31-43`

`Registry.Register(name string, instance any)` accepts `any`. Any invalid struct or scalar can be registered without compile-time or registration-time interface validation. At runtime, `engine.go` performs type assertions (`inst.(PreAuthPlugin)`). If an instance does not implement the expected hook interface, it is silently ignored without error, warning, or fallback.

**Before (`internal/plugin/registry.go:11, 31-35`):**
```go
// ❌ Violates LSP & Type Safety: plugins map stores untyped `any`.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]any
}

func (r *Registry) Register(name string, instance any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[name] = instance // ⚠️ Accepts anything, e.g. Register("guard", 123)
}
```

**After:**
```go
// ✅ LSP Refactor: Enforce BasePlugin contract at registration time.
type Plugin interface {
	Name() string
}

type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return errors.New("cannot register nil plugin")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name()] = p
	return nil
}
```

#### 2. Async Plugins Violate the Execution Contract of `PostUpstreamPlugin`
`internal/plugin/plugin.go:74-79`, `internal/plugin/engine.go:368-384`

`AsyncPostUpstreamPlugin` marker interface indicates that a post-upstream plugin is pure observability and should run in the background without delaying the client response. However, `ExecPostUpstream` receives `*PluginContext`, which contains mutable control state (`pctx.Stop`, `pctx.ExtraHeaders`, `pctx.ResponseWriter`).
- A synchronous `PostUpstreamPlugin` can modify response headers or mutate canonical responses.
- An `AsyncPostUpstreamPlugin` running in a detached goroutine cannot modify headers or stop the request because the HTTP response has already been flushed to the client.
- Subtyping `AsyncPostUpstreamPlugin` therefore cannot safely substitute for `PostUpstreamPlugin` under the same signature without silently dropping mutations or inducing race conditions.

#### 3. Unused `BasePlugin.Hooks()` Creates Misleading Implementation Invariants
`internal/plugin/plugin.go:81-85`

`BasePlugin` declares `Hooks() []HookType`, and multiple plugins in `builtin/` implement it (e.g. `rate_limiting.go:19`, `prompt_guard.go:38`). However, `PluginEngine` never queries `Hooks()`. It only performs dynamic type assertions on the hook interfaces (`inst.(PreAuthPlugin)`). If a plugin declares `Hooks() []HookType{HookPreRouting}` but accidentally omits `ExecPreRouting`, `Hooks()` promises a capability the struct does not provide.

---

### ⚠️ I — Interface Segregation Principle

#### 1. Massive `PluginContext` Passed to Every Plugin Hook
`internal/plugin/plugin.go:24-72`

Every hook interface accepts the entire `*PluginContext` struct:
```go
ExecPreRouting(pctx *PluginContext, cfg map[string]any) error
```
- A pure validation plugin like `PromptGuardPlugin` or `ContentModerationPlugin` only needs to read `pctx.CanReq.Messages` and set `pctx.Stop`. Yet it is given access to `pctx.Postgres`, `pctx.Redis`, `pctx.ResponseWriter`, and `pctx.APIKey`.
- A header transformation plugin like `HeaderTransformPlugin` only needs to inspect and mutate headers, but receives database handles and request bodies.
- Clients are forced to depend on fields and capabilities they do not require.

**Before (`internal/plugin/plugin.go:50-72`):**
```go
// ❌ Violates ISP: Monolithic PluginContext forced into all hook signatures.
type PreRoutingPlugin interface {
	Name() string
	ExecPreRouting(pctx *PluginContext, cfg map[string]any) error
}
```

**After:**
```go
// ✅ ISP Refactor: Narrow, segregated context interfaces per capability.

type RequestReader interface {
	Model() string
	Messages() []canonical.ChatMessage
	SystemPrompt() string
	Header(key string) string
}

type HeaderModifier interface {
	SetHeader(key, value string)
	RemoveHeader(key string)
}

type PipelineController interface {
	Halt(statusCode int, errResponse any)
}

// PreRoutingPlugin only requires request inspection and flow control
type PreRoutingPlugin interface {
	Name() string
	ExecPreRouting(req RequestReader, ctrl PipelineController, cfg map[string]any) error
}
```

#### 2. `PostgresReader` Interface is Named After a Concretion
`internal/plugin/engine.go:19-21`

```go
type PostgresReader interface {
	ListActivePluginBindings() ([]model.PluginBinding, error)
}
```
While the interface itself is segregated and small (1 method), its name violates the spirit of interface design by baking the storage technology name (`Postgres`) into the consumer's interface. If bindings are loaded from Redis, MongoDB, or an Admin API, the type `PostgresReader` becomes an oxymoron. It should be `BindingStore` or `BindingReader`.

---

### ⚠️ D — Dependency Inversion Principle

#### 1. High-Level Plugin Implementations Directly Depend on Low-Level Database Structs
`internal/plugin/plugin.go:39-40`, `builtin/rate_limiting.go:25-39`, `builtin/budget_control.go:56-68`, `builtin/semantic_cache.go:37-63`

`PluginContext` directly embeds pointers to concrete database clients:
```go
Redis    *db.RedisClient
Postgres *db.PostgresDB
```
Builtin plugins directly invoke concrete methods on `pctx.Redis` (`AllowRequest`, `GetCachedBudgetSpent`, `GetRaw`, `SetRaw`) and `pctx.Postgres` (`GetSpentCostByKeyID`).

**Consequences:**
- Plugins cannot be tested without real database instances or complex test fixtures.
- In `internal/plugin/builtin/builtin_test.go:154-178` (`TestBudgetControlPlugin_Exceeded`), the test cannot even test the limit-exceeded scenario because `pctx.Redis` and `pctx.Postgres` cannot be mocked!

**Before (`internal/plugin/builtin/budget_control.go:56-68`):**
```go
// ❌ Violates DIP: Directly depends on concrete *db.RedisClient and *db.PostgresDB structs.
if pctx.Redis != nil && keyID != "" {
	if cachedSpent, cErr := pctx.Redis.GetCachedBudgetSpent(pctx.Ctx, keyID); cErr == nil {
		spent = cachedSpent
		gotFromCache = true
	}
}

if !gotFromCache && pctx.Postgres != nil && pctx.APIKey != nil {
	spent, err = pctx.Postgres.GetSpentCostByKeyID(pctx.APIKey.ID)
	// ...
}
```

**After:**
```go
// ✅ DIP Refactor: Invert dependencies via domain service interfaces.
type BudgetTracker interface {
	GetSpentCost(ctx context.Context, keyID string) (float64, error)
}

type BudgetControlPlugin struct {
	tracker BudgetTracker // ℹ️ Injected via constructor or service locator
}

func NewBudgetControlPlugin(tracker BudgetTracker) *BudgetControlPlugin {
	return &BudgetControlPlugin{tracker: tracker}
}

func (p *BudgetControlPlugin) ExecPostAuth(req AuthContext, ctrl PipelineController, cfg map[string]any) error {
	spent, err := p.tracker.GetSpentCost(req.Context(), req.APIKeyID())
	if err != nil {
		return nil // Fail open
	}
	if spent >= cfgLimit {
		ctrl.Halt(http.StatusPaymentRequired, "Budget limit exceeded")
	}
	return nil
}
```

#### 2. `PluginEngine` Directly Depends on Concrete `*Registry` and Concrete `*db.RedisClient`
`internal/plugin/engine.go:26, 139`

`PluginEngine` holds a field `registry *Registry` (concrete struct pointer) rather than an interface. Furthermore, `StartRedisSubscriber` takes `redisClient *db.RedisClient` (concrete struct pointer) rather than a PubSub abstraction like `EventSubscriber`.

#### 3. Package-Level Global Registry Singleton
`internal/plugin/registry.go:14-21, 45-57`

The global variable `defaultRegistry` is automatically used when `nil` is passed to `NewPluginEngine(pg, nil)`. This creates hidden global state and makes isolated concurrent unit testing error-prone.

---

## Design Pattern Opportunities

### 1. Chain of Responsibility / Pipeline Pattern in `internal/plugin/engine.go`
**Why:** Replaces duplicated procedural loops across `RunPreAuth`, `RunPostAuth`, `RunPreRouting`, and `RunPostUpstream` with an extensible, reusable middleware execution chain that centralizes error handling, timing metrics, and short-circuit evaluation.

**Before (`internal/plugin/engine.go:334-352`):**
```go
// ❌ Duplicated across RunPreAuth, RunPostAuth, RunPreRouting...
func (e *PluginEngine) RunPreRouting(pctx *PluginContext, bindings []model.PluginBinding) error {
	for _, b := range bindings {
		inst, ok := e.registry.Get(b.PluginName)
		if !ok {
			continue
		}
		if p, ok := inst.(PreRoutingPlugin); ok {
			cfg := parseBindingConfig(b)
			if err := p.ExecPreRouting(pctx, cfg); err != nil {
				return err
			}
			if pctx.Stop {
				return nil
			}
		}
	}
	return nil
}
```

**After:**
```go
// ✅ Pipeline / Chain of Responsibility Pattern:
type StepHandler func(ctx context.Context, pctx *PluginContext, cfg map[string]any) error

type PipelineStep struct {
	PluginName string
	Config     map[string]any
	Handler    StepHandler
}

type Pipeline struct {
	steps []PipelineStep
}

func (p *Pipeline) Execute(ctx context.Context, pctx *PluginContext) error {
	for _, step := range p.steps {
		if pctx.Stop {
			break
		}
		if err := step.Handler(ctx, pctx, step.Config); err != nil {
			return fmt.Errorf("plugin %s failed: %w", step.PluginName, err)
		}
	}
	return nil
}
```

---

### 2. Strategy Pattern in `internal/plugin/engine.go`
**Why:** Encapsulates scope matching and precedence ranking behind a polymorphic strategy interface, allowing new binding scopes to be added without modifying the core resolution algorithm.

**Before (`internal/plugin/engine.go:201-228`):**
```go
// ❌ Hardcoded switch statement inside resolution loop:
switch b.ScopeLevel {
case "global":
	matched = true
case "provider":
	if b.ScopeProvider != nil && provider != "" && *b.ScopeProvider == provider {
		matched = true
	}
// ...
}
```

**After:**
```go
// ✅ Strategy Pattern:
type ScopeStrategy interface {
	Matches(b model.PluginBinding, req RequestMetadata) bool
	Specificity() int
}

type ProviderScopeStrategy struct{}
func (s *ProviderScopeStrategy) Specificity() int { return 2 }
func (s *ProviderScopeStrategy) Matches(b model.PluginBinding, req RequestMetadata) bool {
	return b.ScopeProvider != nil && req.Provider != "" && *b.ScopeProvider == req.Provider
}

type ApiKeyScopeStrategy struct{}
func (s *ApiKeyScopeStrategy) Specificity() int { return 7 }
func (s *ApiKeyScopeStrategy) Matches(b model.PluginBinding, req RequestMetadata) bool {
	return b.ScopeKeyID != nil && req.ApiKeyID != nil && *b.ScopeKeyID == *req.ApiKeyID
}
```

---

### 3. Repository & Decorator Pattern in `internal/plugin/engine.go`
**Why:** Decouples database querying from in-memory caching and Redis invalidation, eliminating infrastructure caching logic from `PluginEngine`.

**Before (`internal/plugin/engine.go:79-117`):**
```go
// ❌ PluginEngine manually handles caching, TTL expiry, and DB reloads:
func (e *PluginEngine) GetActiveBindings(ctx context.Context) ([]model.PluginBinding, error) {
	e.mu.RLock()
	cached := e.cachedBindings
	// ... double checked lock, DB reload, fallback to stale cache ...
}
```

**After:**
```go
// ✅ Repository Pattern with Decorator:
type BindingRepository interface {
	ListActiveBindings(ctx context.Context) ([]model.PluginBinding, error)
}

// Postgres implementation
type SQLBindingRepository struct { db *sql.DB }
func (r *SQLBindingRepository) ListActiveBindings(ctx context.Context) ([]model.PluginBinding, error) { ... }

// Cache Decorator wrapping the repository
type CachedBindingRepository struct {
	inner    BindingRepository
	cache    sync.Map
	ttl      time.Duration
}

func (c *CachedBindingRepository) ListActiveBindings(ctx context.Context) ([]model.PluginBinding, error) {
	// Handles in-memory caching transparently
	return c.inner.ListActiveBindings(ctx)
}
```

---

### 4. Adapter Pattern in `internal/plugin/builtin`
**Why:** Decouples plugins from specific database drivers (`*db.RedisClient`, `*db.PostgresDB`) by providing domain-specific adapters (`RateLimiterAdapter`, `TokenBucketAdapter`, `MetricsCollectorAdapter`).

**Before (`internal/plugin/builtin/rate_limiting.go:39`):**
```go
// ❌ Direct dependency on *db.RedisClient concrete API:
allowed, err := pctx.Redis.AllowRequest(pctx.Ctx, fmt.Sprintf("plugin:rate_limiting:%s", keyIdentifier), rpm)
```

**After:**
```go
// ✅ Adapter Pattern:
type RateLimiter interface {
	Allow(ctx context.Context, identifier string, limitPerMinute int) (bool, error)
}

// Redis adapter implementing RateLimiter
type RedisRateLimiterAdapter struct {
	client *db.RedisClient
}

func (a *RedisRateLimiterAdapter) Allow(ctx context.Context, id string, rpm int) (bool, error) {
	return a.client.AllowRequest(ctx, "plugin:rate_limiting:"+id, rpm)
}
```

---

### 5. Config Decorator Pattern in `internal/plugin/engine.go`
**Why:** Eliminates repeated `json.Unmarshal` calls on every single request in the critical path by parsing and caching validated configuration once during binding resolution.

**Before (`internal/plugin/engine.go:286-292, 302`):**
```go
// ❌ Unmarshals JSON raw config on EVERY request in EVERY hook execution!
func parseBindingConfig(b model.PluginBinding) map[string]any {
	cfg := make(map[string]any)
	if len(b.Config) > 0 {
		_ = json.Unmarshal(b.Config, &cfg)
	}
	return cfg
}
```

**After:**
```go
// ✅ Pre-parsed / Cached Config Decorator:
type ResolvedBinding struct {
	Binding model.PluginBinding
	ParsedConfig map[string]any // ℹ️ Parsed once when loading/resolving bindings, zero JSON unmarshals on hot path
}
```

---

## Summary Table

| # | Principle/Pattern | Severity | File | Status |
|---|---|---|---|---|
| 1 | **DIP** | 🔴 High | `internal/plugin/plugin.go:39-40`, `internal/plugin/builtin/*.go` | Direct dependency on concrete `*db.RedisClient` & `*db.PostgresDB`; proposed domain service interfaces & adapters |
| 2 | **ISP** | 🔴 High | `internal/plugin/plugin.go:24-72` | Monolithic `PluginContext` God Object passed to all plugins; proposed segregated capability interfaces (`RequestReader`, `HeaderModifier`, `PipelineController`) |
| 3 | **SRP** | 🔴 High | `internal/plugin/engine.go:24-165, 294-396` | `PluginEngine` manages caching, background sync, pubsub listeners, scope resolution, and pipeline execution; proposed separating into `BindingRepository`, `ScopeResolver`, and `PipelineEngine` |
| 4 | **OCP** | 🟡 Medium | `internal/plugin/engine.go:167-175, 201-229` | Hardcoded scope evaluation map & switch statement; proposed Strategy Pattern (`ScopeStrategy`) |
| 5 | **LSP** | 🟡 Medium | `internal/plugin/registry.go:11, 31-43` | Registry accepts untyped `any` leading to silent runtime skips; proposed typed `Plugin` interface contract |
| 6 | **LSP** | 🟡 Medium | `internal/plugin/plugin.go:74-79`, `engine.go:368-384` | `AsyncPostUpstreamPlugin` marker breaks behavioral subtyping with mutable context; proposed separating read-only telemetry plugins |
| 7 | **Chain of Resp.** | 🟡 Medium | `internal/plugin/engine.go:294-396` | Duplicated execution loop logic across 4 hook methods; proposed reusable `Pipeline` abstraction |
| 8 | **Performance / Decorator** | 🟢 Low | `internal/plugin/engine.go:286-292` | Repeated JSON unmarshaling on request hot path; proposed pre-parsing configuration in `ResolvedBinding` |
| 9 | **DIP / Naming** | 🟢 Low | `internal/plugin/engine.go:19-21` | `PostgresReader` named after concrete technology; proposed renaming to `BindingStore` or `BindingRepository` |

---

## Recommended Next Steps

1. **Invert Database Dependencies (DIP) [High Impact]:**
   - Define domain interfaces (`RateLimiter`, `BudgetStore`, `CacheStore`) in `internal/plugin`.
   - Remove `Redis *db.RedisClient` and `Postgres *db.PostgresDB` from `PluginContext`.
   - Inject adapters into builtin plugins at initialization time (in `builtin/init.go`), immediately making all builtin plugins 100% unit-testable without database dependencies.

2. **Segregate Plugin Context (ISP) [High Impact]:**
   - Replace the 20-field `PluginContext` with narrow interfaces (`RequestReader`, `HeaderModifier`, `PipelineController`).
   - Plugins only receive the specific capabilities they need, preventing unintended mutations and security leaks.

3. **Extract Repository & Caching from `PluginEngine` (SRP) [Medium Impact]:**
   - Move cache management, TTL expiration, and Redis subscriber into a dedicated `CachedBindingRepository`.
   - Keep `PluginEngine` lean and focused solely on request-time pipeline execution.

4. **Implement Strategy Pattern for Scope Resolution (OCP) [Medium Impact]:**
   - Refactor `ResolveEffectiveBindings` to use a `ScopeStrategy` registry, allowing new scope dimensions (e.g. `team`, `ip_subnet`) to be added without modifying the core engine.

5. **Consolidate Hook Execution into a Reusable Pipeline (Chain of Responsibility) [Medium Impact]:**
   - Unify `RunPreAuth`, `RunPostAuth`, `RunPreRouting`, and `RunPostUpstream` into a standardized pipeline executor that eliminates code duplication and enables uniform error handling, telemetry, and panic recovery.
