package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// PostgresReader abstracts database access needed by the plugin engine.
type PostgresReader interface {
	ListActivePluginBindings() ([]model.PluginBinding, error)
}

// PluginEngine evaluates active plugin scopes and executes lifecycle pipelines.
type PluginEngine struct {
	pg       PostgresReader
	registry *Registry
}

// NewPluginEngine constructs a new engine.
func NewPluginEngine(pg PostgresReader, reg *Registry) *PluginEngine {
	if reg == nil {
		reg = DefaultRegistry()
	}
	return &PluginEngine{
		pg:       pg,
		registry: reg,
	}
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
	// 1. Filter bindings that match current request context
	var matching []model.PluginBinding

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

		if matched {
			matching = append(matching, b)
		}
	}

	// 2. Group by PluginName, picking the highest specificity scope
	bestByPlugin := make(map[string]model.PluginBinding)
	for _, b := range matching {
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

	// 3. Collect and sort by Priority ASC
	result := make([]model.PluginBinding, 0, len(bestByPlugin))
	for _, b := range bestByPlugin {
		result = append(result, b)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Priority < result[j].Priority
	})

	return result
}

// LoadBindings loads active bindings from DB and computes effective bindings for the request context.
func (e *PluginEngine) LoadBindings(
	ctx context.Context,
	apiKey *model.APIKey,
	user *model.User,
	tenantID *uuid.UUID,
	provider string,
	modelName string,
) ([]model.PluginBinding, error) {
	if e.pg == nil {
		return nil, nil
	}

	all, err := e.pg.ListActivePluginBindings()
	if err != nil {
		return nil, fmt.Errorf("failed to list active plugin bindings: %w", err)
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
func (e *PluginEngine) RunPostUpstream(pctx *PluginContext, resp *canonical.CanonicalResponse, bindings []model.PluginBinding) error {
	for _, b := range bindings {
		inst, ok := e.registry.Get(b.PluginName)
		if !ok {
			continue
		}
		if p, ok := inst.(PostUpstreamPlugin); ok {
			cfg := parseBindingConfig(b)
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
