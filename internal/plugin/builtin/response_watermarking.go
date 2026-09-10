package builtin

import (
	"strings"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

// ResponseWatermarkingPlugin stamps outbound LLM responses with traceable verification metadata.
type ResponseWatermarkingPlugin struct{}

func NewResponseWatermarkingPlugin() *ResponseWatermarkingPlugin {
	return &ResponseWatermarkingPlugin{}
}

func (p *ResponseWatermarkingPlugin) Name() string {
	return "response-watermarking"
}

func (p *ResponseWatermarkingPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostUpstream}
}

func (p *ResponseWatermarkingPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	watermarkID := getStringConfig(cfg, "watermark_id", "llm-gw-verified")
	headerName := getStringConfig(cfg, "header_name", "X-Gateway-Watermark")
	appendFooter := getBoolConfig(cfg, "append_footer", false)
	footerText := getStringConfig(cfg, "footer_text", "\n\n---\n*Generated and verified via Enterprise LLM Gateway*")

	if pctx.ResponseWriter != nil && headerName != "" && watermarkID != "" {
		pctx.ResponseWriter.Header().Set(headerName, watermarkID)
	}

	if appendFooter && resp != nil && strings.TrimSpace(resp.Content) != "" {
		resp.Content += footerText
	}

	return nil
}
