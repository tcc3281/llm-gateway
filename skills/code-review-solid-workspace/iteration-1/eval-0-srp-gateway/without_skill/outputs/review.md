# Architecture Code Review: `internal/service/gateway.go`
**Focus:** SOLID Principles Violations & Design Pattern Improvements  
**Target File:** `internal/service/gateway.go`  
**Date:** September 16, 2026  

---

## 1. Executive Summary

A comprehensive architectural review of `internal/service/gateway.go` confirms the team's concern: **`GatewayService` is currently acting as a classic "God Object" anti-pattern**. It has accumulated excessive responsibilities across protocol adaptation, identity & access control, multi-tenant billing, response caching, routing and fallback orchestration, transport writing, and telemetry recording.

### Summary Assessment

| Principle | Status | Severity | Primary Symptoms |
| :--- | :--- | :--- | :--- |
| **S** - Single Responsibility | **VIOLATED** | Critical | 10+ distinct responsibilities in one service and method (`HandleRequest`). |
| **O** - Open/Closed | **VIOLATED** | High | Hardcoded `if/else` and `switch` statements for protocol codecs and upstream providers; modifying or adding a provider/format requires editing core service logic. |
| **L** - Liskov Substitution | **VIOLATED** | Medium | Direct concrete types prevent polymorphic substitution; nil-checks act as fragile pseudo-polymorphism; inconsistent error/HTTP side effects. |
| **I** - Interface Segregation | **VIOLATED** | High | Service consumes monolithic database structs (`*db.PostgresDB`, `*db.RedisClient`) instead of role-focused interfaces; no client interface exposed. |
| **D** - Dependency Inversion | **VIOLATED** | Critical | High-level orchestration depends directly on concrete low-level packages and implementations; 12-parameter constructor couples tightly to internal modules. |

---

## 2. In-Depth SOLID Principle Violations

### 2.1. Single Responsibility Principle (SRP)
> *"A class or module should have one, and only one, reason to change."*

`GatewayService` and its primary method `HandleRequest` (lines 224–493) change for at least **10 distinct, unrelated business and operational reasons**:

1. **In-Memory Hot-Path Caching (Lines 76–84, 102–106, 166–213):**
   Manages `sync.Map` TTL-based caching for users and tenants (`getCachedUser`, `getCachedTenant`).
   *Reason to change:* Cache invalidation strategy, TTL adjustment, distributed caching migration.
2. **Plugin Lifecycle Orchestration (Lines 228–271, 318–350, 478–484, 518–525):**
   Instantiates `PluginContext` and invokes hook phases (`RunPreAuth`, `RunPostAuth`, `RunPreRouting`, `RunPostUpstream`).
   *Reason to change:* New plugin phase, plugin execution policy, hook error handling.
3. **Inbound Protocol Translation & Deserialization (Lines 273–288):**
   Parses raw JSON bytes into `canonical.CanonicalRequest` based on string format tags (`openai`, `anthropic`).
   *Reason to change:* New client format support (e.g. Gemini, Cohere, Bedrock), schema changes in client wire payloads.
4. **IAM & Access Control Evaluation (Lines 294–311):**
   Evaluates per-key allowlist/denylist rules and user IAM group policies via `permEngine`.
   *Reason to change:* Policy enforcement semantics, role hierarchy changes.
5. **Multi-Tenant System Boundary & Billing Holds (Lines 313–316, 430–441):**
   Calculates cost estimates, executes Redis credit holds (`reserveTenantHold`), and registers deferred hold releases.
   *Reason to change:* Billing logic, quota algorithms, credit hold release mechanisms.
6. **OpenTelemetry & Semantic Event Auditing (Lines 352–361, 387–391, 455–458, 474–477, 561–568):**
   Inspects payload shapes, injects trace span attributes, and captures prompt/completion events.
   *Reason to change:* OpenTelemetry GenAI semantic convention updates, redaction/content-capture policies.
7. **Response Caching (Lines 364–396, 546–549):**
   Evaluates caching eligibility, generates cache keys, writes cached responses, sets cache headers (`X-Cache`), and updates cache metrics.
   *Reason to change:* Cache storage strategy, TTL hierarchy, caching headers specification.
8. **Upstream Target Resolution & Fallback Scheduling (Lines 398–428, 450–465):**
   Resolves virtual models, catalog lookups, and dispatches requests across fallback ladders.
   *Reason to change:* Routing algorithms, fallback strategy, load balancing.
