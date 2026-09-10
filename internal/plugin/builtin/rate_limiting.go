package builtin

import (
	"fmt"

	"llm-gateway/internal/plugin"
)

type RateLimitingPlugin struct{}

func NewRateLimitingPlugin() *RateLimitingPlugin {
	return &RateLimitingPlugin{}
}

func (p *RateLimitingPlugin) Name() string {
	return "rate-limiting"
}

func (p *RateLimitingPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostAuth}
}

func (p *RateLimitingPlugin) ExecPostAuth(pctx *plugin.PluginContext, cfg map[string]any) error {
	rpm := getIntConfig(cfg, "requests_per_minute", 0)
	if rpm <= 0 || pctx.Redis == nil {
		return nil
	}

	// Derive rate limit key based on available context identity
	keyIdentifier := "global"
	if pctx.APIKey != nil {
		keyIdentifier = fmt.Sprintf("key:%s", pctx.APIKey.ID.String())
	} else if pctx.User != nil {
		keyIdentifier = fmt.Sprintf("user:%s", pctx.User.ID.String())
	} else if pctx.Tenant != nil {
		keyIdentifier = fmt.Sprintf("tenant:%s", pctx.Tenant.ID.String())
	}

	allowed, err := pctx.Redis.AllowRequest(pctx.Ctx, fmt.Sprintf("plugin:rate_limiting:%s", keyIdentifier), rpm)
	if err != nil {
		// Log and fail open to not block traffic on Redis error
		return nil
	}

	if !allowed {
		pctx.Stop = true
		pctx.StopStatusCode = 429
		pctx.StopErrorBody = []byte(fmt.Sprintf(`{"error":{"code":"rate_limit_exceeded","message":"Rate limit of %d requests/minute exceeded by plugin policy"}}`, rpm))
	}

	return nil
}
