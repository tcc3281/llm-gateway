# Code Review Report: SOLID & Design Patterns

## Scope
- **Target File**: `internal/service/gateway.go` (and associated `GatewayService` methods in `internal/service/`)
- **Language**: Go (Go 1.22+, module `llm-gateway`)
- **Existing Architectural Patterns**: Layered proxy architecture (`cmd/gateway` → `internal/proxy` → `service.GatewayService` → adapters/db/plugins/telemetry). `GatewayService` acts as the primary data-plane coordinator for inbound LLM API calls, transforming protocols, validating permissions, enforcing multi-tenant boundaries, caching responses, resolving virtual models, routing to upstream providers (OpenAI, Anthropic, Gemini, custom), executing plugins, and logging telemetry.

---

## SOLID Audit

### ⚠️ S — Single Responsibility Principle
**File**: `internal/service/gateway.go` (lines 86–106, 166–213, 224–493, 495–586)

`GatewayService` is a classic **God Object** with 17 struct fields, and its primary method `HandleRequest` is a 270-line procedural coordinator that violates SRP by taking on at least 10 disparate responsibilities:
1. **HTTP Transport Handling**: Directly binds to `http.ResponseWriter`, writes HTTP status codes, formats JSON error envelopes, and sets headers (`Content-Type`, `X-Cache`).
2. **In-Memory Cache Management**: Manages entity caching and TTL eviction for users and tenants (`userCache sync.Map`, `tenantCache sync.Map`, `getCachedUser`, `getCachedTenant`).
3. **Protocol Inbound/Outbound Transformation**: Directly executes decoding and encoding for OpenAI and Anthropic wire formats.
4. **Authorization & Security**: Executes key model policies and IAM group access checks via `PermissionEngine`.
5. **Multi-Tenancy & Financial Quotas**: Enforces tenant isolation boundaries and performs credit hold reservations and Redis releases.
6. **Plugin Lifecycle Orchestration**: Dispatches pre-auth, post-auth, pre-routing, and post-upstream plugin interceptors.
7. **Distributed Tracing**: Inspects payload shapes and records prompt/completion events on OpenTelemetry spans.
8. **Response Caching (L2 Cache)**: Checks exact-match cache hits, sets cache headers, and writes entries to `ResponseCache`.
9. **Upstream Routing & Resiliency**: Resolves models/aliases and executes fallback ladders via `sendWithFallback`.
10. **Telemetry & Billing Side-Effects**: Calculates costs, emits token metrics, checks agent runaway loop thresholds, and asynchronously deducts prepaid tenant credits.

Any change to transport protocols, caching policies, billing logic, or plugin hooks forces modifications to `gateway.go`.

**Before:**
```go
// internal/service/gateway.go:224-271
func (g *GatewayService) HandleRequest(ctx context.Context, params ProxyParams) error {
    startTime := time.Now()

    // ❌ Concern 1: Plugin Context Setup & Pre-Auth Plugin execution
    var pctx *plugin.PluginContext
    if g.pluginEngine != nil {
        user, _ := g.getCachedUser(params.ApiKey.UserID) // ❌ Concern 2: In-memory DB entity cache
        // ... plugin execution ...
        if pctx.Stop {
            params.ResponseWriter.WriteHeader(pctx.StopStatusCode) // ❌ Concern 3: Direct HTTP transport writing
            params.ResponseWriter.Write(pctx.StopErrorBody)
            return nil
        }
    }

    // ❌ Concern 4: Inbound Protocol Wire Decoding
    var canReq *canonical.CanonicalRequest
    if params.InboundFormat == "openai" {
        canReq, err = openai.DecodeInboundRequest(params.RequestBody)
    } // ...

    // ❌ Concern 5: IAM Permission Enforcement
    if g.permEngine != nil && params.ApiKey != nil {
        allowed, pErr := g.permEngine.CheckKeyModelAccess(ctx, params.ApiKey.ID, canReq.Model)
        // ...
    }

    // ❌ Concern 6: Multi-Tenant Credit Reservation & Redis Hold Management
    heldAmount, hErr := g.reserveTenantHold(ctx, params.ApiKey, primaryTarget.Name, targetModelName, ...)
    // ...
}
```

