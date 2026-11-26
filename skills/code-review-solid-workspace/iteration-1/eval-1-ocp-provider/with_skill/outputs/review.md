# Code Review Report: SOLID & Design Patterns

## Scope

- **Target Package:** [`internal/provider`](file:///data/per-pro/llm-gateway/internal/provider) ([`client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go), [`keystore.go`](file:///data/per-pro/llm-gateway/internal/provider/keystore.go)) and its direct call-sites across [`internal/service`](file:///data/per-pro/llm-gateway/internal/service) ([`gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go), [`gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go), [`fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go), [`gateway_discovery.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go), [`gateway_embeddings.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_embeddings.go)) and [`internal/adapter`](file:///data/per-pro/llm-gateway/internal/adapter).
- **Language:** Go (1.22+).
- **Existing Architectural Patterns:**
  - **Layered Architecture:** Clear separation between entry handlers (`internal/handler`), orchestration service (`internal/service`), downstream providers (`internal/provider`), protocol adapters (`internal/adapter`), and storage/cache (`internal/db`).
  - **Canonical Data Model (CDM):** Inter-provider exchange format encapsulated via [`canonical.CanonicalRequest`](file:///data/per-pro/llm-gateway/internal/adapter/canonical/canonical.go#L67-L93), [`canonical.CanonicalResponse`](file:///data/per-pro/llm-gateway/internal/adapter/canonical/canonical.go#L95-L108), and [`canonical.CanonicalStreamChunk`](file:///data/per-pro/llm-gateway/internal/adapter/canonical/canonical.go#L110-L135).
  - **Secure Keystore:** AES-GCM encrypted provider credentials in PostgreSQL with Redis sub-millisecond caching in [`KeyStore`](file:///data/per-pro/llm-gateway/internal/provider/keystore.go#L21-L33).
- **Core Problem Statement:** Adding any new LLM provider (or maintaining existing ones like OpenAI, Anthropic, Gemini) currently requires modifying multiple disparate files across the codebase due to tight coupling, absence of abstraction interfaces, and pervasive hardcoded `switch` statements.

---

## SOLID Audit

### ⚠️ S — Single Responsibility Principle

**Finding:**
[`ProviderClient`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L23-L25) violates the Single Responsibility Principle by accumulating multiple orthogonal concerns:
1. **HTTP Connection Pooling & Transport Execution:** Managing keep-alives, connection pooling, and issuing raw HTTP requests ([`NewProviderClient`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L27-L47), [`Do`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L50-L52)).
2. **Vendor-Specific Upstream Request Construction:** Knowledge of wire formats, authorization headers (Bearer vs `x-api-key`), URL endpoints, version headers, and query parameter escaping (e.g. Gemini `alt=sse` query params in [`buildRequestDetails`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L319-L373)).
3. **Vendor-Specific Catalog Listing:** Querying and decoding JSON responses for upstream model lists across different vendors ([`listAnthropicModels`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L74-L109), [`listOpenAIModels`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L167-L201), [`listGeminiModels`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L203-L238)).
4. **Model Name Heuristic Resolution:** Parsing model name prefixes via [`ResolveProvider`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L240-L249).

Additionally, [`KeyStore`](file:///data/per-pro/llm-gateway/internal/provider/keystore.go#L21-L33) combines cryptographic key derivation/encryption, Redis cache lifecycle management, and PostgreSQL database queries.

**Before ([`client.go#L319-L373`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L319-L373)):**
```go
// ❌ ProviderClient directly builds endpoints, headers, and query strings for every vendor
func (c *ProviderClient) buildRequestDetails(req UpstreamRequest, apiKey string) (string, map[string]string, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	switch req.Provider {
	case ProviderOpenAI:
		return "https://api.openai.com/v1/chat/completions", headers, nil
	case ProviderAnthropic:
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
		return "https://api.anthropic.com/v1/messages", headers, nil
	case ProviderGemini:
		query := "key=" + url.QueryEscape(apiKey)
		endpoint := "generateContent"
		if req.Stream {
			endpoint = "streamGenerateContent"
			query = "alt=sse&" + query
		}
		return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:%s?%s", req.Model, endpoint, query), headers, nil
	default:
		return "", nil, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
}
```

**After:**
```go
// ✅ ProviderClient delegates request building to the provider strategy and only executes HTTP transport
type ProviderClient struct {
	httpClient *http.Client
}

func (c *ProviderClient) Send(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req.WithContext(ctx))
}
```

---

### ⚠️ O — Open/Closed Principle (Critical Violation 🔴)

**Finding:**
The codebase is closed to extension and wide open to modification. Every time a new LLM provider is introduced (e.g., Mistral, Cohere, Bedrock, Groq), developers must manually modify at least 6 files containing hardcoded `switch` or `if-else` blocks:
1. [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L15-L21): Add to `ProviderType` enum.
2. [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L61-L72): Add `case` in `ListModels` and implement a new `list<Provider>Models` method.
3. [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L319-L373): Add `case` in `buildRequestDetails` (for both custom base URLs and built-ins).
4. [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L240-L249): Add model name prefix matching in `ResolveProvider`.
5. [`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L503-L510): Add `case` in `handleNonStreamResponse` to decode upstream response.
6. [`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L265): Add `case` in SSE stream parser.
7. [`internal/service/fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L136-L145): Add `case` in `encodeForTarget`.
8. [`internal/service/gateway_discovery.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go#L77-L81): Append to hardcoded provider slice in `ListModels`.

**Before ([`internal/service/gateway.go#L501-L511`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L501-L511) & [`internal/service/fallback.go#L131-L146`](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L131-L146)):**
```go
// ❌ Gateway and Fallback service must be edited whenever a provider is added
func encodeForTarget(canReq *canonical.CanonicalRequest, target ResolvedTarget, modelForTarget string) ([]byte, error) {
	reqCopy := *canReq
	reqCopy.Model = modelForTarget

	switch target.Provider {
	case provider.ProviderOpenAI:
		return openai.EncodeCanonicalRequest(&reqCopy)
	case provider.ProviderAnthropic:
		return anthropic.EncodeCanonicalRequest(&reqCopy)
	case provider.ProviderGemini:
		return gemini.EncodeCanonicalRequest(&reqCopy)
	default:
		return nil, fmt.Errorf("unsupported provider type %v", target.Provider)
	}
}
```

**After:**
```go
// ✅ Polymorphic dispatch via a Provider interface and Registry: open for extension, closed for modification
func (s *GatewayService) encodeForTarget(canReq *canonical.CanonicalRequest, target ResolvedTarget, modelForTarget string) ([]byte, error) {
	reqCopy := *canReq
	reqCopy.Model = modelForTarget

	// ℹ️ Provider resolved dynamically from registry without switch statements
	p, err := s.registry.Get(target.Provider)
	if err != nil {
		return nil, err
	}
	return p.EncodeRequest(&reqCopy)
}
```

---

### ⚠️ L — Liskov Substitution Principle

**Finding:**
There is currently no formal interface contract representing a provider. Concrete behaviors vary drastically and leak into the orchestration layer:
- In [`internal/service/gateway_stream.go#L214-L265`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L265), the consumer assumes OpenAI streams terminate with `event.Data == "[DONE]"`, while Anthropic streams emit event types (`content_block_delta`, `message_delta`), and Gemini emits raw JSON objects without termination sentinels. The caller must know these implementation details because there is no substitutable stream abstraction.
- In [`internal/provider/client.go#L54-L56`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L54-L56), `ErrNoModelsEndpoint` was defined as an ad-hoc error contract for Anthropic, while other providers returned valid slices. Later, Anthropic model listing was implemented via a different schema format ([`client.go#L93-L100`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L93-L100)), while Gemini returns IDs with a `"models/"` prefix requiring trimming ([`client.go#L232`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L232)).
- Any provider implementation must be a drop-in replacement that normalizes streaming events, model IDs, and errors to the canonical domain model.

**Before ([`internal/service/gateway_stream.go#L214-L265`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L265)):**
```go
// ❌ Leaky abstraction: caller must know the idiosyncratic streaming semantics of each provider
switch target.Provider {
case provider.ProviderOpenAI:
	if string(event.Data) == "[DONE]" {
		chunk = &canonical.CanonicalStreamChunk{IsLast: true}
	} else {
		c, cErr := openai.DecodeStreamChunk(event.Data, canReq.Model)
		// ...
	}
case provider.ProviderAnthropic:
	c, cErr := anthropic.DecodeStreamChunk(event.Event, event.Data, canReq.Model)
	// ...
case provider.ProviderGemini:
	c, cErr := gemini.DecodeStreamChunk(event.Data, canReq.Model)
	// ...
}
```

**After:**
```go
// ✅ Subtypes satisfy the exact same interface contract; consumers do not inspect wire details
chunk, err := p.DecodeStreamChunk(event.Event, event.Data, canReq.Model)
if err != nil {
	return err
}
if chunk.IsLast {
	// Uniform termination across all providers
}
```

---

### ⚠️ I — Interface Segregation Principle

**Finding:**
Currently, `internal/provider` contains **zero interfaces**; callers depend entirely on concrete types ([`*ProviderClient`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L23-L25), [`*KeyStore`](file:///data/per-pro/llm-gateway/internal/provider/keystore.go#L21-L25)).
Furthermore, LLM providers do not have identical capabilities:
- Some providers support Chat Completions + Streaming (OpenAI, Anthropic, Gemini).
- Some support Embeddings (OpenAI, Gemini), while others do not (Anthropic).
- Some provide live Model Discovery APIs (OpenAI, Gemini), while others rely on static tables or require different administrative keys.

Forcing all providers into a single fat interface or concrete implementation leads to dummy implementations (`ErrNoModelsEndpoint` or panic stubs). Segregated interfaces allow providers to declare only what they support.

**Before:**
```go
// ❌ No interfaces. ProviderClient is a monolithic concrete struct with all capabilities lumped together.
type ProviderClient struct { httpClient *http.Client }
func (c *ProviderClient) ListModels(...) ([]string, error)
func (c *ProviderClient) ListModelsCustom(...) ([]string, error)
func (c *ProviderClient) SendRequest(...) (*UpstreamResponse, error)
```

**After:**
```go
// ✅ Small, focused capability interfaces defined at the abstraction boundary
type ChatProvider interface {
	Name() string
	BuildChatRequest(ctx context.Context, req *canonical.CanonicalRequest, apiKey, baseURL string) (*http.Request, error)
	DecodeChatResponse(body []byte) (*canonical.CanonicalResponse, error)
	DecodeStreamChunk(eventType string, data []byte, model string) (*canonical.CanonicalStreamChunk, error)
}

type ModelLister interface {
	ListModels(ctx context.Context, apiKey, baseURL string) ([]string, error)
}

type EmbeddingProvider interface {
	BuildEmbeddingRequest(ctx context.Context, model string, inputs []string, apiKey, baseURL string) (*http.Request, error)
	DecodeEmbeddingResponse(body []byte) (*canonical.EmbeddingResponse, error)
}
```

---

### ⚠️ D — Dependency Inversion Principle

**Finding:**
High-level modules in [`internal/service`](file:///data/per-pro/llm-gateway/internal/service) depend directly on concrete low-level packages:
- [`GatewayService`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L81-L97) has direct struct fields `providerClient *provider.ProviderClient` and `keyStore *provider.KeyStore`.
- [`handleNonStreamResponse`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L503-L510), [`fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L136-L145), and [`gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L265) directly import and execute low-level packages:
  - `"llm-gateway/internal/adapter/openai"`
  - `"llm-gateway/internal/adapter/anthropic"`
  - `"llm-gateway/internal/adapter/gemini"`
- Hardcoded URLs like `"https://api.openai.com/v1/chat/completions"` and `"https://generativelanguage.googleapis.com/v1beta/..."` are directly embedded in [`internal/provider/client.go#L347-L368`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L347-L368).

Both high-level services and low-level adapters should depend on abstractions (interfaces).

**Before ([`internal/service/gateway.go#L501-L511`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L501-L511)):**
```go
// ❌ High-level GatewayService directly couples to concrete adapter packages
import (
	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
)
```

**After:**
```go
// ✅ High-level GatewayService depends exclusively on the Provider interface/registry
type ProviderRegistry interface {
	Get(name string) (ChatProvider, error)
	ListListers() []ModelLister
}

type GatewayService struct {
	providers  ProviderRegistry // ℹ️ Interface injection
	httpClient HTTPExecutor
	// ...
}
```

---

## Design Pattern Opportunities

### 1. Strategy & Adapter Patterns in `internal/provider`

**Why:** Replace the fragmented encoding/decoding and URL construction scattered across `client.go`, `gateway.go`, `gateway_stream.go`, and `fallback.go` with self-contained, interchangeable provider adapters conforming to a common Strategy interface.

**Before ([`internal/provider/client.go#L345-L373`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L345-L373)):**
```go
// ❌ Ad-hoc procedural logic branching on provider enum
switch req.Provider {
case ProviderOpenAI:
	url := "https://api.openai.com/v1/chat/completions"
	headers["Authorization"] = "Bearer " + apiKey
	return url, headers, nil
case ProviderAnthropic:
	url := "https://api.anthropic.com/v1/messages"
	headers["x-api-key"] = apiKey
	headers["anthropic-version"] = "2023-06-01"
	return url, headers, nil
case ProviderGemini:
	query := "key=" + url.QueryEscape(apiKey)
	endpoint := "generateContent"
	if req.Stream { endpoint = "streamGenerateContent"; query = "alt=sse&" + query }
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:%s?%s", req.Model, endpoint, query), headers, nil
}
```

**After ([`internal/provider/adapter.go`](file:///data/per-pro/llm-gateway/internal/provider/adapter.go)):**
```go
// ✅ Adapter Pattern: Each provider encapsulates its wire protocol, endpoints, and translation
package provider

import (
	"context"
	"net/http"
	"llm-gateway/internal/adapter/canonical"
)

type Provider interface {
	Name() string
	BuildChatRequest(ctx context.Context, req *canonical.CanonicalRequest, apiKey, baseURL string) (*http.Request, error)
	DecodeChatResponse(body []byte, model string) (*canonical.CanonicalResponse, error)
	DecodeStreamChunk(eventType string, data []byte, model string) (*canonical.CanonicalStreamChunk, error)
}
```

---

### 2. Registry Pattern (Factory Method) in `internal/provider`

**Why:** Eliminate all `switch` statements used for provider dispatch and enable plug-and-play addition of new providers (e.g., `mistral`, `groq`, `bedrock`) without modifying existing gateway logic.

**Before ([`internal/provider/client.go#L61-L72`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L61-L72)):**
```go
// ❌ Closed enum dispatch requiring modification of client.go on every new provider
func (c *ProviderClient) ListModels(ctx context.Context, p ProviderType, apiKey string) ([]string, error) {
	switch p {
	case ProviderOpenAI:
		return c.listOpenAIModels(ctx, apiKey)
	case ProviderAnthropic:
		return c.listAnthropicModels(ctx, apiKey)
	case ProviderGemini:
		return c.listGeminiModels(ctx, apiKey)
	default:
		return nil, ErrNoModelsEndpoint
	}
}
```

**After ([`internal/provider/registry.go`](file:///data/per-pro/llm-gateway/internal/provider/registry.go)):**
```go
// ✅ Registry Pattern: Thread-safe, self-registering provider registry
package provider

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

var defaultRegistry = &Registry{providers: make(map[string]Provider)}

func Register(p Provider) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	defaultRegistry.providers[p.Name()] = p
}

func Get(name string) (Provider, error) {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	p, ok := defaultRegistry.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}

func All() []Provider {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	res := make([]Provider, 0, len(defaultRegistry.providers))
	for _, p := range defaultRegistry.providers {
		res = append(res, p)
	}
	return res
}
```

With this pattern, adding Mistral is as simple as creating `internal/provider/mistral/mistral.go` containing:
```go
func init() {
	provider.Register(&MistralAdapter{})
}
```
No existing gateway, service, or client files are modified.

---

### 3. Facade Pattern in `internal/provider/client.go`

**Why:** Decouple HTTP client connection pooling, metrics, and network execution from vendor wire details. `ProviderClient` becomes a clean Facade that accepts a `Provider` strategy, requests its `http.Request`, and executes it through the pooled transport.

**Before ([`internal/provider/client.go#L282-L317`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L282-L317)):**
```go
// ❌ SendRequest directly handles apiKey validation, URL building, header manipulation, and dispatch
func (c *ProviderClient) SendRequest(ctx context.Context, req UpstreamRequest, apiKey string) (*UpstreamResponse, error) {
	url, headers, err := c.buildRequestDetails(req, apiKey)
	// ...
}
```

**After:**
```go
// ✅ Facade: ProviderClient manages transport execution; delegates wire details to Provider strategy
func (c *ProviderClient) ExecuteChat(
	ctx context.Context,
	p Provider,
	canReq *canonical.CanonicalRequest,
	apiKey, baseURL string,
	extraHeaders map[string]string,
	removeHeaders []string,
) (*http.Response, error) {
	httpReq, err := p.BuildChatRequest(ctx, canReq, apiKey, baseURL)
	if err != nil {
		return nil, fmt.Errorf("%s build request: %w", p.Name(), err)
	}

	for k, v := range extraHeaders {
		httpReq.Header.Set(k, v)
	}
	for _, r := range removeHeaders {
		httpReq.Header.Del(r)
	}

	return c.httpClient.Do(httpReq)
}
```

---

## Summary Table

| # | Principle / Pattern | Severity | Primary Files Affected | Status |
|---|---|---|---|---|
| 1 | **Open/Closed Principle (OCP)** | 🔴 High | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go), [`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go), [`internal/service/fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go), [`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go) | Proposed Registry & Strategy patterns |
| 2 | **Single Responsibility (SRP)** | 🟡 Medium | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go), [`internal/provider/keystore.go`](file:///data/per-pro/llm-gateway/internal/provider/keystore.go) | Propose splitting HTTP pooling from vendor wire logic |
| 3 | **Dependency Inversion (DIP)** | 🔴 High | [`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go), [`internal/service/fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go) | Propose injecting `ProviderRegistry` interface |
| 4 | **Interface Segregation (ISP)** | 🟡 Medium | [`internal/provider`](file:///data/per-pro/llm-gateway/internal/provider) | Propose granular `ChatProvider`, `StreamDecoder`, `ModelLister` |
| 5 | **Liskov Substitution (LSP)** | 🟡 Medium | [`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go), [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go) | Propose uniform chunk & model response contracts |
| 6 | **Strategy Pattern** | 🔴 High | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go) ➔ `internal/provider/adapter.go` | Proposed interchangeable provider strategy |
| 7 | **Registry / Factory Pattern** | 🔴 High | `internal/provider/registry.go` (new) | Proposed self-registering provider catalog |
| 8 | **Facade Pattern** | 🟢 Low | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go) | Simplify client to pure HTTP transport execution |

---

## Recommended Next Steps

1. **Step 1: Define the `Provider` Strategy & Capability Interfaces**
   Create `internal/provider/provider.go` defining [`ChatProvider`](file:///data/per-pro/llm-gateway/internal/provider), [`ModelLister`](file:///data/per-pro/llm-gateway/internal/provider), and [`EmbeddingProvider`](file:///data/per-pro/llm-gateway/internal/provider). Ensure the methods take canonical types and return standard `*http.Request`, `*canonical.CanonicalResponse`, or `*canonical.CanonicalStreamChunk`.

2. **Step 2: Implement the `ProviderRegistry` (Registry / Factory)**
   Create `internal/provider/registry.go` providing `Register(name string, p Provider)` and `Get(name string) (Provider, error)`. Replace the hardcoded `ProviderType` enum with string-based dynamic registry lookup.

3. **Step 3: Wrap Existing Adapters into the Provider Interface**
   In [`internal/adapter/openai`](file:///data/per-pro/llm-gateway/internal/adapter/openai), [`internal/adapter/anthropic`](file:///data/per-pro/llm-gateway/internal/adapter/anthropic), and [`internal/adapter/gemini`](file:///data/per-pro/llm-gateway/internal/adapter/gemini), expose adapter structs satisfying `ChatProvider` and `ModelLister`. Move vendor-specific URL, header, and query string logic from [`client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go) directly into their respective adapter packages.

4. **Step 4: Refactor `GatewayService` to Use Polymorphic Dispatch**
   In [`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L501-L511), [`gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L265), and [`fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L131-L146), replace the `switch target.Provider` statements with a single call to `provider, err := registry.Get(target.Name)` followed by `provider.DecodeChatResponse(...)` or `provider.DecodeStreamChunk(...)`.

5. **Step 5: Simplify Model Discovery (`gateway_discovery.go`)**
   In [`internal/service/gateway_discovery.go#L76-L82`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go#L76-L82), replace the hardcoded `providers := []provider.ProviderType{...}` slice with `registry.AllListers()`, enabling automatic dynamic discovery for any registered provider.
