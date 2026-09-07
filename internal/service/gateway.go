package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/analytics"
	"llm-gateway/internal/cache"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/permission"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/telemetry"
	"llm-gateway/pkg/sse"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// ErrPermissionDenied is returned when a user or API key lacks IAM permission for a model.
type ErrPermissionDenied struct {
	Message string
}

func (e *ErrPermissionDenied) Error() string {
	return e.Message
}

func IsPermissionDeniedError(err error) bool {
	var ep *ErrPermissionDenied
	return errors.As(err, &ep)
}

// ErrProviderKey is returned when the upstream provider has no API key
// configured in the stateful provider_keys table.
type ErrProviderKey struct {
	Provider string
}

func (e *ErrProviderKey) Error() string {
	return fmt.Sprintf("no API key configured for provider %q — set it via admin API or `llmgw provider set`", e.Provider)
}

type GatewayService struct {
	providerClient       *provider.ProviderClient
	keyStore             *provider.KeyStore
	costCalc             *analytics.CostCalculator
	telemetry            *analytics.TelemetryLogger
	redis                *db.RedisClient
	pg                   *db.PostgresDB
	responseCache        *cache.ResponseCache
	permEngine           *permission.PermissionEngine
	contentCapturePolicy string
	sloMaxTTFTMs         int
	sloMaxTBTMs          int
	agentMaxToolCalls    int
}

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
) *GatewayService {
	return &GatewayService{
		providerClient:       p,
		keyStore:             k,
		costCalc:             c,
		telemetry:            t,
		redis:                redis,
		pg:                   pg,
		responseCache:        rc,
		permEngine:           permEngine,
		contentCapturePolicy: contentCapturePolicy,
		sloMaxTTFTMs:         sloMaxTTFTMs,
		sloMaxTBTMs:          sloMaxTBTMs,
		agentMaxToolCalls:    agentMaxToolCalls,
	}
}

const (
	modelsCacheKey   = "models:list"
	modelsCacheTTL   = time.Hour
	modelsEmptyCache = 5 * time.Minute
)

// ListAvailableModels returns the union of:
//   - models configured in the DB model_pricings table (authoritative; also the
//     only source for Anthropic, which has no public models-list API), and
//   - models advertised live by each provider that has a key configured.
//
// The result is cached in Redis for modelsCacheTTL. A cold start performs the
// live fetches inline; after that the endpoint is served from cache. Providers
// without a key, or whose list API fails, are skipped rather than failing the
// whole request.
func (g *GatewayService) ListAvailableModels(ctx context.Context) ([]model.ProviderModel, error) {
	if g.redis != nil {
		if cached, err := g.redis.GetCachedModels(ctx); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}

	models := make([]model.ProviderModel, 0, 32)
	seen := make(map[string]bool)

	// 1. Admin-configured models from the pricing table.
	if g.pg != nil {
		if dbModels, err := g.pg.GetAllModelsList(); err == nil {
			for _, m := range dbModels {
				if seen[m.ModelName] {
					continue
				}
				seen[m.ModelName] = true
				created := m.UpdatedAt.Unix()
				if created <= 0 {
					created = 1700000000
				}
				models = append(models, model.ProviderModel{
					ID:       m.ModelName,
					Object:   "model",
					Created:  created,
					OwnedBy:  m.Provider,
					Provider: m.Provider,
				})
			}
		}
	}

	// 2. Live model lists from each provider that has a key configured (concurrent fetch).
	providers := []provider.ProviderType{
		provider.ProviderOpenAI,
		provider.ProviderAnthropic,
		provider.ProviderGemini,
	}

	type providerModelsResult struct {
		provider provider.ProviderType
		ids      []string
	}

	resChan := make(chan providerModelsResult, len(providers))
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(pt provider.ProviderType) {
			defer wg.Done()
			key, err := g.keyStore.GetKey(ctx, string(pt))
			if err != nil {
				return // no key configured — nothing to fetch
			}
			ids, err := g.providerClient.ListModels(ctx, pt, key)
			if err != nil {
				return // upstream list API failed — keep the configured set
			}
			resChan <- providerModelsResult{provider: pt, ids: ids}
		}(p)
	}

	wg.Wait()
	close(resChan)

	nowUnix := time.Now().Unix()
	for res := range resChan {
		for _, id := range res.ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			models = append(models, model.ProviderModel{
				ID:       id,
				Object:   "model",
				Created:  nowUnix,
				OwnedBy:  string(res.provider),
				Provider: string(res.provider),
			})
		}
	}

	// Cache what we got. An empty set (no DB rows, no reachable provider) is
	// cached briefly so a transient outage doesn't hammer upstream on every miss.
	ttl := modelsCacheTTL
	if len(models) == 0 {
		ttl = modelsEmptyCache
	}
	if g.redis != nil {
		_ = g.redis.SetCachedModels(ctx, models, ttl)
	}
	return models, nil
}

type ProxyParams struct {
	InboundFormat  string // "openai" or "anthropic"
	RequestBody    []byte
	ApiKey         *model.APIKey
	UpstreamAPIKey string // optional per-request override (x-upstream-api-key header)
	ResponseWriter http.ResponseWriter
}

// ResolvedTarget is the routing decision for one request: which upstream to
// call and with which wire protocol. Provider is the protocol used for the
// encode/decode switches; Name is the real provider name for key lookup, cost
// and telemetry; BaseURL is empty for built-in providers and the custom endpoint
// for OpenAI/Anthropic-compatible providers.
type ResolvedTarget struct {
	Provider provider.ProviderType
	Name     string
	BaseURL  string
}