9. **Outbound Protocol Transcoding (Lines 501–536):**
   Decodes upstream provider responses (OpenAI, Anthropic, Gemini) and re-encodes them into the client's expected format.
   *Reason to change:* Upstream provider changes, response transformation nuances.
10. **HTTP Transport Serialization (Lines 264–268, 330–334, 342–346, 374–377, 471–473, 539–544):**
    Directly writes HTTP status codes, headers, and raw response bytes to `http.ResponseWriter`.
    *Reason to change:* HTTP transport concerns, streaming/framing mechanisms, header conventions.

#### Impact
Any PR touching authentication, caching, telemetry, billing, or provider integration must touch `gateway.go`. This causes high merge friction, severe regression risk, and makes comprehensive testing nearly impossible.

---

### 2.2. Open/Closed Principle (OCP)
> *"Software entities should be open for extension, but closed for modification."*

The codebase relies heavily on conditional dispatch (`if/else` and `switch` statements) based on type indicators and string identifiers.

#### Code Evidence: Inbound Protocol Parsing (`HandleRequest`, lines 277–283)
```go
if params.InboundFormat == "openai" {
    canReq, err = openai.DecodeInboundRequest(params.RequestBody)
} else if params.InboundFormat == "anthropic" {
    canReq, err = anthropic.DecodeInboundRequest(params.RequestBody)
} else {
    return fmt.Errorf("unsupported inbound format: %s", params.InboundFormat)
}
```

#### Code Evidence: Outbound Transcoding (`handleNonStreamResponse`, lines 503–534)
```go
switch target.Provider {
case provider.ProviderOpenAI:
    canResp, err = openai.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderAnthropic:
    canResp, err = anthropic.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderGemini:
    canResp, err = gemini.DecodeUpstreamResponse(bodyBytes, canReq.Model)
}

// ... and later:
if params.InboundFormat == "openai" {
    clientResponseBytes, err = openai.FormatCanonicalToOpenAIResponse(canResp)
} else if params.InboundFormat == "anthropic" {
    clientResponseBytes, err = anthropic.FormatCanonicalToAnthropicResponse(canResp)
}
```

#### Impact
Adding a new client protocol (e.g., Google Gemini or Ollama format) or a new upstream provider (e.g., Mistral, Bedrock, Vertex AI) requires directly modifying multiple methods in `gateway.go`. The system is closed to extension and constantly forced open for modification.

---

### 2.3. Liskov Substitution Principle (LSP)
> *"Objects of a superclass should be replaceable with objects of a subclass without affecting the correctness of the program."*

While Go uses structural interfaces rather than classical inheritance, LSP principles are violated in two major ways:

1. **Defensive Nil Checks as Fragile Pseudo-Polymorphism:**
   Throughout `gateway.go`, dependencies are frequently checked for `nil`:
   - `if g.pg == nil` (lines 174, 199)
   - `if g.permEngine != nil` (line 295)
   - `if g.pluginEngine != nil` (lines 231, 319, 478, 519)
   - `if g.responseCache != nil` (lines 368, 547)
   - `if g.redis != nil` (line 437)
   
   Instead of providing valid test doubles or a **Null Object** implementation that satisfies a common interface contract, `GatewayService` dynamically alters its execution branches when dependencies are nil. This makes component substitution unpredictable: a `nil` engine causes silent bypasses in some paths, but could cause a nil pointer panic in un-guarded paths (e.g. `g.costCalc.CalculateCost` at line 558 has no nil check).

2. **Inconsistent Error Propagation and Side Effects:**
   `HandleRequest` has inconsistent return semantics:
   - When a plugin triggers a stop (lines 262–270), it writes to `ResponseWriter` and returns `nil`.
   - When an upstream fails with non-200 (lines 467–485), it writes to `ResponseWriter` and returns `nil`.
   - When routing fails (lines 423, 458), it records telemetry and returns an `error`.
   
   Because `ResponseWriter` is passed directly in `ProxyParams`, the caller (`internal/handler/gateway.go`) cannot know whether the response was already written or if it should write an HTTP error. In `handler/gateway.go:81`:
   ```go
   if err := h.gwService.HandleRequest(ctx, params); err != nil {
       h.writeServiceError(w, "openai", err) // Risk of writing duplicate HTTP headers/bodies
   }
   ```

