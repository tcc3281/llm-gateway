package builtin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

type RetryCircuitBreakerPlugin struct{}

func NewRetryCircuitBreakerPlugin() *RetryCircuitBreakerPlugin {
	return &RetryCircuitBreakerPlugin{}
}

func (p *RetryCircuitBreakerPlugin) Name() string {
	return "retry-circuit-breaker"
}

func (p *RetryCircuitBreakerPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting, plugin.HookPostUpstream}
}

func (p *RetryCircuitBreakerPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.Redis == nil || pctx.Provider == "" {
		return nil
	}

	circuitKey := fmt.Sprintf("cb:state:%s", pctx.Provider)
	state, err := pctx.Redis.Get(pctx.Ctx, circuitKey)
	if err == nil && state == "OPEN" {
		pctx.Stop = true
		pctx.StopStatusCode = http.StatusServiceUnavailable
		errResp := map[string]any{
			"error": map[string]any{
				"message": fmt.Sprintf("Circuit breaker is OPEN for provider %q due to repeated upstream failures. Request rejected to prevent cascading errors.", pctx.Provider),
				"type":    "circuit_breaker_open",
				"code":    "upstream_unavailable",
			},
		}
		pctx.StopErrorBody, _ = json.Marshal(errResp)
		return nil
	}

	return nil
}

func (p *RetryCircuitBreakerPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	// PostUpstream executes upon successful non-streaming response; we can reset consecutive error counter
	if pctx.Redis == nil || pctx.Provider == "" {
		return nil
	}

	failCountKey := fmt.Sprintf("cb:failcount:%s", pctx.Provider)
	_ = pctx.Redis.Del(pctx.Ctx, failCountKey)
	return nil
}