// resolveTarget decides the upstream provider for a model name.
//
// Resolution order — the gateway NEVER guesses an upstream from the model-name
// prefix:
//  1. Explicit custom-provider prefix ("providername/model") — the client picks
//     a configured OpenAI/Anthropic-compatible provider by name.
//  2. The catalog: a model explicitly registered on the Models page
//     (model_pricings) wins over a row discovered from a provider's /models API
//     (provider_models). This is how a Google model (e.g. gemma-*) reaches the
//     Gemini API without any hardcoded model list.
//  3. Unknown model → ErrUnknownModel, so the caller can surface a clear
//     "model not in catalog" error instead of silently misrouting.
func (g *GatewayService) resolveTarget(ctx context.Context, modelName string) (ResolvedTarget, error) {
	// 1. Explicit custom-provider prefix: "providername/model".
	if seg, _, ok := strings.Cut(modelName, "/"); ok && seg != "" {
		if cfg, err := g.keyStore.GetProviderInfo(ctx, seg); err == nil && cfg.BaseURL != "" {
			return g.targetForProvider(seg, cfg.Protocol, cfg.BaseURL), nil
		}
	}

	// 2. Catalog: registered (model_pricings) then discovered (provider_models).
	if g.costCalc != nil {
		if prov, ok := g.costCalc.LookupProviderByModel(modelName); ok {
			return g.targetForProviderName(ctx, prov)
		}
	}

	// 3. Unknown model — never guess.
	return ResolvedTarget{}, &ErrUnknownModel{Model: modelName}
}

// ErrUnknownModel is returned when a request model is in no provider's catalog
// and the client did not use an explicit "providername/model" prefix. Routing is
// catalog-driven, so this is a configuration gap, not a name-prefix fallback.
type ErrUnknownModel struct {
	Model string
}

func (e *ErrUnknownModel) Error() string {
	return fmt.Sprintf("model %q is not in the catalog — register it on the Models page / `llmgw model add`, refresh the catalog with `llmgw model discover`, or call it as \"providername/model\"", e.Model)
}

// IsUnknownModelError reports whether err indicates a model missing from the catalog.
func IsUnknownModelError(err error) bool {
	var unknownErr *ErrUnknownModel
	return errors.As(err, &unknownErr)
}

// targetForProviderName builds a ResolvedTarget for a provider name that came
// from the model registry: built-ins map to their wire protocol, custom
// compatible providers use their registered base_url + protocol.
func (g *GatewayService) targetForProviderName(ctx context.Context, prov string) (ResolvedTarget, error) {
	switch prov {
	case "openai":
		return ResolvedTarget{Provider: provider.ProviderOpenAI, Name: "openai"}, nil
	case "anthropic":
		return ResolvedTarget{Provider: provider.ProviderAnthropic, Name: "anthropic"}, nil
	case "gemini":
		return ResolvedTarget{Provider: provider.ProviderGemini, Name: "gemini"}, nil
	default:
		// Custom compatible provider: route via its registered endpoint.
		if cfg, err := g.keyStore.GetProviderInfo(ctx, prov); err == nil && cfg.BaseURL != "" {
			return g.targetForProvider(prov, cfg.Protocol, cfg.BaseURL), nil
		}
		// Registered under a name with no key/base_url yet — use the openai
		// wire so the missing-key error surfaces clearly.
		return ResolvedTarget{Provider: provider.ProviderOpenAI, Name: prov}, nil
	}
}

// targetForProvider builds a ResolvedTarget for a custom compatible provider
// given its wire protocol and base URL.
func (g *GatewayService) targetForProvider(name, protocol, baseURL string) ResolvedTarget {
	pt := provider.ProviderOpenAI
	if protocol == "anthropic" {
		pt = provider.ProviderAnthropic
	}
	return ResolvedTarget{Provider: pt, Name: name, BaseURL: baseURL}
}

// DiscoverProvider fetches one provider's /models API and persists the result
// into the provider_models catalog, then refreshes the routing cache. Returns
// the number of models stored.
func (g *GatewayService) DiscoverProvider(ctx context.Context, providerName string) (int, error) {
	pc, err := g.keyStore.GetProviderInfo(ctx, providerName)
	if err != nil {
		return 0, err
	}
	n, err := g.discoverProvider(ctx, *pc)
	if err != nil {
		return 0, err
	}
	if g.costCalc != nil {
		_ = g.costCalc.RefreshCatalog()
	}
	return n, nil
}

// DiscoverAllModels refreshes the persisted catalog for every configured
// provider. Returns the total number of models stored and per-provider errors;
// a provider whose /models API fails is skipped, not fatal.
func (g *GatewayService) DiscoverAllModels(ctx context.Context) (int, map[string]string) {
	providers, err := g.keyStore.ListActiveProviders()
	if err != nil {
		return 0, map[string]string{"": err.Error()}
	}
	total := 0
	errs := make(map[string]string)
	for _, pc := range providers {
		n, derr := g.discoverProvider(ctx, pc)
		if derr != nil {
			errs[pc.Provider] = derr.Error()
			continue
		}
		total += n
	}
	if g.costCalc != nil {
		_ = g.costCalc.RefreshCatalog()
	}
	return total, errs
}

// discoverProvider fetches and persists one provider's model list.
func (g *GatewayService) discoverProvider(ctx context.Context, pc provider.ProviderConfig) (int, error) {
	key, err := g.keyStore.GetKey(ctx, pc.Provider)
	if err != nil {
		return 0, err
	}

	var ids []string
	if pc.BaseURL != "" {
		// Custom OpenAI/Anthropic-compatible provider.
		ids, err = g.providerClient.ListModelsCustom(ctx, provider.ProviderType(pc.Protocol), pc.BaseURL, key)
	} else {
		ids, err = g.providerClient.ListModels(ctx, provider.ProviderType(pc.Provider), key)
	}
	if err != nil {
		return 0, err
	}
	if g.pg == nil {
		return 0, errors.New("database unavailable")
	}
	if err := g.pg.ReplaceProviderModels(pc.Provider, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// StartModelDiscoveryLoop refreshes the catalog on a fixed interval and once at
// startup, so newly configured providers and upstream model additions are picked
// up without hardcoded model lists.
func (g *GatewayService) StartModelDiscoveryLoop(ctx context.Context, interval time.Duration) {
	g.DiscoverAllModels(ctx) // first pass at startup
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.DiscoverAllModels(ctx)
		}
	}
}

func countMessagesToolCalls(messages []canonical.ChatMessage) int {
	count := 0
	for _, msg := range messages {
		count += len(msg.ToolCalls)
		if len(msg.ToolCalls) == 0 && msg.Role == "tool" {
			count++
		}
	}
	return count
}