**After:**
```go
// ✅ Solution: Decompose into focused domain components and an execution pipeline.
// GatewayService coordinates high-level execution; transport and concerns are delegated.

type RequestPipeline interface {
    Execute(ctx context.Context, req *GatewayContext) (*GatewayResult, error)
}

// GatewayContext decouples the core domain from HTTP net/http abstractions
type GatewayContext struct {
    Context        context.Context
    InboundFormat  string
    RequestBody    []byte
    APIKey         *model.APIKey
    ClientIP       string
    CanonicalReq   *canonical.CanonicalRequest
    Tenant         *model.Tenant
    User           *model.User
}

// ✅ GatewayService has a single responsibility: orchestrate request processing through the pipeline
type GatewayService struct {
    pipeline RequestPipeline
}

func (g *GatewayService) HandleRequest(ctx context.Context, req *GatewayContext) (*GatewayResult, error) {
    // ℹ️ Single reason to change: alterations in the top-level pipeline execution flow
    return g.pipeline.Execute(ctx, req)
}
```

---

### ⚠️ O — Open/Closed Principle
**File**: `internal/service/gateway.go` (lines 277–287, 503–511, 528–536)

The service is closed to extension and open to constant modification. Whenever a new wire protocol (e.g. AWS Bedrock, Cohere, Mistral, Ollama) or upstream provider is supported, developers must edit `gateway.go` in multiple `if-else` branches and `switch` statements:
- Lines 277–283: `if params.InboundFormat == "openai" { ... } else if params.InboundFormat == "anthropic" { ... }`
- Lines 503–510: `switch target.Provider { case provider.ProviderOpenAI: ... case provider.ProviderAnthropic: ... case provider.ProviderGemini: ... }`
- Lines 528–533: `if params.InboundFormat == "openai" { ... } else if params.InboundFormat == "anthropic" { ... }`

**Before:**
```go
// internal/service/gateway.go:277-287
// ❌ Adding a new format requires editing core HandleRequest logic
if params.InboundFormat == "openai" {
    canReq, err = openai.DecodeInboundRequest(params.RequestBody)
} else if params.InboundFormat == "anthropic" {
    canReq, err = anthropic.DecodeInboundRequest(params.RequestBody)
} else {
    return fmt.Errorf("unsupported inbound format: %s", params.InboundFormat)
}

// internal/service/gateway.go:503-511
// ❌ Adding an upstream provider requires editing handleNonStreamResponse
switch target.Provider {
case provider.ProviderOpenAI:
    canResp, err = openai.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderAnthropic:
    canResp, err = anthropic.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderGemini:
    canResp, err = gemini.DecodeUpstreamResponse(bodyBytes, canReq.Model)
}
```

**After:**
```go
// ✅ Solution: Define a ProtocolCodec strategy and registry open for extension.

type ProtocolCodec interface {
    Format() string
    DecodeRequest(body []byte) (*canonical.CanonicalRequest, error)
    FormatResponse(resp *canonical.CanonicalResponse) ([]byte, error)
}

type CodecRegistry struct {
    codecs map[string]ProtocolCodec
}

func (r *CodecRegistry) Register(codec ProtocolCodec) {
    r.codecs[codec.Format()] = codec
}

func (r *CodecRegistry) Get(format string) (ProtocolCodec, error) {
    c, ok := r.codecs[format]
    if !ok {
        return nil, fmt.Errorf("unsupported format: %s", format)
    }
    return c, nil
}

// ℹ️ HandleRequest is now closed for modification; new formats are registered at startup:
func (g *GatewayService) decodeInbound(format string, body []byte) (*canonical.CanonicalRequest, error) {
    codec, err := g.codecRegistry.Get(format) // ✅ Dynamic strategy lookup
    if err != nil {
        return nil, err
    }
    return codec.DecodeRequest(body)
}
```

---

### ⚠️ L — Liskov Substitution Principle
**File**: `internal/service/gateway.go` (lines 174–176, 199–201, 263–270, 467–485, 513–516)

