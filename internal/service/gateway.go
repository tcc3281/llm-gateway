package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/analytics"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/provider"
	"llm-gateway/pkg/sse"

	"github.com/google/uuid"
)

// ErrProviderKey is returned when the upstream provider has no API key
// configured in the stateful provider_keys table.
type ErrProviderKey struct {
	Provider string
}

func (e *ErrProviderKey) Error() string {
	return fmt.Sprintf("no API key configured for provider %q — set it via admin API or `llmgw provider set`", e.Provider)
}

type GatewayService struct {
	providerClient *provider.ProviderClient
	keyStore       *provider.KeyStore
	costCalc       *analytics.CostCalculator
	telemetry      *analytics.TelemetryLogger
	redis          *db.RedisClient
	pg             *db.PostgresDB
}

func NewGatewayService(p *provider.ProviderClient, k *provider.KeyStore, c *analytics.CostCalculator, t *analytics.TelemetryLogger, redis *db.RedisClient, pg *db.PostgresDB) *GatewayService {
	return &GatewayService{
		providerClient: p,
		keyStore:       k,
		costCalc:       c,
		telemetry:      t,
		redis:          redis,
		pg:             pg,
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

	// 2. Live model lists from each provider that has a key configured.
	for _, p := range []provider.ProviderType{
		provider.ProviderOpenAI,
		provider.ProviderAnthropic,
		provider.ProviderGemini,
	} {
		key, err := g.keyStore.GetKey(ctx, string(p))
		if err != nil {
			continue // no key configured — nothing to fetch
		}
		ids, err := g.providerClient.ListModels(ctx, p, key)
		if err != nil {
			continue // upstream list API failed — keep the configured set
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			models = append(models, model.ProviderModel{
				ID:       id,
				Object:   "model",
				Created:  time.Now().Unix(),
				OwnedBy:  string(p),
				Provider: string(p),
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

	// 2. Resolve target Upstream Provider
	targetProvider := provider.ResolveProvider(canReq.Model)

	// 3. Encode Canonical Request to Upstream Format Payload
	var upstreamPayload []byte
	switch targetProvider {
	case provider.ProviderOpenAI:
		upstreamPayload, err = openai.EncodeCanonicalRequest(canReq)
	case provider.ProviderAnthropic:
		upstreamPayload, err = anthropic.EncodeCanonicalRequest(canReq)
	case provider.ProviderGemini:
		upstreamPayload, err = gemini.EncodeCanonicalRequest(canReq)
	}

	if err != nil {
		return fmt.Errorf("failed to encode payload for provider %s: %w", targetProvider, err)
	}

	// 4. Resolve upstream credentials: per-request override > stateful DB key
	upstreamKey := params.UpstreamAPIKey
	if upstreamKey == "" {
		upstreamKey, err = g.keyStore.GetKey(ctx, string(targetProvider))
		if err != nil {
			g.recordTelemetry(params, string(targetProvider), canReq.Model, 0, 0, 0, 0, http.StatusServiceUnavailable, err.Error(), startTime)
			return &ErrProviderKey{Provider: string(targetProvider)}
		}
	}

	// 5. Dispatch Upstream Request
	upReq := provider.UpstreamRequest{
		Provider: targetProvider,
		Model:    canReq.Model,
		Stream:   canReq.Stream,
		Payload:  upstreamPayload,
	}

	upResp, err := g.providerClient.SendRequest(ctx, upReq, upstreamKey)
	if err != nil {
		g.recordTelemetry(params, string(targetProvider), canReq.Model, 0, 0, 0, 0, http.StatusInternalServerError, err.Error(), startTime)
		return err
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(upResp.Body)
		params.ResponseWriter.WriteHeader(upResp.StatusCode)
		params.ResponseWriter.Write(bodyBytes)
		g.recordTelemetry(params, string(targetProvider), canReq.Model, 0, 0, 0, 0, upResp.StatusCode, string(bodyBytes), startTime)
		return nil
	}

	// 6. Handle Streaming vs Non-Streaming Response
	if canReq.Stream {
		return g.handleStreamResponse(ctx, params, targetProvider, canReq, upResp, startTime)
	}

	return g.handleNonStreamResponse(params, targetProvider, canReq, upResp, startTime)
}

func (g *GatewayService) handleNonStreamResponse(params ProxyParams, targetProvider provider.ProviderType, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time) error {
	bodyBytes, err := io.ReadAll(upResp.Body)
	if err != nil {
		return err
	}

	// Decode upstream response to Canonical
	var canResp *canonical.CanonicalResponse
	switch targetProvider {
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
	params.ResponseWriter.WriteHeader(http.StatusOK)
	params.ResponseWriter.Write(clientResponseBytes)

	// Record Telemetry
	cost := g.costCalc.CalculateCost(string(targetProvider), canReq.Model, canResp.PromptTokens, canResp.CompletionTokens)
	g.recordTelemetry(params, string(targetProvider), canReq.Model, canResp.PromptTokens, canResp.CompletionTokens, cost, 0, http.StatusOK, "", startTime)

	return nil
}

func (g *GatewayService) handleStreamResponse(ctx context.Context, params ProxyParams, targetProvider provider.ProviderType, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time) error {
	flusher, ok := params.ResponseWriter.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	params.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	params.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	params.ResponseWriter.Header().Set("Connection", "keep-alive")
	params.ResponseWriter.WriteHeader(http.StatusOK)

	// Anthropic clients require the full message lifecycle; emit the opening
	// events before forwarding any upstream deltas.
	if params.InboundFormat == "anthropic" {
		msgID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
		messageStart := fmt.Sprintf(
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":%q,\"type\":\"message\",\"role\":\"assistant\",\"model\":%q,\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":1}}}\n\n",
			msgID, canReq.Model,
		)
		contentBlockStart := "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"
		params.ResponseWriter.Write([]byte(messageStart))
		params.ResponseWriter.Write([]byte(contentBlockStart))
		flusher.Flush()
	}

	sseReader := sse.NewReader(upResp.Body)
	var ttftMs int64 = 0
	var totalPromptTokens, totalCompletionTokens int
	var lastFinishReason string
	isFirstChunk := true

	for {
		event, err := sseReader.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		if isFirstChunk {
			ttftMs = time.Since(startTime).Milliseconds()
			isFirstChunk = false
		}

		// Convert SSE event to Canonical Stream Chunk
		var chunk *canonical.CanonicalStreamChunk

		switch targetProvider {
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
			}

		case provider.ProviderGemini:
			c, cErr := gemini.DecodeStreamChunk(event.Data, canReq.Model)
			if cErr == nil && c != nil {
				chunk = c
				if c.CompletionTokens > 0 {
					totalCompletionTokens = c.CompletionTokens
					totalPromptTokens = c.PromptTokens
				}
			}
		}

		if chunk == nil {
			continue
		}
		if chunk.FinishReason != "" {
			lastFinishReason = chunk.FinishReason
		}

		if params.InboundFormat == "openai" {
			// Skip the bare terminal marker; the [DONE] line is written below.
			if chunk.IsLast && chunk.DeltaContent == "" && chunk.FinishReason == "" {
				continue
			}
			sseFormatted, fErr := openai.FormatCanonicalToOpenAISSE(chunk)
			if fErr == nil && sseFormatted != "" {
				params.ResponseWriter.Write([]byte(sseFormatted))
				flusher.Flush()
			}
		} else {
			// Anthropic inbound: only forward content deltas here.
			if chunk.DeltaContent != "" {
				sseFormatted, fErr := anthropic.FormatCanonicalToAnthropicSSE(chunk)
				if fErr == nil && sseFormatted != "" {
					params.ResponseWriter.Write([]byte(sseFormatted))
					flusher.Flush()
				}
			}
		}
	}

	// Final stream completion signal
	if params.InboundFormat == "openai" {
		params.ResponseWriter.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	} else {
		stopReason := lastFinishReason
		if stopReason == "" {
			stopReason = "end_turn"
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

	cost := g.costCalc.CalculateCost(string(targetProvider), canReq.Model, totalPromptTokens, totalCompletionTokens)
	g.recordTelemetry(params, string(targetProvider), canReq.Model, totalPromptTokens, totalCompletionTokens, cost, ttftMs, http.StatusOK, "", startTime)

	return nil
}

func (g *GatewayService) recordTelemetry(params ProxyParams, provider, modelName string, promptTokens, completionTokens int, cost float64, ttftMs int64, statusCode int, errMsg string, startTime time.Time) {
	if g.telemetry == nil || params.ApiKey == nil {
		return
	}

	totalLatency := time.Since(startTime).Milliseconds()
	logEntry := model.UsageLog{
		CreatedAt:        time.Now(),
		UserID:           params.ApiKey.UserID,
		KeyID:            params.ApiKey.ID,
		InboundFormat:    params.InboundFormat,
		Provider:         provider,
		Model:            modelName,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		Cost:             cost,
		TTFTMs:           ttftMs,
		TotalLatencyMs:   totalLatency,
		StatusCode:       statusCode,
		ErrorMessage:     errMsg,
	}

	g.telemetry.Log(logEntry)
}

// IsProviderKeyError reports whether err indicates a missing provider credential.
func IsProviderKeyError(err error) bool {
	var pkeyErr *ErrProviderKey
	return errors.As(err, &pkeyErr)
}