func (g *GatewayService) HandleRequest(ctx context.Context, params ProxyParams) error {
	startTime := time.Now()

	// 1. Decode Inbound Client Request to Canonical Request
	var canReq *canonical.CanonicalRequest
	var err error

	if params.InboundFormat == "openai" {
		canReq, err = openai.DecodeInboundRequest(params.RequestBody)
	} else if params.InboundFormat == "anthropic" {
		canReq, err = anthropic.DecodeInboundRequest(params.RequestBody)
	} else {
		return fmt.Errorf("unsupported inbound format: %s", params.InboundFormat)
	}

	if err != nil {
		return fmt.Errorf("failed to decode request: %w", err)
	}

	// 1b. IAM & Key Model Permission Check
	if g.permEngine != nil && params.ApiKey != nil {
		// Evaluate per-key allowlist/denylist rules
		allowed, pErr := g.permEngine.CheckKeyModelAccess(ctx, params.ApiKey.ID, canReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by API key policy", canReq.Model),
			}
		}

		// Evaluate user IAM group policies
		allowed, pErr = g.permEngine.CheckModelAccess(ctx, params.ApiKey.UserID, canReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by IAM group policy", canReq.Model),
			}
		}
	}

	// Content-Agnostic Structural Tracing on OTel span
	requestShape := telemetry.InspectPayloadShape(params.InboundFormat, params.RequestBody)

	// Capture prompt event if policy allows
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		telemetry.SetGenAIShapeAttributes(span, requestShape)
		telemetry.RecordPromptEvent(span, g.contentCapturePolicy, canReq.Messages)
	}

	inputToolCalls := countMessagesToolCalls(canReq.Messages)

	// 2. Exact-Match Response Cache Check (non-streaming only, per API key)
	var cacheKey string
	var cacheTTL time.Duration
	cacheEnabled := false
	if !canReq.Stream && g.responseCache != nil && params.ApiKey != nil {
		if ttl, ok := g.responseCache.EffectiveTTL(params.ApiKey); ok {
			cacheKey = cache.CacheKey(canReq)
			cacheTTL = ttl
			cacheEnabled = true
			if cached, hit := g.responseCache.Get(ctx, cacheKey); hit {
				params.ResponseWriter.Header().Set("Content-Type", "application/json")
				params.ResponseWriter.Header().Set("X-Cache", "HIT")
				params.ResponseWriter.WriteHeader(http.StatusOK)
				params.ResponseWriter.Write(cached)

				// Record Telemetry for cache hit
				g.recordTelemetry(params, "cache", canReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusOK, "", startTime)

				// Metrics
				telemetry.RecordCacheMetric(ctx, "hit", canReq.Model)
				durationSec := time.Since(startTime).Seconds()
				telemetry.RecordDurationMetric(ctx, "cache", canReq.Model, "chat", durationSec, http.StatusOK)

				// Set OTel span attributes
				if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
					telemetry.SetGenAIAttributes(span, "cache", canReq.Model, "chat")
					telemetry.SetLLMGWAttributes(span, "cache", true, false, 0)
				}
				return nil
			}
			telemetry.RecordCacheMetric(ctx, "miss", canReq.Model)
		}
	}

	// 3. Resolve target Upstream Provider & Fallback Ladder
	primaryTarget, err := g.resolveTarget(ctx, canReq.Model)
	if err != nil {
		return err
	}

	targets := g.resolveFallbackChain(ctx, primaryTarget)

	// 4. Dispatch with Fallback Ladder
	upResp, usedTarget, usedModel, attempts, err := g.sendWithFallback(ctx, targets, canReq, params.UpstreamAPIKey)
	if err != nil {
		durationSec := time.Since(startTime).Seconds()
		telemetry.RecordDurationMetric(ctx, primaryTarget.Name, canReq.Model, "chat", durationSec, http.StatusServiceUnavailable)
		g.recordTelemetry(params, primaryTarget.Name, canReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusServiceUnavailable, err.Error(), startTime)
		if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
			span.RecordError(err)
		}
		return err
	}
	defer upResp.Body.Close()

	fallbackUsed := len(attempts) > 1
	if fallbackUsed {
		telemetry.RecordFallbackMetric(ctx, primaryTarget.Name, usedTarget.Name, canReq.Model)
	}

	if upResp.StatusCode != http.StatusOK {
		durationSec := time.Since(startTime).Seconds()
		telemetry.RecordDurationMetric(ctx, usedTarget.Name, usedModel, "chat", durationSec, upResp.StatusCode)
		bodyBytes, _ := io.ReadAll(upResp.Body)
		params.ResponseWriter.WriteHeader(upResp.StatusCode)
		params.ResponseWriter.Write(bodyBytes)
		g.recordTelemetry(params, usedTarget.Name, usedModel, 0, 0, 0, 0, analytics.CostResult{}, 0, upResp.StatusCode, string(bodyBytes), startTime)
		if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
			telemetry.SetGenAIAttributes(span, usedTarget.Name, usedModel, "chat")
			telemetry.SetLLMGWAttributes(span, usedTarget.Name, false, fallbackUsed, 0)
		}
		return nil
	}

	// 5. Handle Streaming vs Non-Streaming Response
	if canReq.Stream {
		return g.handleStreamResponse(ctx, params, usedTarget, canReq, upResp, startTime, fallbackUsed, inputToolCalls)
	}

	return g.handleNonStreamResponse(ctx, params, usedTarget, canReq, upResp, startTime, cacheKey, cacheTTL, cacheEnabled, fallbackUsed, inputToolCalls)
}

