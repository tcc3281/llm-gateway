package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/alert"
	"llm-gateway/internal/analytics"
	"llm-gateway/internal/cache"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/permission"
	"llm-gateway/internal/plugin"
	_ "llm-gateway/internal/plugin/builtin"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/telemetry"

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

// IsProviderKeyError reports whether err indicates a missing provider credential.
func IsProviderKeyError(err error) bool {
	var pkeyErr *ErrProviderKey
	return errors.As(err, &pkeyErr)
}

// ErrPaymentRequired is returned when a tenant has insufficient prepaid credit balance.
type ErrPaymentRequired struct {
	Message string
}

func (e *ErrPaymentRequired) Error() string {
	return e.Message
}

func IsPaymentRequiredError(err error) bool {
	var ep *ErrPaymentRequired
	return errors.As(err, &ep)
}

type cachedUserItem struct {
	user      *model.User
	expiresAt time.Time
}

type cachedTenantItem struct {
	tenant    *model.Tenant
	expiresAt time.Time
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
	alertEngine          *alert.Engine
	pluginEngine         *plugin.PluginEngine

	// Zero-DB Hot Path in-memory caches
	userCache         sync.Map
	tenantCache       sync.Map
	virtualModelCache sync.Map
}

func (g *GatewayService) SetAlertEngine(e *alert.Engine) {
	g.alertEngine = e
}

func (g *GatewayService) AlertEngine() *alert.Engine {
	return g.alertEngine
}

func (g *GatewayService) SetPluginEngine(pe *plugin.PluginEngine) {
	g.pluginEngine = pe
}

func (g *GatewayService) PluginEngine() *plugin.PluginEngine {
	return g.pluginEngine
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
	var pe *plugin.PluginEngine
	if pg != nil {
		pe = plugin.NewPluginEngine(pg, nil)
	}
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
		pluginEngine:         pe,
	}
}

const (
	modelsCacheKey   = "models:list"
	modelsCacheTTL   = time.Hour
	modelsEmptyCache = 5 * time.Minute
)

// getCachedUser avoids DB queries on the hot path for user details
func (g *GatewayService) getCachedUser(userID uuid.UUID) (*model.User, error) {
	if val, ok := g.userCache.Load(userID); ok {
		item := val.(cachedUserItem)
		if time.Now().Before(item.expiresAt) {
			return item.user, nil
		}
		g.userCache.Delete(userID)
	}
	if g.pg == nil {
		return nil, nil
	}
	u, err := g.pg.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		g.userCache.Store(userID, cachedUserItem{
			user:      u,
			expiresAt: time.Now().Add(5 * time.Minute),
		})
	}
	return u, nil
}

// getCachedTenant avoids DB queries on the hot path for tenant details
func (g *GatewayService) getCachedTenant(tenantID uuid.UUID) (*model.Tenant, error) {
	if val, ok := g.tenantCache.Load(tenantID); ok {
		item := val.(cachedTenantItem)
		if time.Now().Before(item.expiresAt) {
			return item.tenant, nil
		}
		g.tenantCache.Delete(tenantID)
	}
	if g.pg == nil {
		return nil, nil
	}
	t, err := g.pg.GetTenantByID(tenantID)
	if err != nil {
		return nil, err
	}
	if t != nil {
		g.tenantCache.Store(tenantID, cachedTenantItem{
			tenant:    t,
			expiresAt: time.Now().Add(5 * time.Minute),
		})
	}
	return t, nil
}

type ProxyParams struct {
	InboundFormat  string
	RequestBody    []byte
	ApiKey         *model.APIKey
	UpstreamAPIKey string
	ClientIP       string
	ResponseWriter http.ResponseWriter
}

