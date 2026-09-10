package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// PostgresReader abstracts database access needed by the plugin engine.
type PostgresReader interface {
	ListActivePluginBindings() ([]model.PluginBinding, error)
}

// PluginEngine evaluates active plugin scopes and executes lifecycle pipelines.
type PluginEngine struct {
	pg             PostgresReader
	registry       *Registry
	mu             sync.RWMutex
	cachedBindings []model.PluginBinding
	lastLoaded     time.Time
	cacheTTL       time.Duration
}

// NewPluginEngine constructs a new engine and initializes in-memory cache.
func NewPluginEngine(pg PostgresReader, reg *Registry) *PluginEngine {
	if reg == nil {
		reg = DefaultRegistry()
	}
	pe := &PluginEngine{
		pg:       pg,
		registry: reg,
		cacheTTL: 30 * time.Second,
	}
	// Best-effort warm-up cache on startup
	if pg != nil {
		_ = pe.ReloadBindings(context.Background())
	}
	return pe
}

// SetCacheTTL adjusts the local in-memory cache expiry window.
func (e *PluginEngine) SetCacheTTL(ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cacheTTL = ttl
}

// ReloadBindings refreshes the in-memory cache directly from the database.
func (e *PluginEngine) ReloadBindings(ctx context.Context) error {
	if e.pg == nil {
		return nil
	}
	all, err := e.pg.ListActivePluginBindings()
	if err != nil {
		return fmt.Errorf("failed to reload active plugin bindings: %w", err)
	}

	e.mu.Lock()
	e.cachedBindings = all
	e.lastLoaded = time.Now()
	e.mu.Unlock()
	return nil
}

// InvalidateCache invalidates the in-memory cache immediately.
func (e *PluginEngine) InvalidateCache(ctx context.Context) error {
	return e.ReloadBindings(ctx)
}

// GetActiveBindings returns active bindings using the high-performance concurrent in-memory cache.
func (e *PluginEngine) GetActiveBindings(ctx context.Context) ([]model.PluginBinding, error) {
	e.mu.RLock()
	cached := e.cachedBindings
	lastLoaded := e.lastLoaded
	ttl := e.cacheTTL
	e.mu.RUnlock()

	// High-performance hot path: return cached bindings without DB hit
	if len(cached) > 0 && time.Since(lastLoaded) < ttl {
		return cached, nil
	}

	// Cache miss or expired: reload under write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-check after acquiring write lock
	if len(e.cachedBindings) > 0 && time.Since(e.lastLoaded) < e.cacheTTL {
		return e.cachedBindings, nil
	}

	if e.pg == nil {
		return nil, nil
	}

	loaded, err := e.pg.ListActivePluginBindings()
	if err != nil {
		// Graceful degradation: return stale cache on transient DB errors
		if len(e.cachedBindings) > 0 {
			return e.cachedBindings, nil
		}
		return nil, fmt.Errorf("failed to list active plugin bindings: %w", err)
	}

	e.cachedBindings = loaded
	e.lastLoaded = time.Now()
	return loaded, nil
}

// StartBackgroundSync periodically checks and refreshes plugin bindings.
func (e *PluginEngine) StartBackgroundSync(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = e.ReloadBindings(ctx)
			}
		}
	}()
}

// StartRedisSubscriber listens for real-time cache invalidation events published over Redis.
func (e *PluginEngine) StartRedisSubscriber(ctx context.Context, redisClient *db.RedisClient) {
	if redisClient == nil {
		return
	}
	go func() {
		pubsub := redisClient.SubscribePluginBindings(ctx)
		if pubsub == nil {
			return
		}
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg != nil {
					_ = e.ReloadBindings(ctx)
				}
			}
		}
	}()
}

// Scope specificity ranks: higher number = higher priority override.
var scopeSpecificity = map[string]int{
	"global":   1,
	"provider": 2,
	"model":    3,
	"tenant":   4,
	"role":     5,
	"user":     6,
	"api_key":  7,
}