func (g *GatewayService) handleNonStreamResponse(ctx context.Context, params ProxyParams, target ResolvedTarget, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time, cacheKey string, cacheTTL time.Duration, cacheEnabled, fallbackUsed bool, inputToolCalls int) error {
	bodyBytes, err := io.ReadAll(upResp.Body)
	if err != nil {
		return err
	}

	// Decode upstream response to Canonical
	var canResp *canonical.CanonicalResponse
	switch target.Provider {
	case provider.ProviderOpenAI:
		canResp, err = openai.DecodeUpstreamResponse(bodyBytes)
	case provider.ProviderAnthropic:
		canResp, err = anthropic.DecodeUpstreamResponse(bodyBytes)
	case provider.ProviderGemini:
		canResp, err = gemini.DecodeUpstreamResponse(bodyBytes, canReq.Model)
	}

	if err != nil {
		params.ResponseWriter.WriteHeader(http.StatusInternalServerError)
		params.ResponseWriter.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
		return err
	}

	// Format Canonical to Client Inbound Format
	var clientResponseBytes []byte
	if params.InboundFormat == "openai" {
		clientResponseBytes, err = openai.FormatCanonicalToOpenAIResponse(canResp)
	} else if params.InboundFormat == "anthropic" {
		clientResponseBytes, err = anthropic.FormatCanonicalToAnthropicResponse(canResp)
	}

	if err != nil {
		return err
	}

	// Send Response to Client
	params.ResponseWriter.Header().Set("Content-Type", "application/json")
	if cacheEnabled {
		params.ResponseWriter.Header().Set("X-Cache", "MISS")
	}
	params.ResponseWriter.WriteHeader(http.StatusOK)
	params.ResponseWriter.Write(clientResponseBytes)

	// Cache the response if caching is enabled
	if cacheEnabled && g.responseCache != nil && len(clientResponseBytes) > 0 {
		g.responseCache.Set(ctx, cacheKey, clientResponseBytes, cacheTTL)
	}

	var keyID, userID string
	if params.ApiKey != nil {
		keyID = params.ApiKey.ID.String()
		userID = params.ApiKey.UserID.String()
	}

	// Record Telemetry
	costResult := g.costCalc.CalculateCost(target.Name, canReq.Model, canResp.PromptTokens, canResp.CompletionTokens, canResp.CachedInputTokens, canResp.ReasoningTokens)
	g.recordTelemetry(params, target.Name, canReq.Model, canResp.PromptTokens, canResp.CompletionTokens, canResp.CachedInputTokens, canResp.ReasoningTokens, costResult, 0, http.StatusOK, "", startTime)

	// Set OTel attributes and completion event
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		telemetry.SetGenAIAttributes(span, target.Name, canReq.Model, "chat")
		telemetry.SetGenAIUsage(span, canResp.PromptTokens, canResp.CompletionTokens)
		telemetry.SetLLMGWAttributes(span, target.Name, false, fallbackUsed, 0)
		telemetry.RecordCompletionEvent(span, g.contentCapturePolicy, canResp.Content)
	}

	// Agent runaway loop detection
	totalToolCalls := inputToolCalls + len(canResp.ToolCalls)
	if g.agentMaxToolCalls > 0 && totalToolCalls > g.agentMaxToolCalls {
		telemetry.RecordRunawayLoop(ctx, target.Name, canReq.Model, keyID, totalToolCalls)
		if span != nil && span.IsRecording() {
			telemetry.RecordRunawayLoopEvent(span, totalToolCalls, g.agentMaxToolCalls)
		}
	}

	// Metrics recording
	durationSec := time.Since(startTime).Seconds()
	telemetry.RecordDurationMetric(ctx, target.Name, canReq.Model, "chat", durationSec, http.StatusOK)
	telemetry.RecordTokenMetrics(ctx, target.Name, canReq.Model, keyID, userID, canResp.PromptTokens, canResp.CompletionTokens, canResp.CachedInputTokens)
	telemetry.RecordCostMetric(ctx, target.Name, canReq.Model, keyID, userID, costResult.Cost)

	return nil
}