1. **Inconsistent Error/Response Contract**:
   `HandleRequest` presents an inconsistent return contract regarding `http.ResponseWriter`:
   - On plugin termination (lines 263–269, 329–335, 342–348) and non-200 upstream responses (lines 467–485), the method writes the HTTP status code and body directly to `params.ResponseWriter` and returns `nil`.
   - On response decode error (lines 513–516), it writes an HTTP 500 header and body to `ResponseWriter` **and also returns `err`**.
   - On permission denial or routing failure (lines 282, 300, 423, 458), it writes nothing and returns `err`.
   This breaks substitutability and caller expectations: callers wrapping `HandleRequest` cannot know whether an error has already written HTTP headers or if the caller must write an error response. Calling `w.WriteHeader(...)` when `err != nil` causes runtime warnings (`http: superfluous response.WriteHeader call`).

2. **Silent Failure / Leaky Nil Checks**:
   In `getCachedUser` and `getCachedTenant` (lines 174–176, 199–201):
   When `g.pg == nil`, it returns `nil, nil`. The caller receives no error and `nil` entity, proceeding under the assumption that the entity does not exist or that database validation succeeded without error.

**Before:**
```go
// internal/service/gateway.go:512-516
if err != nil {
    // ❌ Side effect: writes HTTP response headers and body
    params.ResponseWriter.WriteHeader(http.StatusInternalServerError)
    params.ResponseWriter.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
    // ❌ Contract violation: returns err AFTER committing response!
    return err
}

// internal/service/gateway.go:471-485
if upResp.StatusCode != http.StatusOK {
    params.ResponseWriter.WriteHeader(upResp.StatusCode)
    params.ResponseWriter.Write(bodyBytes)
    // ❌ Here it commits response but returns nil!
    return nil
}
```

**After:**
```go
// ✅ Solution: Standardize the return contract. The service returns pure results or typed errors.
// Transport rendering is handled exclusively by the transport layer / HTTP adapter.

type GatewayResponse struct {
    StatusCode int
    Headers    map[string]string
    Body       []byte
    Stream     io.ReadCloser
}

// ℹ️ Service never writes to ResponseWriter directly; always returns (*GatewayResponse, error)
func (g *GatewayService) HandleRequest(ctx context.Context, req *GatewayContext) (*GatewayResponse, error) {
    // ✅ Either returns a valid domain response OR a typed domain error
    if err != nil {
        return nil, fmt.Errorf("decoding upstream response: %w", err) // ✅ No transport side effects
    }
    return &GatewayResponse{
        StatusCode: http.StatusOK,
        Headers:    map[string]string{"Content-Type": "application/json"},
        Body:       clientResponseBytes,
    }, nil
}
```

---

### ⚠️ I — Interface Segregation Principle
**File**: `internal/service/gateway.go` (lines 86–106, 124–157)

`GatewayService` depends directly on monolithic concrete structs that bundle vast amounts of functionality that `gateway.go` never uses:
- `pg *db.PostgresDB`: Exposes dozens of database query methods across audit logging, keys, aliases, and admin tables. `gateway.go` only requires `GetUserByID` and `GetTenantByID`.
- `redis *db.RedisClient`: Exposes full Redis commands, caching, pubsub, and rate limiting. `gateway.go` only invokes `ReleaseTenantCreditHold`.
- Monolithic engines: Depends directly on `*permission.PermissionEngine`, `*analytics.CostCalculator`, `*analytics.TelemetryLogger`, `*alert.Engine`, and `*plugin.PluginEngine`.

Furthermore, `GatewayService` itself does not implement or define an interface. Callers (`internal/proxy/proxy.go`, `cmd/gateway/main.go`) are tightly bound to the concrete `GatewayService` struct, making test doubles or alternative proxy engine implementations impossible.

**Before:**
```go
// internal/service/gateway.go:86-106
type GatewayService struct {
    providerClient       *provider.ProviderClient      // ❌ Fat concrete struct
    keyStore             *provider.KeyStore            // ❌ Fat concrete struct
    costCalc             *analytics.CostCalculator     // ❌ Fat concrete struct
    telemetry            *analytics.TelemetryLogger    // ❌ Fat concrete struct
    redis                *db.RedisClient               // ❌ Monolithic Redis struct (only uses hold release)
    pg                   *db.PostgresDB                // ❌ Monolithic DB struct (only uses 2 query methods)
    responseCache        *cache.ResponseCache          // ❌ Fat concrete struct
    permEngine           *permission.PermissionEngine  // ❌ Fat concrete struct
    // ...
}
```

