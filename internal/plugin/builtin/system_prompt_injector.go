package builtin

import (
	"strings"

	"llm-gateway/internal/plugin"
)

// SystemPromptInjectorPlugin injects or overrides system instructions based on scope.
type SystemPromptInjectorPlugin struct{}

func NewSystemPromptInjectorPlugin() *SystemPromptInjectorPlugin {
	return &SystemPromptInjectorPlugin{}
}

func (p *SystemPromptInjectorPlugin) Name() string {
	return "system-prompt-injector"
}

func (p *SystemPromptInjectorPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *SystemPromptInjectorPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	promptToInject := strings.TrimSpace(getStringConfig(cfg, "system_prompt", ""))
	if promptToInject == "" {
		return nil
	}

	mode := getStringConfig(cfg, "mode", "prepend")

	currentPrompt := strings.TrimSpace(pctx.CanReq.SystemPrompt)

	switch mode {
	case "override":
		pctx.CanReq.SystemPrompt = promptToInject
	case "append":
		if currentPrompt != "" {
			pctx.CanReq.SystemPrompt = currentPrompt + "\n\n" + promptToInject
		} else {
			pctx.CanReq.SystemPrompt = promptToInject
		}
	case "prepend":
		fallthrough
	default:
		if currentPrompt != "" {
			pctx.CanReq.SystemPrompt = promptToInject + "\n\n" + currentPrompt
		} else {
			pctx.CanReq.SystemPrompt = promptToInject
		}
	}

	return nil
}