func (g *GatewayService) handleStreamResponse(ctx context.Context, params ProxyParams, target ResolvedTarget, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time, fallbackUsed bool, inputToolCalls int) error {
	flusher, ok := params.ResponseWriter.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	params.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	params.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	params.ResponseWriter.Header().Set("Connection", "keep-alive")
	params.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
	params.ResponseWriter.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Anthropic clients require the full message lifecycle (message_start →
	// content_block_start/delta/stop → message_delta → message_stop). That is
	// reconstructed lazily from the canonical deltas below so thinking blocks
	// (which precede the text block) are surfaced correctly and message_start
	// can carry the real input token counts gathered from the upstream.
	var anthroMsgID = "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
	anthroMessageStarted := false
	anthroOpenBlock := "" // "" | "thinking" | "text"
	anthroBlockIndex := -1

	sseReader := sse.NewReader(upResp.Body)
	var ttftMs int64 = 0
	var totalPromptTokens, totalCompletionTokens int
	var totalCachedInputTokens, totalReasoningTokens int
	var lastFinishReason string
	isFirstChunk := true
	var streamContent strings.Builder

	// emitAnthropicMessageStart lazily writes the message_start opener once,
	// using the real input/cache counts accumulated so far from the upstream.
	emitAnthropicMessageStart := func() {
		if anthroMessageStarted {
			return
		}
		anthroMessageStarted = true
		usage := map[string]interface{}{"input_tokens": totalPromptTokens, "output_tokens": 1}
		if totalCachedInputTokens > 0 {
			usage["cache_read_input_tokens"] = totalCachedInputTokens
		}
		msg := map[string]interface{}{
			"type": "message", "id": anthroMsgID, "role": "assistant", "model": canReq.Model,
			"content": []interface{}{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage,
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "message_start", "message": msg}); err == nil {
			params.ResponseWriter.Write([]byte("event: message_start\ndata: " + string(data) + "\n\n"))
		}
	}

	// startAnthropicBlock opens a content block, closing any currently-open
	// block first so the client sees a strict thinking → text ordering.
	startAnthropicBlock := func(blockType string) {
		if anthroOpenBlock == blockType {
			return
		}
		if anthroOpenBlock != "" {
			if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
				params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
			}
		}
		anthroBlockIndex++
		block := map[string]interface{}{"type": blockType}
		if blockType == "thinking" {
			block["thinking"] = ""
		} else {
			block["text"] = ""
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_start", "index": anthroBlockIndex, "content_block": block}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_start\ndata: " + string(data) + "\n\n"))
		}
		anthroOpenBlock = blockType
	}

	// Tool-call fragments accumulate over the stream (OpenAI delta.tool_calls,
	// Anthropic content_block_start/input_json_delta, Gemini functionCall parts)
	// and are reconstructed as complete calls at stream end. They cannot be
	// streamed incrementally because fragments arrive split across chunks; the
	// client receives the full calls in one terminal block.
	type pendingToolCall struct {
		key  string
		id   string
		name string
		args strings.Builder
	}
	acc := map[string]*pendingToolCall{}
	order := []string{} // insertion order → stable call ordering

	// emitAnthropicToolBlock writes one reconstructed tool_use block (start →
	// input_json_delta → stop) for an Anthropic-bound client, mirroring how
	// Anthropic streams a complete tool_use.
	emitAnthropicToolBlock := func(tc canonical.ToolCall) {
		anthroBlockIndex++
		block := map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": map[string]interface{}{},
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_start", "index": anthroBlockIndex, "content_block": block}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_start\ndata: " + string(data) + "\n\n"))
		}
		deltaPayload := map[string]interface{}{
			"type":  "content_block_delta",
			"index": anthroBlockIndex,
			"delta": map[string]string{"type": "input_json_delta", "partial_json": tc.Arguments},
		}
		if data, err := json.Marshal(deltaPayload); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_delta\ndata: " + string(data) + "\n\n"))
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
		}
		anthroOpenBlock = ""
	}

	// The upstream body read (sseReader.ReadEvent) blocks until a full SSE event
	// arrives; reasoning models can sit silent for tens of seconds, so a plain
	// blocking loop would leave the downstream connection idle long enough for a
	// proxy/load-balancer to reap it mid-thought. Drive the read from a goroutine
	// so the relay loop stays selectable: it emits flushed keep-alive comment
	// lines during those gaps and aborts promptly when the client disconnects
	// instead of draining (and paying for) the rest of the upstream stream. The
	// channel buffer caps the read-ahead at one event.
	eventC := make(chan *sse.Event, 1)
	readDoneC := make(chan error, 1)
	go func() {
		for {
			ev, rErr := sseReader.ReadEvent()
			if rErr != nil {
				readDoneC <- rErr
				return
			}
			select {
			case eventC <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Comment frames are ignored by SSE clients but reset idle timers on
	// intermediaries. 15s stays well under the common 60s LB/proxy idle timeout
	// (nginx, AWS ALB), so an upstream that is thinking — not dead — is not
	// mistaken for a hung connection.
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	// streamErr records an abnormal end (mid-stream read failure or client
	// cancellation); the terminal block below then emits an explicit error marker
	// instead of a clean [DONE]/message_stop.
	var streamErr error

relayLoop:
	for {
		var event *sse.Event
		var err error
		select {
		case event = <-eventC:
			// a full SSE event from the upstream to relay
		case err = <-readDoneC:
			if err == io.EOF {
				break relayLoop
			}
			streamErr = err
			break relayLoop
		case <-keepAlive.C:
			params.ResponseWriter.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
			continue relayLoop
		case <-ctx.Done():
			streamErr = ctx.Err()
			break relayLoop
		}

		if isFirstChunk {
			ttftMs = time.Since(startTime).Milliseconds()
			isFirstChunk = false
		}

		// Convert SSE event to Canonical Stream Chunk
		var chunk *canonical.CanonicalStreamChunk

		switch target.Provider {
		case provider.ProviderOpenAI:
			if string(event.Data) == "[DONE]" {
				chunk = &canonical.CanonicalStreamChunk{IsLast: true}
			} else {
				c, cErr := openai.DecodeStreamChunk(event.Data, canReq.Model)
				if cErr == nil && c != nil {
					chunk = c
					if c.PromptTokens > totalPromptTokens {
						totalPromptTokens = c.PromptTokens
					}
					if c.CompletionTokens > totalCompletionTokens {
						totalCompletionTokens = c.CompletionTokens
					}
					if c.CachedInputTokens > totalCachedInputTokens {
						totalCachedInputTokens = c.CachedInputTokens
					}
					if c.ReasoningTokens > totalReasoningTokens {
						totalReasoningTokens = c.ReasoningTokens
					}
				}
			}

		case provider.ProviderAnthropic:
			c, cErr := anthropic.DecodeStreamChunk(event.Event, event.Data, canReq.Model)
			if cErr == nil && c != nil {
				chunk = c
				if c.PromptTokens > 0 {
					totalPromptTokens = c.PromptTokens
				}
				if c.CompletionTokens > 0 {
					totalCompletionTokens = c.CompletionTokens
				}
				if c.CachedInputTokens > 0 {
					totalCachedInputTokens = c.CachedInputTokens
				}
				if c.ReasoningTokens > 0 {
					totalReasoningTokens = c.ReasoningTokens
				}
			}

		case provider.ProviderGemini:
			c, cErr := gemini.DecodeStreamChunk(event.Data, canReq.Model)
			if cErr == nil && c != nil {
				chunk = c
				if c.CompletionTokens > 0 {
					totalCompletionTokens = c.CompletionTokens
					totalPromptTokens = c.PromptTokens
				}
				if c.CachedInputTokens > 0 {
					totalCachedInputTokens = c.CachedInputTokens
				}
				if c.ReasoningTokens > 0 {
					totalReasoningTokens = c.ReasoningTokens
				}
			}
		}

		if chunk == nil {
			continue
		}
		if chunk.FinishReason != "" {
			lastFinishReason = chunk.FinishReason
		}
		if chunk.DeltaContent != "" && strings.EqualFold(g.contentCapturePolicy, "event_only") {
			streamContent.WriteString(chunk.DeltaContent)
		}

		// Feed tool-call fragments into the accumulator (never streamed
		// incrementally). OpenAI/Anthropic key by the stable chunk index; Gemini
		// parts are atomic with no stable cross-chunk index, so key by name.
		for _, dtc := range chunk.DeltaToolCalls {
			key := strconv.Itoa(dtc.Index)
			if target.Provider == provider.ProviderGemini {
				key = "g:" + dtc.Name
			}
			p, ok := acc[key]
			if !ok {
				p = &pendingToolCall{key: key}
				acc[key] = p
				order = append(order, key)
			}
			if p.id == "" {
				p.id = dtc.ID
			}
			if p.name == "" {
				p.name = dtc.Name
			}
			if dtc.Arguments != "" {
				p.args.WriteString(dtc.Arguments)
			}
		}

		if params.InboundFormat == "openai" {
			// Skip the bare terminal marker; the [DONE] line is written below.
			if chunk.IsLast && chunk.DeltaContent == "" && chunk.FinishReason == "" {
				continue
			}
			// Pure tool-call fragments carry no text/reasoning to emit — they are
			// reconstructed at stream end.
			if len(chunk.DeltaToolCalls) > 0 && chunk.DeltaContent == "" && chunk.DeltaReasoning == "" {
				continue
			}
			// The upstream's own finish_reason:"tool_calls" chunk is replaced by
			// the reconstructed terminal block (which carries its own finish).
			if chunk.FinishReason == "tool_calls" && len(acc) > 0 {
				continue
			}
			sseFormatted, fErr := openai.FormatCanonicalToOpenAISSE(chunk)
			if fErr == nil && sseFormatted != "" {
				params.ResponseWriter.Write([]byte(sseFormatted))
				flusher.Flush()
			}
		} else {
			// Anthropic inbound: rebuild the block lifecycle from canonical
			// deltas. Thinking deltas open the thinking block (index 0) before
			// the text block (index 1); message_start is emitted lazily ahead
			// of the first content event.
			if chunk.DeltaReasoning != "" {
				emitAnthropicMessageStart()
				startAnthropicBlock("thinking")
				sseFormatted, fErr := anthropic.FormatCanonicalToAnthropicSSE(chunk, anthroBlockIndex, "thinking")
				if fErr == nil && sseFormatted != "" {
					params.ResponseWriter.Write([]byte(sseFormatted))
					flusher.Flush()
				}
			}
			if chunk.DeltaContent != "" {
				emitAnthropicMessageStart()
				startAnthropicBlock("text")
				sseFormatted, fErr := anthropic.FormatCanonicalToAnthropicSSE(chunk, anthroBlockIndex, "text")
				if fErr == nil && sseFormatted != "" {
					params.ResponseWriter.Write([]byte(sseFormatted))
					flusher.Flush()
				}
			}
		}
	}

	// Reconstruct complete tool calls from the accumulated fragments, preserving
	// insertion order.
	var toolCalls []canonical.ToolCall
	for _, key := range order {
		p := acc[key]
		if p.name == "" {
			continue
		}
		toolCalls = append(toolCalls, canonical.ToolCall{
			ID:        p.id,
			Name:      p.name,
			Arguments: p.args.String(),
		})
	}
	hasToolCalls := len(toolCalls) > 0

	// Final stream completion signal. A mid-stream failure must not be masked as
	// a clean end: emit an explicit error event instead of [DONE]/message_stop so
	// clients surface the interruption rather than treating the response as
	// complete (and, for Anthropic, so they do not hang waiting for message_stop).
	if streamErr != nil {
		if params.InboundFormat == "openai" {
			// OpenAI SSE has no spec'd terminal error event; the de-facto
			// convention (also used by OpenAI itself) is a final data frame
			// carrying an "error" object, which official SDKs translate into a
			// raised error. No [DONE] is sent, so truncation is unambiguous.
			errObj, _ := json.Marshal(map[string]interface{}{
				"error": map[string]interface{}{
					"message": truncateStreamError(streamErr.Error()),
					"type":    "upstream_stream_error",
				},
			})
			params.ResponseWriter.Write([]byte("data: " + string(errObj) + "\n\n"))
		} else {
			// Anthropic streams a first-class error event. Close any open content
			// block first so the lifecycle stays balanced for strict clients.
			if anthroOpenBlock != "" {
				if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
					params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
				}
				anthroOpenBlock = ""
			}
			errObj, _ := json.Marshal(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": truncateStreamError(streamErr.Error()),
				},
			})
			params.ResponseWriter.Write([]byte("event: error\ndata: " + string(errObj) + "\n\n"))
		}
		flusher.Flush()
	} else if params.InboundFormat == "openai" {
		if hasToolCalls {
			// Emit the reconstructed tool calls, then the standard [DONE] marker.
			if sse, fErr := openai.FormatOpenAIToolCallSSE(toolCalls, canReq.Model, anthroMsgID, time.Now().Unix()); fErr == nil && sse != "" {
				params.ResponseWriter.Write([]byte(sse))
			}
		}
		params.ResponseWriter.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	} else {
		// Ensure message_start is on the wire even if the upstream produced no
		// content deltas (e.g. it errored mid-stream), then close any still-open
		// block before ending the message lifecycle.
		emitAnthropicMessageStart()
		if anthroOpenBlock != "" {
			if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
				params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
			}
			anthroOpenBlock = ""
		}
		// Map the upstream finish reason to Anthropic's stop_reason vocabulary.
		// Gemini reports raw "STOP"/"MAX_TOKENS"; Anthropic expects
		// end_turn/max_tokens/stop_sequence/tool_use.
		stopReason := lastFinishReason
		switch stopReason {
		case "STOP":
			stopReason = "end_turn"
		case "MAX_TOKENS":
			stopReason = "max_tokens"
		}
		if stopReason == "" {
			stopReason = "end_turn"
		}
		if hasToolCalls {
			// Reconstructed tool_use blocks, then stop_reason:"tool_use" (the
			// Anthropic analogue of OpenAI's finish_reason:"tool_calls").
			for _, tc := range toolCalls {
				emitAnthropicToolBlock(tc)
			}
			stopReason = "tool_use"
		}
		messageDelta := fmt.Sprintf(
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":%q,\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n",
			stopReason, totalCompletionTokens,
		)
		messageStop := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		params.ResponseWriter.Write([]byte(messageDelta))
		params.ResponseWriter.Write([]byte(messageStop))
		flusher.Flush()
	}

	var keyID, userID string
	if params.ApiKey != nil {
		keyID = params.ApiKey.ID.String()
		userID = params.ApiKey.UserID.String()
	}

	totalDurationMs := time.Since(startTime).Milliseconds()
	totalDurationSec := float64(totalDurationMs) / 1000.0
	ttftSec := float64(ttftMs) / 1000.0

	var tbtMs float64
	var tbtSec float64
	if totalCompletionTokens > 1 {
		tbtMs = float64(totalDurationMs-ttftMs) / float64(totalCompletionTokens-1)
		tbtSec = tbtMs / 1000.0
	}

	isGoodput := true
	var violationType string
	if g.sloMaxTTFTMs > 0 && int(ttftMs) > g.sloMaxTTFTMs {
		isGoodput = false
		violationType = "ttft"
	}
	if totalCompletionTokens > 1 && g.sloMaxTBTMs > 0 && int(tbtMs) > g.sloMaxTBTMs {
		isGoodput = false
		if violationType == "" {
			violationType = "tbt"
		} else {
			violationType = "both"
		}
	}

	costResult := g.costCalc.CalculateCost(target.Name, canReq.Model, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens, totalReasoningTokens)

	// Record mid-stream truncation in the usage log so it is distinguishable
	// from a healthy completion.
	errMsg := ""
	if streamErr != nil {
		errMsg = streamErr.Error()
	}
	g.recordTelemetry(params, target.Name, canReq.Model, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens, totalReasoningTokens, costResult, ttftMs, http.StatusOK, errMsg, startTime)

	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		telemetry.SetGenAIAttributes(span, target.Name, canReq.Model, "chat")
		telemetry.SetGenAIUsage(span, totalPromptTokens, totalCompletionTokens)
		telemetry.SetLLMGWAttributes(span, target.Name, false, fallbackUsed, ttftMs)
		if streamContent.Len() > 0 {
			telemetry.RecordCompletionEvent(span, g.contentCapturePolicy, streamContent.String())
		}
	}

	// Agent runaway loop detection
	totalToolCalls := inputToolCalls + len(toolCalls)
	if g.agentMaxToolCalls > 0 && totalToolCalls > g.agentMaxToolCalls {
		telemetry.RecordRunawayLoop(ctx, target.Name, canReq.Model, keyID, totalToolCalls)
		if span != nil && span.IsRecording() {
			telemetry.RecordRunawayLoopEvent(span, totalToolCalls, g.agentMaxToolCalls)
		}
	}

	// Metrics recording
	telemetry.RecordStreamSLOMetrics(ctx, target.Name, canReq.Model, keyID, ttftSec, tbtSec, isGoodput, violationType)
	telemetry.RecordDurationMetric(ctx, target.Name, canReq.Model, "chat", totalDurationSec, http.StatusOK)
	telemetry.RecordTokenMetrics(ctx, target.Name, canReq.Model, keyID, userID, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens)
	telemetry.RecordCostMetric(ctx, target.Name, canReq.Model, keyID, userID, costResult.Cost)

	return nil
}