**After:**
```go
// ✅ Solution: Define narrow, client-specific interfaces at the consumer site (ISP)

type UserTenantReader interface {
    GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
    GetTenantByID(ctx context.Context, id uuid.UUID) (*model.Tenant, error)
}

type CreditHoldManager interface {
    ReleaseCreditHold(ctx context.Context, tenantID string, amount float64) error
}

type ModelAuthorizer interface {
    CheckKeyModelAccess(ctx context.Context, keyID uuid.UUID, model string) (bool, error)
    CheckModelAccess(ctx context.Context, userID uuid.UUID, model string) (bool, error)
}

// ✅ GatewayService depends only on the minimal method sets it actually uses
type GatewayService struct {
    userTenantRepo UserTenantReader
    creditHolder   CreditHoldManager
    authorizer     ModelAuthorizer
    // ...
}
```

---

### ⚠️ D — Dependency Inversion Principle
**File**: `internal/service/gateway.go` (lines 13–27, 86–106, 138–141)

High-level business orchestration (`GatewayService`) depends directly on low-level detail packages and concrete implementations:
1. **Direct Package Coupling**: Imports concrete provider adapters (`internal/adapter/openai`, `internal/adapter/anthropic`, `internal/adapter/gemini`) and infrastructure modules (`internal/db`).
2. **In-Constructor Instantiation**: In `NewGatewayService` (lines 138–141), `GatewayService` directly instantiates `plugin.NewPluginEngine(pg, nil)` rather than receiving an abstraction:
   ```go
   var pe *plugin.PluginEngine
   if pg != nil {
       pe = plugin.NewPluginEngine(pg, nil) // ❌ Violates DIP: Instantiates concrete dependency
   }
   ```
3. **Untestable Unit Scope**: Because all 12 constructor parameters and struct fields are concrete structs without interfaces, writing a unit test for `HandleRequest` requires initializing live or complex mock instances of Postgres, Redis, and upstream provider clients.

**Before:**
```go
// internal/service/gateway.go:124-156
func NewGatewayService(
    p *provider.ProviderClient,          // ❌ Concretion
    k *provider.KeyStore,                // ❌ Concretion
    c *analytics.CostCalculator,         // ❌ Concretion
    t *analytics.TelemetryLogger,        // ❌ Concretion
    redis *db.RedisClient,               // ❌ Concretion
    pg *db.PostgresDB,                   // ❌ Concretion
    rc *cache.ResponseCache,             // ❌ Concretion
    permEngine *permission.PermissionEngine, // ❌ Concretion
    // ...
) *GatewayService {
    var pe *plugin.PluginEngine
    if pg != nil {
        pe = plugin.NewPluginEngine(pg, nil) // ❌ Instantiating concrete dependency internally
    }
    // ...
}
```

**After:**
```go
// ✅ Solution: High-level policy depends on abstractions; dependencies are injected from outside.

type UpstreamDispatcher interface {
    Dispatch(ctx context.Context, targets []ResolvedTarget, req *canonical.CanonicalRequest) (*provider.UpstreamResponse, error)
}

type PluginRunner interface {
    RunPreAuth(ctx *plugin.PluginContext) error
    RunPostAuth(ctx *plugin.PluginContext) error
    RunPreRouting(ctx *plugin.PluginContext) error
    RunPostUpstream(ctx *plugin.PluginContext, resp *canonical.CanonicalResponse) error
}

func NewGatewayService(
    dispatcher UpstreamDispatcher,
    plugins PluginRunner,
    authorizer ModelAuthorizer,
    codecs CodecRegistry,
    // ✅ Injected purely as interfaces
) *GatewayService {
    return &GatewayService{
        dispatcher: dispatcher,
        plugins:    plugins,
        authorizer: authorizer,
        codecs:     codecs,
    }
}
```

---

## Design Pattern Opportunities

