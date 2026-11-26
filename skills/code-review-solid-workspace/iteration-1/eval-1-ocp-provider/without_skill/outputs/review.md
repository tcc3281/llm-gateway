# Code Review & Architecture Analysis: `internal/provider` Package

## 1. Executive Summary

A comprehensive architectural review of [`internal/provider`](file:///data/per-pro/llm-gateway/internal/provider) and its consumers across [`internal/service`](file:///data/per-pro/llm-gateway/internal/service) was conducted to diagnose why introducing new LLM providers (e.g., OpenAI, Anthropic, Gemini) requires modifying numerous disjoint files across the codebase.

The root cause is a fundamental violation of the **Open/Closed Principle (OCP)** (the "O" in SOLID design): the codebase is currently **closed to extension and open to modification**. Provider dispatching, URL construction, authentication headers, request serialization, response deserialization, streaming SSE parsing, model listing, and fallback translation are implemented via hardcoded `switch` statements and `if/else` ladders scattered across at least seven different files.

To support adding new providers without modifying existing core logic, the system should transition to an **Interface Abstraction (Strategy Pattern)** coupled with a **Provider Registry & Factory Pattern**.

---

## 2. Core Problem: Open/Closed Principle (OCP) Violation & Shotgun Surgery

### 2.1 The "Shotgun Surgery" Anti-Pattern
When adding a new provider (or even tweaking an existing one), developers are forced to touch multiple distinct components across the repository. This anti-pattern is known as **Shotgun Surgery** (Fowler, 1999) and directly stems from violating OCP.

```
                    +-------------------------------------+
                    | Adding a New Provider (e.g. Cohere) |
                    +-------------------------------------+
                                       |
       +---------------+---------------+---------------+---------------+
       |               |               |               |               |
       v               v               v               v               v
internal/provider/  internal/service/  internal/service/  internal/service/  internal/service/
    client.go          gateway.go       gateway_stream.go    fallback.go    gateway_embeddings.go
 [Switches in:      [Switch in:         [Switch in:         [Switches in:    [Hardcoded if/else
  - ListModels       - handleNonStream   - DecodeStream      - fallbackChain  for Gemini vs
  - ListCustom         Response]           Chunk]            - fallbackModel  OpenAI/Anthropic]
  - buildRequest]                        - tool accumulator] - encodeTarget]
```

### 2.2 Concrete Manifestations Across the Codebase

#### 1. [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go)
- **Hardcoded enum**: [`ProviderType`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L15-L21) contains static constants (`ProviderOpenAI`, `ProviderAnthropic`, `ProviderGemini`).
- **`ListModels` switch**: [`ListModels`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L61-L72) executes a `switch p` over every known provider to call private helper methods (`listOpenAIModels`, `listAnthropicModels`, `listGeminiModels`).
- **`ListModelsCustom` switch**: [`ListModelsCustom`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L114-L128) hardcodes protocol branches for `ProviderOpenAI` vs `ProviderAnthropic`.
- **`ResolveProvider` static prefixes**: [`ResolveProvider`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L240-L249) inspects model name prefixes with hardcoded string checks (`"claude"`, `"gemini"`).
- **`buildRequestDetails` switch**: [`buildRequestDetails`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L319-L373) contains two nested switches: one for custom base URLs and one for upstream endpoints, headers, and query parameter formatting (e.g., Gemini's custom `:streamGenerateContent?key=...` query structure).

#### 2. [`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go)
- **Response decoding switch**: [`handleNonStreamResponse`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L503-L510) uses a `switch target.Provider` to dispatch between `openai.DecodeUpstreamResponse`, `anthropic.DecodeUpstreamResponse`, and `gemini.DecodeUpstreamResponse`.

#### 3. [`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go)
- **Streaming SSE chunk translation switch**: Lines [214-270](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L270) switch on `target.Provider` to decode SSE events into [`canonical.CanonicalStreamChunk`](file:///data/per-pro/llm-gateway/internal/adapter/canonical/canonical.go) and extract provider-specific token usages.
- **Provider-specific tool call accumulator**: Lines [285-290](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L285-L290) embed provider-specific branching (`if target.Provider == provider.ProviderGemini { key = "g:" + dtc.Name }`) directly within the generic streaming loop.

#### 4. [`internal/service/fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go)
- **Fallback provider translation switch**: Lines [58-70](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L58-L70) map provider name strings to `provider.ProviderType`.
- **Fallback model selection switch**: Lines [98-128](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L98-L128) hardcode provider names (`"anthropic"`, `"gemini"`, `"openai"`) and model families.
- **Request encoding switch**: Lines [136-146](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L136-L146) switch on `target.Provider` to invoke `openai.EncodeCanonicalRequest`, `anthropic.EncodeCanonicalRequest`, or `gemini.EncodeCanonicalRequest`.

#### 5. [`internal/service/gateway_discovery.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go)
- **Hardcoded provider slice**: Lines [77-81](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go#L77-L81) hardcode `[]provider.ProviderType{ProviderOpenAI, ProviderAnthropic, ProviderGemini}` for catalog discovery.

#### 6. [`internal/service/gateway_embeddings.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_embeddings.go)
- **Hardcoded protocol branches**: Lines [125-145](file:///data/per-pro/llm-gateway/internal/service/gateway_embeddings.go#L125-L145) and [291-296](file:///data/per-pro/llm-gateway/internal/service/gateway_embeddings.go#L291-L296) branch on whether `target.Provider` is Gemini, OpenAI, or Anthropic.

---

## 3. Recommended Architecture: Interface Abstraction & Registry Pattern

To eliminate shotgun surgery and adhere to OCP, provider-specific details must be encapsulated behind a **polymorphic interface**, and instances must be managed through a **Provider Registry / Factory**.

### 3.1 Architectural Overview

```
                      +-----------------------------+
                      |      Provider Registry      |
                      |  (Factory / Service Locator)|
                      +-----------------------------+
                                     |
             +-----------------------+-----------------------+
             |                       |                       |
             v                       v                       v
     +---------------+       +---------------+       +---------------+
     | OpenAIAdapter |       |AnthropicAdapt.|       | GeminiAdapter |
     | (implements   |       | (implements   |       | (implements   |
     |   Provider)   |       |   Provider)   |       |   Provider)   |
     +---------------+       +---------------+       +---------------+
```

With this architecture:
1. **Adding a new provider** involves creating a single new struct that implements the `Provider` interface and registering it via `registry.Register(myProvider)`.
2. **Existing files remain untouched**—`gateway.go`, `gateway_stream.go`, `fallback.go`, and `client.go` execute against the interface abstraction.

---

## 4. Proposed Design & Interface Abstraction

### 4.1 The `Provider` Interface Abstraction

We define a standardized provider interface in `internal/provider` (or unifying with `internal/adapter`) that encapsulates all provider-specific behaviors:

```go
package provider

import (
	"context"
	"net/http"

	"llm-gateway/internal/adapter/canonical"
)

// Provider defines the contract that each LLM upstream integration must fulfill.
type Provider interface {
	// ID returns the unique provider identifier (e.g., "openai", "anthropic", "gemini").
	ID() ProviderType

	// MatchesModel reports whether this provider handles the given model identifier.
	MatchesModel(model string) bool

	// BuildChatRequest constructs the upstream HTTP request for chat completions.
	BuildChatRequest(ctx context.Context, baseURL, apiKey string, req *canonical.CanonicalRequest, stream bool) (*http.Request, error)

	// DecodeResponse deserializes a non-streaming HTTP response into the canonical format.
	DecodeResponse(statusCode int, body []byte, model string) (*canonical.CanonicalResponse, error)

	// DecodeStreamChunk parses an individual SSE event into a canonical chunk.
	DecodeStreamChunk(event SSEEvent, model string) (*canonical.CanonicalStreamChunk, error)

	// ToolCallKey returns a stable key for grouping incremental tool call deltas.
	ToolCallKey(delta canonical.DeltaToolCall) string

	// ListModels queries upstream for available model names.
	ListModels(ctx context.Context, httpClient *http.Client, baseURL, apiKey string) ([]string, error)

	// FallbackModel returns a suitable alternative model for this provider given an input tier.
	FallbackModel(originalModel string, isLightTier bool) string
}

// SSEEvent represents a raw server-sent event line.
type SSEEvent struct {
	Event string
	Data  []byte
}
```

### 4.2 The Provider Registry (Factory Pattern)

The Registry acts as a thread-safe factory and catalog for provider implementations.

```go
package provider

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderType]Provider
}

var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderType]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

// Get retrieves a provider by its type.
func (r *Registry) Get(pt ProviderType) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[pt]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}
	return p, nil
}

// ResolveForModel returns the provider that handles the requested model.
func (r *Registry) ResolveForModel(model string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if p.MatchesModel(model) {
			return p, nil
		}
	}
	// Fallback to default (OpenAI protocol)
	if p, ok := r.providers[ProviderOpenAI]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("no provider found for model %q", model)
}

// All returns a slice of all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	return list
}
```

---

## 5. Concrete Code Comparison: Before vs. After

### 5.1 Request Dispatching & Construction

#### Before ([`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L319-L373))
Adding a provider requires editing the monolithic `buildRequestDetails` method with unique URL formatting, query strings, and auth schemes:
```go
// BEFORE: Monolithic switch requiring modification for every new provider
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

#### After: Polymorphic Dispatch via Registry & Provider Adapter
The dispatch code never changes; it delegates directly to the registered `Provider` implementation:
```go
// AFTER: Zero modifications required when new providers are added
func (c *ProviderClient) SendRequest(ctx context.Context, pt ProviderType, req *canonical.CanonicalRequest, apiKey string, stream bool) (*UpstreamResponse, error) {
	p, err := c.registry.Get(pt)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.BuildChatRequest(ctx, c.baseURL, apiKey, req, stream)
	if err != nil {
		return nil, fmt.Errorf("provider %s failed to build request: %w", pt, err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed sending request to %s: %w", pt, err)
	}

	return &UpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}
```

---

### 5.2 Non-Streaming Response Handling

#### Before ([`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go#L503-L510))
```go
// BEFORE: Hardcoded switch across adapter packages
switch target.Provider {
case provider.ProviderOpenAI:
	canResp, err = openai.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderAnthropic:
	canResp, err = anthropic.DecodeUpstreamResponse(bodyBytes)
case provider.ProviderGemini:
	canResp, err = gemini.DecodeUpstreamResponse(bodyBytes, canReq.Model)
}
```

#### After
```go
// AFTER: Single call to the provider interface
p, err := g.providerRegistry.Get(target.Provider)
if err != nil {
	return err
}
canResp, err := p.DecodeResponse(upResp.StatusCode, bodyBytes, canReq.Model)
```

---

### 5.3 Streaming SSE Event Processing

#### Before ([`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L214-L290))
Each provider has custom logic for `[DONE]`, extra arguments, token counters, and tool-call keys:
```go
// BEFORE: 60+ lines of branching in the middle of the streaming loop
switch target.Provider {
case provider.ProviderOpenAI:
	if string(event.Data) == "[DONE]" {
		chunk = &canonical.CanonicalStreamChunk{IsLast: true}
	} else {
		c, cErr := openai.DecodeStreamChunk(event.Data, canReq.Model)
		// ... token accumulation ...
	}
case provider.ProviderAnthropic:
	c, cErr := anthropic.DecodeStreamChunk(event.Event, event.Data, canReq.Model)
	// ... token accumulation ...
case provider.ProviderGemini:
	c, cErr := gemini.DecodeStreamChunk(event.Data, canReq.Model)
	// ... token accumulation ...
}

// Tool call keying has another provider-specific check:
for _, dtc := range chunk.DeltaToolCalls {
	key := strconv.Itoa(dtc.Index)
	if target.Provider == provider.ProviderGemini {
		key = "g:" + dtc.Name
	}
	// ...
}
```

#### After
All provider specifics are encapsulated inside the provider adapter:
```go
// AFTER: Clean, uniform stream loop
p, err := g.providerRegistry.Get(target.Provider)
if err != nil {
	return err
}

chunk, cErr := p.DecodeStreamChunk(SSEEvent{Event: event.Event, Data: event.Data}, canReq.Model)
if cErr != nil || chunk == nil {
	continue
}

// Universal token accounting using normalized fields
accumulateTokens(chunk)

// Uniform tool-call keying
for _, dtc := range chunk.DeltaToolCalls {
	key := p.ToolCallKey(dtc)
	// ...
}
```

---

### 5.4 How a New Provider Is Added (e.g., Mistral / Cohere / DeepSeek)

Under the refactored architecture, adding support for a new provider (e.g. `DeepSeek`) requires **zero modifications** to existing core code. The engineer creates one self-contained file:

```go
package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/provider"
)

type DeepSeekProvider struct{}

func init() {
	provider.DefaultRegistry.Register(&DeepSeekProvider{})
}

func (d *DeepSeekProvider) ID() provider.ProviderType {
	return "deepseek"
}

func (d *DeepSeekProvider) MatchesModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "deepseek")
}

func (d *DeepSeekProvider) BuildChatRequest(ctx context.Context, baseURL, apiKey string, req *canonical.CanonicalRequest, stream bool) (*http.Request, error) {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	payload, err := openai.EncodeCanonicalRequest(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func (d *DeepSeekProvider) DecodeResponse(status int, body []byte, model string) (*canonical.CanonicalResponse, error) {
	return openai.DecodeUpstreamResponse(body)
}

func (d *DeepSeekProvider) DecodeStreamChunk(event provider.SSEEvent, model string) (*canonical.CanonicalStreamChunk, error) {
	if string(event.Data) == "[DONE]" {
		return &canonical.CanonicalStreamChunk{IsLast: true}, nil
	}
	return openai.DecodeStreamChunk(event.Data, model)
}

func (d *DeepSeekProvider) ToolCallKey(dtc canonical.DeltaToolCall) string {
	return fmt.Sprintf("%d", dtc.Index)
}

func (d *DeepSeekProvider) ListModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	return openai.ListModels(ctx, client, baseURL, apiKey)
}

func (d *DeepSeekProvider) FallbackModel(orig string, isLight bool) string {
	if isLight {
		return "deepseek-chat"
	}
	return "deepseek-reasoner"
}
```

---

## 6. Structured Summary & Action Items

| ID | Issue / Finding | Severity | Affected Files | Proposed Remedy |
|:---|:---|:---:|:---|:---|
| **FIND-01** | **Open/Closed Principle Violation**<br>New providers force modifications across core routing, client, fallback, and streaming logic. | **High** | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go)<br>[`internal/service/gateway.go`](file:///data/per-pro/llm-gateway/internal/service/gateway.go)<br>[`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go) | Define uniform `Provider` interface abstraction and replace all hardcoded switches with polymorphic method calls. |
| **FIND-02** | **Lack of Central Provider Registry / Factory**<br>Provider instances are instantiated ad-hoc without discovery or dynamic lookup. | **High** | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go)<br>[`internal/service/gateway_discovery.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go) | Introduce `ProviderRegistry` with thread-safe registration (`Register`) and lookup (`Get`, `ResolveForModel`). |
| **FIND-03** | **Scattered Provider Quirk Handling**<br>Special cases (Gemini tool-call keys, query string formatting, Anthropic header requirements) pollute generic service logic. | **Medium** | [`internal/service/gateway_stream.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_stream.go#L287)<br>[`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L358-L368) | Encapsulate provider quirks inside their respective adapter structs (`ToolCallKey`, `BuildChatRequest`). |
| **FIND-04** | **Hardcoded Model-to-Provider Prefix Resolution**<br>`ResolveProvider` and fallback matching hardcode `"claude"`, `"gemini"`. | **Medium** | [`internal/provider/client.go`](file:///data/per-pro/llm-gateway/internal/provider/client.go#L240-L249)<br>[`internal/service/fallback.go`](file:///data/per-pro/llm-gateway/internal/service/fallback.go#L98-L128) | Delegate model matching to `Provider.MatchesModel()` and fallback mapping to `Provider.FallbackModel()`. |
| **FIND-05** | **Hardcoded Provider Slice in Discovery**<br>Discovery only queries OpenAI, Anthropic, and Gemini; new providers are excluded from background model discovery. | **Low** | [`internal/service/gateway_discovery.go`](file:///data/per-pro/llm-gateway/internal/service/gateway_discovery.go#L77-L81) | Iterate over `registry.All()` dynamically rather than using a static slice. |

---

## 7. Migration Plan

1. **Phase 1: Define Interface & Registry**
   - Create `internal/provider/provider.go` with the [`Provider`](#41-the-provider-interface-abstraction) interface.
   - Implement the [`Registry`](#42-the-provider-registry-factory-pattern) in `internal/provider/registry.go`.
2. **Phase 2: Wrap Existing Adapters**
   - Implement `OpenAIProvider`, `AnthropicProvider`, and `GeminiProvider` implementing `Provider` by wrapping existing packages in `internal/adapter/*`.
   - Auto-register them in `DefaultRegistry`.
3. **Phase 3: Refactor Consumer Services**
   - Update [`GatewayService`](file:///data/per-pro/llm-gateway/internal/service/gateway.go) and [`ProviderClient`](file:///data/per-pro/llm-gateway/internal/provider/client.go) to inject and consume `Registry`.
   - Replace switches in `handleNonStreamResponse`, `streamResponse`, and `fallback.go` with polymorphic calls.
4. **Phase 4: Verify & Extend**
   - Run unit and integration test suites (`go test ./...`).
   - Add a new test provider (e.g. `mock` or `deepseek`) to verify zero-touch extension.