// truncateStreamError bounds an upstream error string before it is echoed into a
// terminal SSE error frame, so a huge raw error body cannot bloat the frame.
func truncateStreamError(s string) string {
	const maxLen = 400
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func (g *GatewayService) recordTelemetry(params ProxyParams, provider, modelName string, promptTokens, completionTokens, cachedInputTokens, reasoningTokens int, costResult analytics.CostResult, ttftMs int64, statusCode int, errMsg string, startTime time.Time) {
	if g.telemetry == nil || params.ApiKey == nil {
		return
	}

	totalLatency := time.Since(startTime).Milliseconds()
	shape := telemetry.InspectPayloadShape(params.InboundFormat, params.RequestBody)

	logEntry := model.UsageLog{
		CreatedAt:           time.Now(),
		UserID:              params.ApiKey.UserID,
		KeyID:               params.ApiKey.ID,
		InboundFormat:       params.InboundFormat,
		Provider:            provider,
		Model:               modelName,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		TotalTokens:         promptTokens + completionTokens,
		CachedInputTokens:   cachedInputTokens,
		ReasoningTokens:     reasoningTokens,
		PromptPriceUsed:     costResult.PromptPriceUsed,
		CompletionPriceUsed: costResult.CompletionPriceUsed,
		PricingSource:       costResult.PricingSource,
		PricingFallback:     costResult.PricingFallback,
		Cost:                costResult.Cost,
		TTFTMs:              ttftMs,
		TotalLatencyMs:      totalLatency,
		StatusCode:          statusCode,
		ErrorMessage:        errMsg,
		RequestShape:        shape,
	}

	g.telemetry.Log(logEntry)
}

// IsProviderKeyError reports whether err indicates a missing provider credential.
func IsProviderKeyError(err error) bool {
	var pkeyErr *ErrProviderKey
	return errors.As(err, &pkeyErr)
}

func (g *GatewayService) HandleEmbeddings(ctx context.Context, params ProxyParams) error {
	startTime := time.Now()

	// 1. Parse client payload in OpenAI format
	var embedReq struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}
	if err := json.Unmarshal(params.RequestBody, &embedReq); err != nil {
		return fmt.Errorf("invalid embeddings payload: %w", err)
	}

	// 1b. IAM & Key Model Permission Check
	if g.permEngine != nil && params.ApiKey != nil {
		allowed, pErr := g.permEngine.CheckKeyModelAccess(ctx, params.ApiKey.ID, embedReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by API key policy", embedReq.Model),
			}
		}

		allowed, pErr = g.permEngine.CheckModelAccess(ctx, params.ApiKey.UserID, embedReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by IAM group policy", embedReq.Model),
			}
		}
	}

	// 2. Resolve target provider
	target, err := g.resolveTarget(ctx, embedReq.Model)
	if err != nil {
		return err
	}

	// 3. Resolve upstream API key
	upstreamKey := params.UpstreamAPIKey
	if upstreamKey == "" {
		upstreamKey, err = g.keyStore.GetKey(ctx, target.Name)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusServiceUnavailable, err.Error(), startTime)
			return &ErrProviderKey{Provider: target.Name}
		}
	}

	// 4. Extract inputs as string slice
	var inputs []string
	switch val := embedReq.Input.(type) {
	case string:
		inputs = []string{val}
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				inputs = append(inputs, s)
			}
		}
	}

	// Approximate input tokens
	promptTokens := 0
	for _, s := range inputs {
		tokens := len(s) / 4
		if tokens == 0 && len(s) > 0 {
			tokens = 1
		}
		promptTokens += tokens
	}

	// 5. Execute based on Provider protocol
	if target.Provider == provider.ProviderGemini {
		// Translation OpenAI -> Gemini -> OpenAI
		var url string
		var payloadBytes []byte

		if len(inputs) <= 1 {
			// Single embedding
			textVal := ""
			if len(inputs) == 1 {
				textVal = inputs[0]
			}
			url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s", embedReq.Model, upstreamKey)
			
			geminiReq := map[string]interface{}{
				"content": map[string]interface{}{
					"parts": []map[string]interface{}{
						{"text": textVal},
					},
				},
			}
			payloadBytes, _ = json.Marshal(geminiReq)
		} else {
			// Batch embeddings
			url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s", embedReq.Model, upstreamKey)
			
			modelPath := embedReq.Model
			if !strings.HasPrefix(modelPath, "models/") {
				modelPath = "models/" + modelPath
			}

			requestsList := make([]interface{}, 0, len(inputs))
			for _, text := range inputs {
				requestsList = append(requestsList, map[string]interface{}{
					"model": modelPath,
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": text},
						},
					},
				})
			}
			geminiReq := map[string]interface{}{
				"requests": requestsList,
			}
			payloadBytes, _ = json.Marshal(geminiReq)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := g.providerClient.Do(httpReq)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusInternalServerError, err.Error(), startTime)
			return fmt.Errorf("failed sending request to Gemini: %w", err)
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			params.ResponseWriter.Header().Set("Content-Type", "application/json")
			params.ResponseWriter.WriteHeader(resp.StatusCode)
			params.ResponseWriter.Write(respBytes)
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, resp.StatusCode, string(respBytes), startTime)
			return nil
		}

		// Decode Gemini response and map back to OpenAI
		type GeminiEmbedding struct {
			Values []float64 `json:"values"`
		}

		var openAIRespData map[string]interface{}
		if len(inputs) <= 1 {
			var geminiResp struct {
				Embedding GeminiEmbedding `json:"embedding"`
			}
			if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
				return fmt.Errorf("failed to decode Gemini response: %w", err)
			}
			openAIRespData = map[string]interface{}{
				"object": "list",
				"data": []interface{}{
					map[string]interface{}{
						"object":    "embedding",
						"index":     0,
						"embedding": geminiResp.Embedding.Values,
					},
				},
				"model": embedReq.Model,
				"usage": map[string]interface{}{
					"prompt_tokens": promptTokens,
					"total_tokens":  promptTokens,
				},
			}
		} else {
			var geminiResp struct {
				Embeddings []GeminiEmbedding `json:"embeddings"`
			}
			if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
				return fmt.Errorf("failed to decode Gemini response: %w", err)
			}
			dataList := make([]interface{}, 0, len(geminiResp.Embeddings))
			for i, emb := range geminiResp.Embeddings {
				dataList = append(dataList, map[string]interface{}{
					"object":    "embedding",
					"index":     i,
					"embedding": emb.Values,
				})
			}
			openAIRespData = map[string]interface{}{
				"object": "list",
				"data":   dataList,
				"model":  embedReq.Model,
				"usage": map[string]interface{}{
					"prompt_tokens": promptTokens,
					"total_tokens":  promptTokens,
				},
			}
		}

		openAIRespBytes, _ := json.Marshal(openAIRespData)
		params.ResponseWriter.Header().Set("Content-Type", "application/json")
		params.ResponseWriter.WriteHeader(http.StatusOK)
		params.ResponseWriter.Write(openAIRespBytes)

		costResult := g.costCalc.CalculateCost(target.Name, embedReq.Model, promptTokens, 0, 0, 0)
		g.recordTelemetry(params, target.Name, embedReq.Model, promptTokens, 0, 0, 0, costResult, 0, http.StatusOK, "", startTime)
		if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
			telemetry.SetGenAIAttributes(span, target.Name, embedReq.Model, "embeddings")
			telemetry.SetGenAIUsage(span, promptTokens, 0)
			telemetry.SetLLMGWAttributes(span, target.Name, false, false, 0)
		}

		var keyID, userID string
		if params.ApiKey != nil {
			keyID = params.ApiKey.ID.String()
			userID = params.ApiKey.UserID.String()
		}
		telemetry.RecordDurationMetric(ctx, target.Name, embedReq.Model, "embeddings", time.Since(startTime).Seconds(), http.StatusOK)
		telemetry.RecordTokenMetrics(ctx, target.Name, embedReq.Model, keyID, userID, promptTokens, 0, 0)
		telemetry.RecordCostMetric(ctx, target.Name, embedReq.Model, keyID, userID, costResult.Cost)

	} else {
		// For OpenAI & OpenAI-compatible: transparent proxying
		var url string
		headers := map[string]string{
			"Content-Type": "application/json",
		}

		if target.BaseURL != "" {
			base := strings.TrimRight(target.BaseURL, "/")
			if strings.HasSuffix(base, "/chat/completions") {
				base = strings.TrimSuffix(base, "/chat/completions")
			} else if strings.HasSuffix(base, "/v1/chat/completions") {
				base = strings.TrimSuffix(base, "/v1/chat/completions")
			} else if strings.HasSuffix(base, "/v1") {
				// keep it
			}
			url = base + "/embeddings"
			if target.Provider == provider.ProviderOpenAI {
				headers["Authorization"] = "Bearer " + upstreamKey
			} else if target.Provider == provider.ProviderAnthropic {
				headers["x-api-key"] = upstreamKey
				headers["anthropic-version"] = "2023-06-01"
			}
		} else {
			// Built-in OpenAI
			url = "https://api.openai.com/v1/embeddings"
			headers["Authorization"] = "Bearer " + upstreamKey
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(params.RequestBody))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}

		resp, err := g.providerClient.Do(httpReq)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusInternalServerError, err.Error(), startTime)
			return fmt.Errorf("failed sending request to upstream: %w", err)
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		params.ResponseWriter.Header().Set("Content-Type", "application/json")
		params.ResponseWriter.WriteHeader(resp.StatusCode)
		params.ResponseWriter.Write(bodyBytes)

		var keyID, userID string
		if params.ApiKey != nil {
			keyID = params.ApiKey.ID.String()
			userID = params.ApiKey.UserID.String()
		}
		telemetry.RecordDurationMetric(ctx, target.Name, embedReq.Model, "embeddings", time.Since(startTime).Seconds(), resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			var respData struct {
				Usage struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			}
			_ = json.Unmarshal(bodyBytes, &respData)

			tokens := respData.Usage.PromptTokens
			if tokens <= 0 {
				tokens = promptTokens
			}
			costResult := g.costCalc.CalculateCost(target.Name, embedReq.Model, tokens, 0, 0, 0)
			g.recordTelemetry(params, target.Name, embedReq.Model, tokens, 0, 0, 0, costResult, 0, resp.StatusCode, "", startTime)
			if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
				telemetry.SetGenAIAttributes(span, target.Name, embedReq.Model, "embeddings")
				telemetry.SetGenAIUsage(span, tokens, 0)
				telemetry.SetLLMGWAttributes(span, target.Name, false, false, 0)
			}
			telemetry.RecordTokenMetrics(ctx, target.Name, embedReq.Model, keyID, userID, tokens, 0, 0)
			telemetry.RecordCostMetric(ctx, target.Name, embedReq.Model, keyID, userID, costResult.Cost)
		} else {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, resp.StatusCode, string(bodyBytes), startTime)
		}
	}

	return nil
}

