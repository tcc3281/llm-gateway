package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/model"
	"llm-gateway/internal/provider"

	"github.com/google/uuid"
)

// ResolvedTarget is the provider and model chosen for an upstream request.
type ResolvedTarget struct {
	Provider    provider.ProviderType
	Name        string // provider key used for lookup ("openai", "anthropic", "gemini", or custom slug)
	BaseURL     string
	TargetModel string // explicit model name for virtual models or aliased targets
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
		return ResolvedTarget{}, fmt.Errorf("unknown provider %q", prov)
	}
}

// targetForProvider builds a ResolvedTarget for an explicit custom provider config.
func (g *GatewayService) targetForProvider(name, protocol, baseURL string) ResolvedTarget {
	p := provider.ProviderOpenAI
	if protocol == "anthropic" {
		p = provider.ProviderAnthropic
	}
	return ResolvedTarget{
		Provider: p,
		Name:     name,
		BaseURL:  baseURL,
	}
}

// resolveVirtualModel resolves dynamic routing for virtual models.
type preparedVirtualModel struct {
	vm           *model.VirtualModel
	primaryPool  []model.VirtualModelTarget
	fallbackPool []model.VirtualModelTarget
	totalWeight  int
	expiresAt    time.Time
}

// If modelName matches an active Virtual Model, it executes dynamic routing
// (e.g. weighted random traffic splitting / canary, followed by priority-based fallback chains),
// and returns the resolved targets ladder with TargetModel set.
func (g *GatewayService) resolveVirtualModel(ctx context.Context, modelName string, tenantID *uuid.UUID) (*model.VirtualModel, []ResolvedTarget, error) {
	if g.pg == nil {
		return nil, nil, nil
	}

	tenantKey := "global"
	if tenantID != nil {
		tenantKey = tenantID.String()
	}
	cacheKey := modelName + ":" + tenantKey

	var prep *preparedVirtualModel
	if val, ok := g.virtualModelCache.Load(cacheKey); ok {
		cached := val.(*preparedVirtualModel)
		if time.Now().Before(cached.expiresAt) {
			prep = cached
		} else {
			g.virtualModelCache.Delete(cacheKey)
		}
	}

	if prep == nil {
		vm, err := g.pg.GetVirtualModelByName(modelName, tenantID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Negative cache to prevent repeated cold DB queries for standard models
				g.virtualModelCache.Store(cacheKey, &preparedVirtualModel{
					vm:        nil,
					expiresAt: time.Now().Add(1 * time.Minute),
				})
				return nil, nil, nil
			}
			return nil, nil, err
		}
		if vm == nil || !vm.IsActive || len(vm.Targets) == 0 {
			g.virtualModelCache.Store(cacheKey, &preparedVirtualModel{
				vm:        nil,
				expiresAt: time.Now().Add(1 * time.Minute),
			})
			return nil, nil, nil
		}

		// Filter active targets
		var activeTargets []model.VirtualModelTarget
		for _, t := range vm.Targets {
			if t.IsActive {
				activeTargets = append(activeTargets, t)
			}
		}
		if len(activeTargets) == 0 {
			return nil, nil, fmt.Errorf("virtual model %q has no active targets configured", modelName)
		}

		// Find the minimum priority among active targets (normally 1)
		minPriority := activeTargets[0].Priority
		for _, t := range activeTargets {
			if t.Priority < minPriority {
				minPriority = t.Priority
			}
		}

		// Separate into Primary pool (priority == minPriority) and Fallback tiers
		var primaryPool []model.VirtualModelTarget
		var fallbackPool []model.VirtualModelTarget
		totalWeight := 0
		for _, t := range activeTargets {
			if t.Priority == minPriority {
				primaryPool = append(primaryPool, t)
				w := t.Weight
				if w <= 0 {
					w = 1
				}
				totalWeight += w
			} else {
				fallbackPool = append(fallbackPool, t)
			}
		}

		// Pre-sort fallbackPool by priority ASC, then ID ASC once upon caching
		sort.SliceStable(fallbackPool, func(i, j int) bool {
			if fallbackPool[i].Priority != fallbackPool[j].Priority {
				return fallbackPool[i].Priority < fallbackPool[j].Priority
			}
			return fallbackPool[i].ID < fallbackPool[j].ID
		})

		if totalWeight <= 0 {
			totalWeight = 1
		}

		prep = &preparedVirtualModel{
			vm:           vm,
			primaryPool:  primaryPool,
			fallbackPool: fallbackPool,
			totalWeight:  totalWeight,
			expiresAt:    time.Now().Add(1 * time.Minute),
		}
		g.virtualModelCache.Store(cacheKey, prep)
	}

	if prep.vm == nil {
		return nil, nil, nil
	}

	var selectedPrimary model.VirtualModelTarget
	var secondaryPrimaryTargets []model.VirtualModelTarget

	if len(prep.primaryPool) == 1 {
		selectedPrimary = prep.primaryPool[0]
		secondaryPrimaryTargets = nil
	} else {
		switch prep.vm.RoutingStrategy {
		case "fallback":
			selectedPrimary = prep.primaryPool[0]
			secondaryPrimaryTargets = prep.primaryPool[1:]
		case "round_robin":
			fallthrough
		default: // "weighted"
			r := rand.Intn(prep.totalWeight)
			accum := 0
			selectedIdx := 0
			for i, t := range prep.primaryPool {
				w := t.Weight
				if w <= 0 {
					w = 1
				}
				accum += w
				if r < accum {
					selectedIdx = i
					break
				}
			}
			selectedPrimary = prep.primaryPool[selectedIdx]
			for i, t := range prep.primaryPool {
				if i != selectedIdx {
					secondaryPrimaryTargets = append(secondaryPrimaryTargets, t)
				}
			}
		}
	}

	orderedTargets := append([]model.VirtualModelTarget{selectedPrimary}, secondaryPrimaryTargets...)
	orderedTargets = append(orderedTargets, prep.fallbackPool...)

	resolvedTargets := make([]ResolvedTarget, 0, len(orderedTargets))
	for _, t := range orderedTargets {
		rt, err := g.targetForProviderName(ctx, t.Provider)
		if err != nil {
			continue
		}
		rt.TargetModel = t.ModelName
		resolvedTargets = append(resolvedTargets, rt)
	}

	if len(resolvedTargets) == 0 {
		return nil, nil, fmt.Errorf("could not resolve any active upstream targets for virtual model %q", modelName)
	}

	return prep.vm, resolvedTargets, nil
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