### Chain of Responsibility (Pipeline / Middleware) in `internal/service/gateway.go`
**Why:** Decomposes the monolithic 270-line procedural `HandleRequest` execution sequence into discrete, orderable, and independently testable stages (PreAuth, Decrypt/Decode, Auth/Quota, Cache, Routing, Upstream, PostUpstream, Metrics).

**Before:**
```go
// internal/service/gateway.go:224-493
// ❌ 270 lines of sequential, hardcoded procedural stages intermingled in one method
func (g *GatewayService) HandleRequest(ctx context.Context, params ProxyParams) error {
    // 1. Plugin PreAuth
    // 2. Decode Inbound Request
    // 3. Check IAM
    // 4. Multi-Tenant Hold
    // 5. Response Cache Lookup
    // 6. Target Resolution
    // 7. Upstream Fallback Dispatch
    // 8. Decode Upstream Response
    // 9. Telemetry & Metrics
}
```

**After:**
```go
// ✅ Define an interceptor/middleware pipeline stage
type StageHandler func(ctx context.Context, req *RequestContext) (*ResponseContext, error)

type Middleware func(StageHandler) StageHandler

type Pipeline struct {
    stages []Middleware
}

func (p *Pipeline) Use(mw Middleware) {
    p.stages = append(p.stages, mw)
}

func (p *Pipeline) Execute(ctx context.Context, req *RequestContext, final StageHandler) (*ResponseContext, error) {
    curr := final
    for i := len(p.stages) - 1; i >= 0; i-- {
        curr = p.stages[i](curr)
    }
    return curr(ctx, req)
}

// ℹ️ Handlers are now cleanly separated into isolated, testable middleware components:
// - AuthMiddleware
// - ResponseCacheMiddleware
// - QuotaHoldMiddleware
// - UpstreamDispatchMiddleware
```

---

### Strategy Pattern (Protocol Codecs) in `internal/service/gateway.go`
**Why:** Replaces hardcoded protocol `if/else` checks for inbound requests (`openai`, `anthropic`) and upstream provider `switch` blocks with interchangeable, polymorphic serialization strategies.

**Before:**
```go
// internal/service/gateway.go:277-284, 528-533
if params.InboundFormat == "openai" {
    canReq, err = openai.DecodeInboundRequest(params.RequestBody)
} else if params.InboundFormat == "anthropic" {
    canReq, err = anthropic.DecodeInboundRequest(params.RequestBody)
}

if params.InboundFormat == "openai" {
    clientResponseBytes, err = openai.FormatCanonicalToOpenAIResponse(canResp)
} else if params.InboundFormat == "anthropic" {
    clientResponseBytes, err = anthropic.FormatCanonicalToAnthropicResponse(canResp)
}
```

**After:**
```go
// ✅ Strategy interface for inbound client protocols
type InboundCodec interface {
    Decode(body []byte) (*canonical.CanonicalRequest, error)
    Encode(resp *canonical.CanonicalResponse) ([]byte, error)
}

// ✅ Strategy implementations in their respective adapter packages
type OpenAICodec struct{}
func (o *OpenAICodec) Decode(b []byte) (*canonical.CanonicalRequest, error) { return openai.DecodeInboundRequest(b) }
func (o *OpenAICodec) Encode(r *canonical.CanonicalResponse) ([]byte, error) { return openai.FormatCanonicalToOpenAIResponse(r) }

type AnthropicCodec struct{}
func (a *AnthropicCodec) Decode(b []byte) (*canonical.CanonicalRequest, error) { return anthropic.DecodeInboundRequest(b) }
func (a *AnthropicCodec) Encode(r *canonical.CanonicalResponse) ([]byte, error) { return anthropic.FormatCanonicalToAnthropicResponse(r) }

// ℹ️ Execution becomes a single polymorphic strategy invocation:
codec := g.codecs[params.InboundFormat]
canReq, err := codec.Decode(params.RequestBody)
```

---

### Builder / Functional Options Pattern in `internal/service/gateway.go`
**Why:** Simplifies the error-prone, 12-parameter constructor `NewGatewayService` and ad-hoc setters (`SetAlertEngine`, `SetPluginEngine`), making configuration safe, readable, and backwards-compatible.