---

### 2.4. Interface Segregation Principle (ISP)
> *"Clients should not be forced to depend upon interfaces that they do not use."*

`GatewayService` depends directly on entire concrete infrastructure packages rather than narrow, client-specific interfaces:

1. **Heavy Concrete Structs:**
   ```go
   type GatewayService struct {
       providerClient       *provider.ProviderClient
       keyStore             *provider.KeyStore
       costCalc             *analytics.CostCalculator
       telemetry            *analytics.TelemetryLogger
       redis                *db.RedisClient
       pg                   *db.PostgresDB
       responseCache        *cache.ResponseCache
       permEngine           *permission.PermissionEngine
       alertEngine          *alert.Engine
       pluginEngine         *plugin.PluginEngine
       // ...
   }
   ```
   - From `*db.PostgresDB`, `gateway.go` only calls `GetUserByID` and `GetTenantByID`. Yet, `GatewayService` binds to the entire PostgreSQL connection pool, schema migrations, and admin query methods.
   - From `*db.RedisClient`, `gateway.go` only calls `ReleaseTenantCreditHold`.

2. **Absence of Service Abstraction:**
   There is no `Gateway` or `ChatCompletionService` interface exposed to handlers. Handlers are forced to bind to the massive `*service.GatewayService` pointer, preventing easy mocking in API handler unit tests.

3. **Violation of Idiomatic Go:**
   Go's core design ethos is: *"Accept interfaces, return structs"*. `GatewayService` accepts only concrete pointers and returns concrete pointers, coupling consumer and producer completely.

---

### 2.5. Dependency Inversion Principle (DIP)
> *"High-level modules should not depend on low-level modules. Both should depend on abstractions."*

`GatewayService` is a high-level application orchestrator, yet it depends directly on low-level concrete implementations:

1. **Direct Package Couplings:**
   The service imports concrete packages for databases (`internal/db`), specific third-party providers (`internal/adapter/openai`, `internal/adapter/anthropic`, `internal/adapter/gemini`), and concrete logging/caching systems.

2. **Constructor Instantiation of Collaborators (Lines 138–141):**
   ```go
   func NewGatewayService(...) *GatewayService {
       var pe *plugin.PluginEngine
       if pg != nil {
           pe = plugin.NewPluginEngine(pg, nil) // Direct instantiation violates Inversion of Control
       }
       return &GatewayService{ ... }
   }
   ```
   The constructor directly instantiates `plugin.PluginEngine`, tight-coupling `GatewayService` to the concrete plugin engine implementation.

3. **Constructor Parameter Bloat:**
   `NewGatewayService` requires 13 parameters. Any new dependency requires breaking the constructor signature across all integration points and test suites.

---

## 3. Additional Architectural Smells

### 3.1. Transport Layer Leak (Coupling with `net/http`)
`ProxyParams` contains `ResponseWriter http.ResponseWriter`. The business service directly writes HTTP headers (`params.ResponseWriter.Header().Set("X-Cache", "HIT")`) and status codes (`params.ResponseWriter.WriteHeader(http.StatusOK)`).
- **Issue:** The service layer is fundamentally locked to HTTP. It cannot be repurposed for gRPC, CLI utilities, Kafka/RabbitMQ async workers, or background evaluation jobs without mocking the `http.ResponseWriter` interface.
- **Remedy:** The service layer should return a domain response object (`*GatewayResponse`, `error`) or a stream reader, leaving HTTP status codes and headers to `internal/handler`.

### 3.2. Concurrency & Cache Stampede in Hot-Path Caching
`userCache` and `tenantCache` use `sync.Map` with manual expiration (`time.Now().Before(item.expiresAt)`).
- **Issue:** Under high load, when a cache entry expires, multiple concurrent requests will simultaneously query the database for the same user/tenant ID (Cache Stampede).
- **Remedy:** Use a dedicated caching decorator with `singleflight.Group` or a dedicated caching abstraction.

---

## 4. Proposed Design Pattern Improvements

To transform `GatewayService` into a clean, maintainable, and extensible architecture, four core design patterns are recommended:

```
                      +------------------------------------------+
                      |         HTTP Handler / Controller        |
                      +------------------------------------------+
                                           |
                                    (Domain Request)
                                           v
+----------------------------------------------------------------------------------+
|                            GATEWAY PIPELINE (Mediator)                           |
|                                                                                  |
|  +----------------+   +----------------+   +----------------+   +-------------+  |
|  | Plugin Stage   |-->| Permission     |-->| Response Cache |-->| Routing     |  |
|  | (Pre/Post Auth)|   | (IAM / Model)  |   | (Check/Store)  |   | & Fallback  |  |
|  +----------------+   +----------------+   +----------------+   +-------------+  |
+----------------------------------------------------------------------------------+
                                           |
                                           | (Strategy)
                       +-------------------+-------------------+
                       v                                       v
        +-----------------------------+         +-----------------------------+
        |     Inbound Codec Registry  |         |   Provider Adapter Registry |
        |   (OpenAI, Anthropic, etc.) |         |  (OpenAI, Anthropic, Gemini)|
        +-----------------------------+         +-----------------------------+
```

### Pattern 1: Chain of Responsibility / Pipeline Pattern
**Target Problem:** Procedural 5-step flow in `HandleRequest` violating SRP and OCP.

Extract each discrete step of request processing into an independent pipeline stage (`Middleware` or `PipelineStage`).

```go
type RequestContext struct {
    Ctx            context.Context
    InboundFormat  string
    RawRequest     []byte
    CanonicalReq   *canonical.CanonicalRequest
    CanonicalResp  *canonical.CanonicalResponse
    Target         ResolvedTarget
    APIKey         *model.APIKey
    User           *model.User
    Tenant         *model.Tenant
    Metadata       map[string]any
}

type Stage func(ctx *RequestContext, next func(*RequestContext) error) error

type Pipeline struct {
    stages []Stage
}

func (p *Pipeline) Use(stages ...Stage) {
    p.stages = append(p.stages, stages...)
}

func (p *Pipeline) Execute(req *RequestContext) error {
    // Standard onion-skin / interceptor execution
}
```

#### Individual Stage Modules:
- `AuthAndPluginStage`: Runs pre-auth and post-auth plugins.
- `PermissionStage`: Evaluates IAM group policies and key permissions.
- `TenantQuotaStage`: Holds credit balance and releases on completion.
- `ResponseCacheStage`: Inspects cache, returns early on cache hit, writes to cache on return.
- `UpstreamDispatchStage`: Invokes the target provider via fallback ladder.

---

### Pattern 2: Strategy & Registry Pattern for Codecs and Providers
**Target Problem:** `if/else` and `switch` statements for protocol translation violating OCP.

Define explicit strategy interfaces for client codecs and upstream adapters:

```go
// 1. Client Protocol Codec Strategy
type InboundCodec interface {
    Format() string // "openai", "anthropic", "gemini"
    DecodeRequest(body []byte) (*canonical.CanonicalRequest, error)
    EncodeResponse(resp *canonical.CanonicalResponse) ([]byte, error)
}

type CodecRegistry interface {
    Register(codec InboundCodec)
    Get(format string) (InboundCodec, bool)
}

// 2. Upstream Provider Adapter Strategy
type UpstreamAdapter interface {
    Provider() provider.ProviderType
    DecodeResponse(body []byte, model string) (*canonical.CanonicalResponse, error)
}

type AdapterRegistry interface {
    Register(adapter UpstreamAdapter)
    Get(provider provider.ProviderType) (UpstreamAdapter, bool)
}
```
*Benefit:* Adding a new inbound protocol or upstream provider requires only implementing the interface and registering it during server bootstrap—**zero modifications** to `GatewayService`.

---

### Pattern 3: Repository / Segregated Interfaces (ISP & DIP)
**Target Problem:** Coupling to concrete `*db.PostgresDB` and `*db.RedisClient`.

Define narrow, consumer-driven interfaces in the `service` package:

```go
// IdentityStore segregates user and tenant queries
type IdentityStore interface {
    GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error)
    GetTenantByID(ctx context.Context, id uuid.UUID) (*model.Tenant, error)
}

// QuotaManager isolates billing holds
type QuotaManager interface {
    ReserveTenantHold(ctx context.Context, key *model.APIKey, provider, model string, promptTokens, maxTokens int) (float64, error)
    ReleaseTenantHold(ctx context.Context, tenantID string, amount float64) error
    CheckTenantBoundary(ctx context.Context, key *model.APIKey, model string) error
}

// PermissionChecker isolates access control
type PermissionChecker interface {
    CheckKeyModelAccess(ctx context.Context, keyID uuid.UUID, model string) (bool, error)
    CheckModelAccess(ctx context.Context, userID uuid.UUID, model string) (bool, error)
}
```

