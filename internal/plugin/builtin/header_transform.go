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
	if removeRaw, ok := cfg["remove"].([]any); ok {
		for _, r := range removeRaw {
			if rStr, ok := r.(string); ok && rStr != "" {
				pctx.RemoveHeaders = append(pctx.RemoveHeaders, rStr)
			}
		}
	} else if removeSlice, ok := cfg["remove"].([]string); ok {
		pctx.RemoveHeaders = append(pctx.RemoveHeaders, removeSlice...)
	}

	return nil
}