**Before:**
```go
// internal/service/gateway.go:124-157
// ❌ 12 positional parameters, easily swapped or broken when adding new options
func NewGatewayService(
    p *provider.ProviderClient,
    k *provider.KeyStore,
    c *analytics.CostCalculator,
    t *analytics.TelemetryLogger,
    redis *db.RedisClient,
    pg *db.PostgresDB,
    rc *cache.ResponseCache,
    permEngine *permission.PermissionEngine,
    contentCapturePolicy string,
    sloMaxTTFTMs int,
    sloMaxTBTMs int,
    agentMaxToolCalls int,
) *GatewayService

// ❌ Requires secondary mutation calls after construction
gw.SetAlertEngine(alertEng)
gw.SetPluginEngine(pluginEng)
```

**After:**
```go
// ✅ Functional Options Pattern
type Option func(*GatewayService)

func WithCache(rc ResponseCacher) Option {
    return func(g *GatewayService) { g.responseCache = rc }
}

func WithAlertEngine(ae AlertEmitter) Option {
    return func(g *GatewayService) { g.alertEngine = ae }
}

func WithSLOThresholds(ttftMs, tbtMs int) Option {
    return func(g *GatewayService) {
        g.sloMaxTTFTMs = ttftMs
        g.sloMaxTBTMs = tbtMs
    }
}

// ℹ️ Clean, robust constructor with required dependencies first, followed by variadic options:
func NewGatewayService(dispatcher UpstreamDispatcher, authorizer ModelAuthorizer, opts ...Option) *GatewayService {
    g := &GatewayService{
        dispatcher: dispatcher,
        authorizer: authorizer,
    }
    for _, opt := range opts {
        opt(g)
    }
    return g
}
```

---

### Decorator Pattern (Response Caching & Telemetry) in `internal/service/gateway.go`
**Why:** Extracts cross-cutting caching (L2 response cache) and telemetry logging out of the core upstream dispatch logic and into modular decorators.

**Before:**
```go
// internal/service/gateway.go:365-396, 547-549
// ❌ Cache check, hit header injection, and store logic embedded directly inside HandleRequest
if !canReq.Stream && g.responseCache != nil {
    if cached, hit := g.responseCache.Get(ctx, cacheKey); hit {
        params.ResponseWriter.Header().Set("X-Cache", "HIT")
        params.ResponseWriter.Write(cached)
        g.recordTelemetry(...)
        return nil
    }
}
// Upstream execution...
if cacheEnabled {
    g.responseCache.Set(ctx, cacheKey, clientResponseBytes, cacheTTL)
}
```

**After:**
```go
// ✅ Decorate the RequestHandler interface with caching
type CachingDecorator struct {
    cache   ResponseCacher
    next    RequestHandler
}

func (c *CachingDecorator) Handle(ctx context.Context, req *GatewayContext) (*GatewayResponse, error) {
    if !req.CanonicalReq.Stream {
        if hit, val := c.cache.Get(ctx, req.CacheKey()); hit {
            return &GatewayResponse{
                StatusCode: http.StatusOK,
                Headers:    map[string]string{"X-Cache": "HIT", "Content-Type": "application/json"},
                Body:       val,
            }, nil // ✅ Cache hit short-circuits transparently
        }
    }

    resp, err := c.next.Handle(ctx, req)
    if err == nil && !req.CanonicalReq.Stream {
        c.cache.Set(ctx, req.CacheKey(), resp.Body, req.CacheTTL())
    }
    return resp, err
}
```

---

### Adapter Pattern (Transport Decoupling) in `internal/service/gateway.go`
**Why:** Decouples `GatewayService` from the HTTP transport layer (`net/http`) by converting HTTP requests into transport-agnostic domain requests and returning structured domain responses.

**Before:**
```go
// internal/service/gateway.go:215-224
// ❌ Service takes raw http.ResponseWriter and writes HTTP status codes and headers
type ProxyParams struct {
    InboundFormat  string
    RequestBody    []byte
    ApiKey         *model.APIKey
    UpstreamAPIKey string
    ClientIP       string
    ResponseWriter http.ResponseWriter // ❌ Leaks HTTP wire transport directly into service layer
}
```