---

### Pattern 4: Decorator Pattern for Hot-Path Caching
**Target Problem:** `userCache` and `tenantCache` polluting `GatewayService` with TTL logic.

Wrap `IdentityStore` with a caching decorator:

```go
type CachedIdentityStore struct {
    next     IdentityStore
    userTTL  time.Duration
    cache    sync.Map
    group    singleflight.Group
}

func NewCachedIdentityStore(store IdentityStore, ttl time.Duration) *CachedIdentityStore {
    return &CachedIdentityStore{next: store, userTTL: ttl}
}

func (c *CachedIdentityStore) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
    // Check in-memory cache, execute via singleflight on miss, populate cache
}
```
*Benefit:* Caching logic is encapsulated and testable independently. `GatewayService` remains completely unaware of whether user data comes from memory, Redis, or PostgreSQL.

---

## 5. Proposed Architecture: Clean `GatewayService`

With these improvements applied, `GatewayService` shifts from an all-knowing God Object to a clean, focused orchestrator:

```go
type GatewayService struct {
    codecs          CodecRegistry
    adapters        AdapterRegistry
    pipeline        *Pipeline
    dispatcher      UpstreamDispatcher
}

// HandleRequest accepts clean domain parameters and returns a domain response
func (g *GatewayService) HandleRequest(ctx context.Context, req DomainRequest) (*DomainResponse, error) {
    codec, ok := g.codecs.Get(req.InboundFormat)
    if !ok {
        return nil, fmt.Errorf("unsupported format: %s", req.InboundFormat)
    }

    canReq, err := codec.DecodeRequest(req.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to decode: %w", err)
    }

    reqCtx := &RequestContext{
        Ctx:          ctx,
        CanonicalReq: canReq,
        Metadata:     req.Metadata,
    }

    if err := g.pipeline.Execute(reqCtx); err != nil {
        return nil, err
    }

    outBytes, err := codec.EncodeResponse(reqCtx.CanonicalResp)
    if err != nil {
        return nil, fmt.Errorf("failed to encode response: %w", err)
    }

    return &DomainResponse{
        StatusCode: http.StatusOK,
        Headers:    reqCtx.ResponseHeaders,
        Body:       outBytes,
    }, nil
}
```

---

## 6. Actionable Refactoring Roadmap

To safely refactor `gateway.go` without breaking existing functionality, execute the refactoring in 4 incremental phases:

### Phase 1: Decouple Transport Layer (Low Risk)
1. Redefine `ProxyParams` to remove `ResponseWriter http.ResponseWriter`.
2. Have `HandleRequest` return a struct containing `StatusCode`, `Headers`, `Body []byte`, and `Stream io.ReadCloser`.
3. Update `internal/handler/gateway.go` to handle writing headers, status codes, and bodies.
*Outcome: Eliminates double-write bugs and isolates HTTP concerns.*

### Phase 2: Interface Segregation & Codec Registry (Medium Risk)
1. Define `InboundCodec` and `UpstreamAdapter` interfaces.
2. Move `openai`, `anthropic`, and `gemini` decode/format logic out of `gateway.go` into dedicated adapter implementations.
3. Instantiate and inject `CodecRegistry` and `AdapterRegistry`.
*Outcome: Satisfies OCP; enables unit testing of codecs without `GatewayService`.*

### Phase 3: Extract Identity Caching & Narrow Stores (Low Risk)
1. Define `IdentityStore` and `QuotaManager` interfaces.
2. Implement `CachedIdentityStore` decorator and remove `userCache` and `tenantCache` from `GatewayService`.
3. Replace `*db.PostgresDB` and `*db.RedisClient` fields in `GatewayService` with the new interfaces.
*Outcome: Satisfies ISP and DIP; eliminates cache stampede risks.*

### Phase 4: Convert Request Lifecycle to Pipeline / Interceptors (Medium Risk)
1. Implement a lightweight `Pipeline` engine.
2. Convert Plugins, IAM check, Tenant Quotas, Response Caching, and Fallback Dispatch into composable stages.
3. Reduce `HandleRequest` to a lean orchestrator that initializes the context and runs the pipeline.
*Outcome: Satisfies SRP; simplifies lifecycle stage reordering and extensions.*
