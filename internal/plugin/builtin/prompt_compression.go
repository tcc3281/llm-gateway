package builtin

import (
	"regexp"
	"strings"

	"llm-gateway/internal/plugin"
)

var (
	multiSpaceRegex   = regexp.MustCompile(`[^\S\r\n]{2,}`)
	multiNewlineRegex = regexp.MustCompile(`\n{3,}`)
)

// PromptCompressionPlugin optimizes prompt size by compressing redundant whitespaces and newlines.
type PromptCompressionPlugin struct{}

func NewPromptCompressionPlugin() *PromptCompressionPlugin {
	return &PromptCompressionPlugin{}
}

func (p *PromptCompressionPlugin) Name() string {
	return "prompt-compression"
}

func (p *PromptCompressionPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *PromptCompressionPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	collapseSpaces := getBoolConfig(cfg, "collapse_spaces", true)
	collapseNewlines := getBoolConfig(cfg, "collapse_newlines", true)
	trimWhitespace := getBoolConfig(cfg, "trim_whitespace", true)

	compress := func(text string) string {
		out := text
		if collapseSpaces {
			out = multiSpaceRegex.ReplaceAllString(out, " ")
		}
		if collapseNewlines {
			out = multiNewlineRegex.ReplaceAllString(out, "\n\n")
		}
		if trimWhitespace {
			out = strings.TrimSpace(out)
		}
		return out
	}

	// Compress System Prompt
	if pctx.CanReq.SystemPrompt != "" {
		pctx.CanReq.SystemPrompt = compress(pctx.CanReq.SystemPrompt)
	}

	// Compress Messages
	for i := range pctx.CanReq.Messages {
		if pctx.CanReq.Messages[i].Content != "" {
			pctx.CanReq.Messages[i].Content = compress(pctx.CanReq.Messages[i].Content)
		}
		for j := range pctx.CanReq.Messages[i].ContentParts {
			if pctx.CanReq.Messages[i].ContentParts[j].Type == "text" && pctx.CanReq.Messages[i].ContentParts[j].Text != "" {
				pctx.CanReq.Messages[i].ContentParts[j].Text = compress(pctx.CanReq.Messages[i].ContentParts[j].Text)
			}
		}
	}

	if pctx.ExtraHeaders == nil {
		pctx.ExtraHeaders = make(map[string]string)
	}
	pctx.ExtraHeaders["X-Prompt-Compressed"] = "true"

	return nil
}