**After:**
```go
// ✅ HTTP Transport Adapter (in internal/handler or internal/proxy)
type HTTPGatewayAdapter struct {
    service GatewayRunner
    codecs  CodecRegistry
}

func (h *HTTPGatewayAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    req, err := h.buildDomainRequest(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    res, err := h.service.Execute(r.Context(), req)
    if err != nil {
        h.renderError(w, err)
        return
    }

    for k, v := range res.Headers {
        w.Header().Set(k, v)
    }
    w.WriteHeader(res.StatusCode)
    w.Write(res.Body)
}
```

---

## Summary Table

| # | Principle/Pattern | Severity | File | Status |
|---|---|---|---|---|
| 1 | **SRP (Single Responsibility)** | 🔴 High | `internal/service/gateway.go` | Proposed fix above (decouple transport, caching, billing, and plugin coordination) |
| 2 | **OCP (Open/Closed)** | 🔴 High | `internal/service/gateway.go` | Proposed fix above (implement `ProtocolCodec` and provider strategy registries) |
| 3 | **LSP (Liskov Substitution)** | 🟡 Medium | `internal/service/gateway.go` | Proposed fix above (standardize return contract, remove mixed HTTP write side-effects) |
| 4 | **ISP (Interface Segregation)** | 🔴 High | `internal/service/gateway.go` | Proposed fix above (replace monolithic `*PostgresDB` and `*RedisClient` with narrow role interfaces) |
| 5 | **DIP (Dependency Inversion)** | 🔴 High | `internal/service/gateway.go` | Proposed fix above (inject abstractions; eliminate internal concretion instantiations) |
| 6 | **Chain of Responsibility** | 🔴 High | `internal/service/gateway.go` | Proposed pattern above (break 270-line `HandleRequest` into pipeline middleware stages) |
| 7 | **Strategy Pattern** | 🔴 High | `internal/service/gateway.go` | Proposed pattern above (replace format `if/else` and `switch` blocks with codec strategies) |
| 8 | **Builder / Functional Options** | 🟡 Medium | `internal/service/gateway.go` | Proposed pattern above (replace 12-argument constructor with functional options) |
| 9 | **Decorator Pattern** | 🟡 Medium | `internal/service/gateway.go` | Proposed pattern above (extract caching and telemetry as decorators) |
| 10 | **Adapter Pattern** | 🔴 High | `internal/service/gateway.go` | Proposed pattern above (remove `http.ResponseWriter` from service parameters) |

---

## Recommended Next Steps

1. **Decouple HTTP Transport via Adapter Pattern (Highest Priority)**:
   Remove `ResponseWriter http.ResponseWriter` from `ProxyParams`. Have `GatewayService` accept a transport-agnostic request model and return a domain `Response` or `error`. Move status code rendering, header setting, and error JSON marshaling into the HTTP handler layer.

2. **Extract Strategy Interfaces for Protocol Codecs (OCP)**:
   Define `InboundCodec` and `UpstreamCodec` interfaces. Move the `openai`, `anthropic`, and `gemini` request/response transformations into implementations of these strategies. Replace `if params.InboundFormat == ...` with registry lookups.

3. **Invert Dependencies with Role-Based Interfaces (ISP / DIP)**:
   Define consumer-side interfaces (`UserTenantReader`, `CreditHoldManager`, `ModelAuthorizer`) in `internal/service`. Update `NewGatewayService` to accept these interfaces instead of concrete `*db.PostgresDB` and `*db.RedisClient`. Inject `PluginEngine` from `main.go` instead of instantiating it inside `NewGatewayService`.

4. **Decompose `HandleRequest` into a Middleware Pipeline (Chain of Responsibility)**:
   Convert the sequential validation, quota reservation, plugin execution, and dispatch steps into composable pipeline stages. This isolates critical concerns like credit holds and plugin executions into unit-testable units.

5. **Adopt Functional Options for `GatewayService` Configuration**:
   Replace the 12-parameter constructor and mutation setters (`SetAlertEngine`, `SetPluginEngine`) with `NewGatewayService(deps..., WithOption(...))`.
