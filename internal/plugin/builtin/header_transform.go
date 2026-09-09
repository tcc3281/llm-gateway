package builtin

import (
	"fmt"

	"llm-gateway/internal/plugin"
)

type HeaderTransformPlugin struct{}

func NewHeaderTransformPlugin() *HeaderTransformPlugin {
	return &HeaderTransformPlugin{}
}

func (p *HeaderTransformPlugin) Name() string {
	return "header-transform"
}

func (p *HeaderTransformPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *HeaderTransformPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.ExtraHeaders == nil {
		pctx.ExtraHeaders = make(map[string]string)
	}

	// 1. Set headers
	if setRaw, ok := cfg["set"].(map[string]any); ok {
		for k, v := range setRaw {
			if vStr, ok := v.(string); ok {
				pctx.ExtraHeaders[k] = vStr
			} else {
				pctx.ExtraHeaders[k] = fmt.Sprintf("%v", v)
			}
		}
	} else if setMap, ok := cfg["set"].(map[string]string); ok {
		for k, v := range setMap {
			pctx.ExtraHeaders[k] = v
		}
	}

	// 2. Remove headers
	if removes := getStringSliceConfig(cfg, "remove"); len(removes) > 0 {
		pctx.RemoveHeaders = append(pctx.RemoveHeaders, removes...)
	}

	return nil
}
