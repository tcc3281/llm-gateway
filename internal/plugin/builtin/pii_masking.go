package builtin

import (
	"regexp"

	"llm-gateway/internal/plugin"
)

var (
	emailRegex       = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	phoneRegex       = regexp.MustCompile(`(\+84|0|\+1)[0-9\s.-]{8,13}`)
	creditCardRegex  = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`)
	nationalIDRegex  = regexp.MustCompile(`\b\d{12}\b|\b\d{3}-\d{2}-\d{4}\b`)
	apiKeySecretRegex = regexp.MustCompile(`\b(sk-[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9-._~+/]+=*)\b`)
)

type PIIMaskingPlugin struct{}

func NewPIIMaskingPlugin() *PIIMaskingPlugin {
	return &PIIMaskingPlugin{}
}

func (p *PIIMaskingPlugin) Name() string {
	return "pii-masking"
}

func (p *PIIMaskingPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *PIIMaskingPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	replacement := getStringConfig(cfg, "replacement", "[REDACTED]")

	// Determine active pattern matchers
	patternsToRun := []string{"email", "phone", "credit_card", "national_id", "api_key"}
	if configuredPatterns := getStringSliceConfig(cfg, "patterns"); len(configuredPatterns) > 0 {
		patternsToRun = configuredPatterns
	}

	maskString := func(input string) string {
		out := input
		for _, pat := range patternsToRun {
			switch pat {
			case "email":
				out = emailRegex.ReplaceAllString(out, replacement)
			case "phone":
				out = phoneRegex.ReplaceAllString(out, replacement)
			case "credit_card":
				out = creditCardRegex.ReplaceAllString(out, replacement)
			case "national_id":
				out = nationalIDRegex.ReplaceAllString(out, replacement)
			case "api_key":
				out = apiKeySecretRegex.ReplaceAllString(out, replacement)
			}
		}
		return out
	}

	// Mask messages
	for i := range pctx.CanReq.Messages {
		if pctx.CanReq.Messages[i].Content != "" {
			pctx.CanReq.Messages[i].Content = maskString(pctx.CanReq.Messages[i].Content)
		}
		for j := range pctx.CanReq.Messages[i].ContentParts {
			if pctx.CanReq.Messages[i].ContentParts[j].Type == "text" {
				pctx.CanReq.Messages[i].ContentParts[j].Text = maskString(pctx.CanReq.Messages[i].ContentParts[j].Text)
			}
		}
	}

	// Mask SystemPrompt if set
	if pctx.CanReq.SystemPrompt != "" {
		pctx.CanReq.SystemPrompt = maskString(pctx.CanReq.SystemPrompt)
	}

	return nil
}