func (g *GatewayService) HandleRequest(ctx context.Context, params ProxyParams) error {
	startTime := time.Now()

	// 0. Setup Plugin Context & Pre-Auth Lifecycle Hook (Zero-DB Hot Path)
	var pctx *plugin.PluginContext
	var pluginBindings []model.PluginBinding

	if g.pluginEngine != nil {
		var user *model.User
		var tenant *model.Tenant
		var tenantID *uuid.UUID
		if params.ApiKey != nil {
			tenantID = params.ApiKey.TenantID
			user, _ = g.getCachedUser(params.ApiKey.UserID)
			if tenantID != nil {
				tenant, _ = g.getCachedTenant(*tenantID)
			}
		}

		pctx = &plugin.PluginContext{
			Ctx:            ctx,
			APIKey:         params.ApiKey,
			User:           user,
			Tenant:         tenant,
			RequestBody:    params.RequestBody,
			InboundFormat:  params.InboundFormat,
			ResponseWriter: params.ResponseWriter,
			Redis:          g.redis,
			Postgres:       g.pg,
			ClientIP:       params.ClientIP,
			StartTime:      startTime,
			ExtraHeaders:   make(map[string]string),
		}

		pluginBindings, _ = g.pluginEngine.LoadBindings(ctx, params.ApiKey, user, tenantID, "", "")
		if err := g.pluginEngine.RunPreAuth(pctx, pluginBindings); err != nil {
			return err
		}
		if pctx.Stop {
			if pctx.StopStatusCode > 0 {
				params.ResponseWriter.WriteHeader(pctx.StopStatusCode)
			}
			if len(pctx.StopErrorBody) > 0 {
				params.ResponseWriter.Write(pctx.StopErrorBody)
			}
			return nil
		}
	}

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

	if pctx != nil {
		pctx.CanReq = canReq
		pctx.Model = canReq.Model
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

	// 1c. Multi-Tenant System Boundary Check (Layer 1 Contract)
	if err := g.checkTenantBoundary(ctx, params.ApiKey, canReq.Model); err != nil {
		return err
	}

	// 1d. Plugin Post-Auth & Pre-Routing Lifecycle Hooks
	if g.pluginEngine != nil && pctx != nil {
		var tenantID *uuid.UUID
		if params.ApiKey != nil {
			tenantID = params.ApiKey.TenantID
		}
		pluginBindings, _ = g.pluginEngine.LoadBindings(ctx, params.ApiKey, pctx.User, tenantID, "", canReq.Model)
		if err := g.pluginEngine.RunPostAuth(pctx, pluginBindings); err != nil {
			return err
		}
		if pctx.Stop {
			if pctx.StopStatusCode > 0 {
				params.ResponseWriter.WriteHeader(pctx.StopStatusCode)
			}
			if len(pctx.StopErrorBody) > 0 {
				params.ResponseWriter.Write(pctx.StopErrorBody)
			}
			return nil
		}

		if err := g.pluginEngine.RunPreRouting(pctx, pluginBindings); err != nil {
			return err
		}
		if pctx.Stop {
			if pctx.StopStatusCode > 0 {
				params.ResponseWriter.WriteHeader(pctx.StopStatusCode)
			}
			if len(pctx.StopErrorBody) > 0 {
				params.ResponseWriter.Write(pctx.StopErrorBody)
			}
			return nil
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
	var primaryTarget ResolvedTarget
	var targets []ResolvedTarget
	targetModelName := canReq.Model

	var tenantID *uuid.UUID
	if params.ApiKey != nil {
		tenantID = params.ApiKey.TenantID
	}

	vm, vmTargets, vmErr := g.resolveVirtualModel(ctx, canReq.Model, tenantID)
	if vmErr != nil {
		return vmErr
	}
	if vm != nil && len(vmTargets) > 0 {
		primaryTarget = vmTargets[0]
		targets = vmTargets
		if primaryTarget.TargetModel != "" {
			targetModelName = primaryTarget.TargetModel
		}
		log.Printf("[virtual-model] %q matched virtual model alias (strategy=%s) -> primary: %s/%s (pool size: %d)",
			canReq.Model, vm.RoutingStrategy, primaryTarget.Name, targetModelName, len(targets))
	} else {
		resolved, err := g.resolveTarget(ctx, canReq.Model)
		if err != nil {
			return err
		}
		primaryTarget = resolved
		targets = g.resolveFallbackChain(ctx, primaryTarget)
		log.Printf("[routing] %q resolved from catalog -> %s (fallback targets: %d)", canReq.Model, primaryTarget.Name, len(targets))
	}

	// 3b. Multi-Tenant Hold & Settle: Reserve worst-case cost against prepaid credit balance
	heldAmount, hErr := g.reserveTenantHold(ctx, params.ApiKey, primaryTarget.Name, targetModelName, len(params.RequestBody)/4, canReq.MaxTokens)
	if hErr != nil {
		return hErr
	}
	if heldAmount > 0 {
		defer func() {
			if g.redis != nil && params.ApiKey != nil && params.ApiKey.TenantID != nil {
				_ = g.redis.ReleaseTenantCreditHold(context.Background(), params.ApiKey.TenantID.String(), heldAmount)
			}
		}()
	}

	// 4. Dispatch with Fallback Ladder (injecting plugin extra/removed headers)
	var extraHeaders map[string]string
	var removeHeaders []string
	if pctx != nil {
		extraHeaders = pctx.ExtraHeaders
		removeHeaders = pctx.RemoveHeaders
	}
	upResp, usedTarget, usedModel, attempts, err := g.sendWithFallback(ctx, targets, canReq, params.UpstreamAPIKey, extraHeaders, removeHeaders)
	if err != nil {
		durationSec := time.Since(startTime).Seconds()
		telemetry.RecordDurationMetric(ctx, primaryTarget.Name, targetModelName, "chat", durationSec, http.StatusServiceUnavailable)
		g.recordTelemetry(params, primaryTarget.Name, targetModelName, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusServiceUnavailable, err.Error(), startTime)
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
		if g.pluginEngine != nil && pctx != nil {
			pctx.Duration = time.Since(startTime)
			pctx.StatusCode = upResp.StatusCode
			pctx.Provider = string(usedTarget.Provider)
			_ = g.pluginEngine.RunPostUpstream(pctx, nil, pluginBindings)
		}
		return nil
	}

	// 5. Handle Streaming vs Non-Streaming Response
	if canReq.Stream {
		return g.handleStreamResponse(ctx, params, usedTarget, canReq, upResp, startTime, fallbackUsed, inputToolCalls)
	}

	return g.handleNonStreamResponse(ctx, params, usedTarget, canReq, upResp, startTime, cacheKey, cacheTTL, cacheEnabled, fallbackUsed, inputToolCalls, pctx, pluginBindings)
}

func (g *GatewayService) handleNonStreamResponse(ctx context.Context, params ProxyParams, target ResolvedTarget, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time, cacheKey string, cacheTTL time.Duration, cacheEnabled, fallbackUsed bool, inputToolCalls int, pctx *plugin.PluginContext, pluginBindings []model.PluginBinding) error {
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

	// Run Post-Upstream Plugins (v0.3.0)
	if g.pluginEngine != nil && pctx != nil {
		pctx.Duration = time.Since(startTime)
		pctx.StatusCode = http.StatusOK
		pctx.Provider = string(target.Provider)
		_ = g.pluginEngine.RunPostUpstream(pctx, canResp, pluginBindings)
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
