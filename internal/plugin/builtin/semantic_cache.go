package builtin

import (
	"encoding/json"
	"net/http"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/cache"
	"llm-gateway/internal/plugin"
)

type SemanticCachePlugin struct{}

func NewSemanticCachePlugin() *SemanticCachePlugin {
	return &SemanticCachePlugin{}
}

func (p *SemanticCachePlugin) Name() string {
	return "semantic-cache"
}

func (p *SemanticCachePlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting, plugin.HookPostUpstream}
}

func (p *SemanticCachePlugin) IsAsyncPostUpstream() bool {
	return true
}

func (p *SemanticCachePlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil || pctx.CanReq.Stream || pctx.Redis == nil {
		return nil
	}

	cacheKey := cache.CacheKey(pctx.CanReq)
	cachedData, err := pctx.Redis.GetRaw(pctx.Ctx, cacheKey)
	if err != nil || len(cachedData) == 0 {
		return nil
	}

	if pctx.ResponseWriter != nil {
		pctx.ResponseWriter.Header().Set("Content-Type", "application/json")
		pctx.ResponseWriter.Header().Set("X-Cache", "HIT")
		pctx.ResponseWriter.WriteHeader(http.StatusOK)
		_, _ = pctx.ResponseWriter.Write(cachedData)
	}

	pctx.Stop = true
	return nil
}

func (p *SemanticCachePlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	if pctx.CanReq == nil || pctx.CanReq.Stream || pctx.Redis == nil || resp == nil {
		return nil
	}

	ttlSeconds := getIntConfig(cfg, "ttl_seconds", 300)

	cacheKey := cache.CacheKey(pctx.CanReq)
	respBytes, err := json.Marshal(resp)
	if err == nil && len(respBytes) > 0 {
		_ = pctx.Redis.SetRaw(pctx.Ctx, cacheKey, respBytes, time.Duration(ttlSeconds)*time.Second)
	}

	return nil
}