func (g *GatewayService) checkTenantBoundary(ctx context.Context, apiKey *model.APIKey, requestedModel string) error {
	if apiKey == nil || apiKey.TenantID == nil {
		return nil
	}

	tenant, err := g.getCachedTenant(*apiKey.TenantID)
	if err != nil {
		return &ErrPermissionDenied{
			Message: "access denied: associated tenant not found",
		}
	}
	if tenant == nil {
		return nil
	}

	if !tenant.IsActive {
		return &ErrPermissionDenied{
			Message: fmt.Sprintf("access denied: tenant %q is inactive", tenant.Name),
		}
	}

	// Layer 1 System Boundary (contract / allowed_models)
	if len(tenant.AllowedModels) > 0 {
		allowed := false
		for _, m := range tenant.AllowedModels {
			if m == "*" || strings.EqualFold(m, requestedModel) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: tenant %q contract boundary restricts access to model %q", tenant.Name, requestedModel),
			}
		}
	}

	return nil
}

// reserveTenantHold performs Hold & Settle pre-flight reservation on RAM/Redis.
// It prevents bad debt from concurrent requests by reserving worst-case cost against prepaid balance.
func (g *GatewayService) reserveTenantHold(ctx context.Context, apiKey *model.APIKey, provider, modelName string, estimatedInputTokens, maxOutputTokens int) (float64, error) {
	if apiKey == nil || apiKey.TenantID == nil {
		return 0, nil
	}

	tenant, err := g.getCachedTenant(*apiKey.TenantID)
	if err != nil || tenant == nil {
		return 0, nil
	}

	if tenant.BillingMode != "prepaid" {
		return 0, nil
	}

	worstCaseCost := g.costCalc.EstimateMaxCost(provider, modelName, estimatedInputTokens, maxOutputTokens)

	var currentHold float64
	if g.redis != nil {
		currentHold, _ = g.redis.GetTenantCreditHold(ctx, tenant.ID.String())
	}

	available := tenant.CreditBalance - currentHold
	if available < worstCaseCost {
		return 0, &ErrPaymentRequired{
			Message: fmt.Sprintf("insufficient available credit balance ($%.4f available, $%.4f active hold, $%.4f required) for tenant %q; please top up", available, currentHold, worstCaseCost, tenant.Name),
		}
	}

	if g.redis != nil {
		_, _ = g.redis.AddTenantCreditHold(ctx, tenant.ID.String(), worstCaseCost, 5*time.Minute)
	}

	return worstCaseCost, nil
}