// ResolveEffectiveBindings filters and deduplicates active bindings for a specific request.
// Under single-run mode, when a plugin is bound at multiple scopes, the binding at the
// most specific scope overrides less specific scopes.
func ResolveEffectiveBindings(
	allBindings []model.PluginBinding,
	apiKey *model.APIKey,
	user *model.User,
	tenantID *uuid.UUID,
	provider string,
	modelName string,
) []model.PluginBinding {
	if len(allBindings) == 0 {
		return nil
	}

	// Single-pass: filter and resolve highest-specificity binding per plugin directly
	bestByPlugin := make(map[string]model.PluginBinding)

	for _, b := range allBindings {
		if !b.IsActive {
			continue
		}

		matched := false
		switch b.ScopeLevel {
		case "global":
			matched = true
		case "provider":
			if b.ScopeProvider != nil && provider != "" && *b.ScopeProvider == provider {
				matched = true
			}
		case "model":
			if b.ScopeModel != nil && modelName != "" && *b.ScopeModel == modelName {
				matched = true
			}
		case "tenant":
			if b.ScopeTenantID != nil && tenantID != nil && *b.ScopeTenantID == *tenantID {
				matched = true
			}
		case "role":
			if b.ScopeRole != nil && user != nil && user.Role == *b.ScopeRole {
				matched = true
			}
		case "user":
			if b.ScopeUserID != nil && user != nil && *b.ScopeUserID == user.ID {
				matched = true
			}
		case "api_key":
			if b.ScopeKeyID != nil && apiKey != nil && *b.ScopeKeyID == apiKey.ID {
				matched = true
			}
		}

		if !matched {
			continue
		}

		currBest, exists := bestByPlugin[b.PluginName]
		if !exists {
			bestByPlugin[b.PluginName] = b
			continue
		}

		currRank := scopeSpecificity[currBest.ScopeLevel]
		newRank := scopeSpecificity[b.ScopeLevel]

		if newRank > currRank {
			bestByPlugin[b.PluginName] = b
		} else if newRank == currRank && b.Priority < currBest.Priority {
			// Lower priority number runs earlier / wins
			bestByPlugin[b.PluginName] = b
		}
	}

	if len(bestByPlugin) == 0 {
		return nil
	}

	// Collect and sort by Priority ASC
	result := make([]model.PluginBinding, 0, len(bestByPlugin))
	for _, b := range bestByPlugin {
		result = append(result, b)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Priority < result[j].Priority
	})

	return result
}

// LoadBindings loads active bindings from memory cache (zero DB queries on hot path)
// and computes effective bindings for the request context.
func (e *PluginEngine) LoadBindings(
	ctx context.Context,
	apiKey *model.APIKey,
	user *model.User,
	tenantID *uuid.UUID,
	provider string,
	modelName string,
) ([]model.PluginBinding, error) {
	all, err := e.GetActiveBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active plugin bindings: %w", err)
	}

	return ResolveEffectiveBindings(all, apiKey, user, tenantID, provider, modelName), nil
}

func parseBindingConfig(b model.PluginBinding) map[string]any {
	cfg := make(map[string]any)
	if len(b.Config) > 0 {
		_ = json.Unmarshal(b.Config, &cfg)
	}
	return cfg
}

// RunPreAuth executes all PreAuth plugins in order.
func (e *PluginEngine) RunPreAuth(pctx *PluginContext, bindings []model.PluginBinding) error {
	for _, b := range bindings {
		inst, ok := e.registry.Get(b.PluginName)
		if !ok {
			continue
		}
		if p, ok := inst.(PreAuthPlugin); ok {
			cfg := parseBindingConfig(b)
			if err := p.ExecPreAuth(pctx, cfg); err != nil {
				return err
			}
			if pctx.Stop {
				return nil
			}
		}
	}
	return nil
}

// RunPostAuth executes all PostAuth plugins in order.
func (e *PluginEngine) RunPostAuth(pctx *PluginContext, bindings []model.PluginBinding) error {
	for _, b := range bindings {
		inst, ok := e.registry.Get(b.PluginName)
		if !ok {
			continue
		}
		if p, ok := inst.(PostAuthPlugin); ok {
			cfg := parseBindingConfig(b)
			if err := p.ExecPostAuth(pctx, cfg); err != nil {
				return err
			}
			if pctx.Stop {
				return nil
			}
		}
	}
	return nil
}

// RunPreRouting executes all PreRouting plugins in order.
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

// RunPostUpstream executes all PostUpstream plugins in order.
// Observability plugins (implementing AsyncPostUpstreamPlugin) are executed in background
// goroutines with a detached context so they never block the proxy response to the client.
// Mutating plugins (e.g. response watermarking) are executed synchronously.
func (e *PluginEngine) RunPostUpstream(pctx *PluginContext, resp *canonical.CanonicalResponse, bindings []model.PluginBinding) error {
	for _, b := range bindings {
		inst, ok := e.registry.Get(b.PluginName)
		if !ok {
			continue
		}
		if p, ok := inst.(PostUpstreamPlugin); ok {
			cfg := parseBindingConfig(b)

			// Check if plugin is pure observability and should run asynchronously
			if asyncP, isAsync := inst.(AsyncPostUpstreamPlugin); isAsync && asyncP.IsAsyncPostUpstream() {
				pctxCopy := *pctx
				asyncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				pctxCopy.Ctx = asyncCtx

				var respCopy *canonical.CanonicalResponse
				if resp != nil {
					rc := *resp
					respCopy = &rc
				}

				go func(targetPlugin PostUpstreamPlugin, cCtx PluginContext, rCopy *canonical.CanonicalResponse, cConfig map[string]any) {
					defer cancel()
					_ = targetPlugin.ExecPostUpstream(&cCtx, rCopy, cConfig)
				}(p, pctxCopy, respCopy, cfg)
				continue
			}

			// Synchronous execution for mutating plugins
			if err := p.ExecPostUpstream(pctx, resp, cfg); err != nil {
				return err
			}
			if pctx.Stop {
				return nil
			}
		}
	}
	return nil
}
