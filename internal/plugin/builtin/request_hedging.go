package builtin

import (
	"fmt"

	"llm-gateway/internal/plugin"
)

type RequestHedgingPlugin struct{}

func NewRequestHedgingPlugin() *RequestHedgingPlugin {
	return &RequestHedgingPlugin{}
}

func (p *RequestHedgingPlugin) Name() string {
	return "request-hedging"
}

func (p *RequestHedgingPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *RequestHedgingPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	delayMs := 300
	if v, ok := cfg["hedging_delay_ms"].(float64); ok && v > 0 {
		delayMs = int(v)
	}

	secProvider, _ := cfg["secondary_provider"].(string)

	if pctx.ExtraHeaders == nil {
		pctx.ExtraHeaders = make(map[string]string)
	}

	pctx.ExtraHeaders["X-Request-Hedging"] = "enabled"
	pctx.ExtraHeaders["X-Hedging-Delay-Ms"] = fmt.Sprintf("%d", delayMs)
	if secProvider != "" {
		pctx.ExtraHeaders["X-Hedging-Secondary-Provider"] = secProvider
	}

	return nil
}
