package builtin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"llm-gateway/internal/plugin"
)

// PromptGuardPlugin provides defense against prompt injection, jailbreaks, and banned keywords.
type PromptGuardPlugin struct {
	injectionPatterns []*regexp.Regexp
	leakPatterns      []*regexp.Regexp
}

func NewPromptGuardPlugin() *PromptGuardPlugin {
	return &PromptGuardPlugin{
		injectionPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\s+(all\s+)?(previous|prior|above)\s+(instructions|directives|rules|prompts)\b`),
			regexp.MustCompile(`(?i)\b(system\s+override|admin\s+mode|root\s+access|developer\s+mode\s+enabled)\b`),
			regexp.MustCompile(`(?i)\b(DAN\s+mode|do\s+anything\s+now|jailbreak|unfiltered\s+mode)\b`),
			regexp.MustCompile(`(?i)\b(you\s+are\s+no\s+longer\s+bound|bypass\s+(safety|content)\s+(filters|guidelines))\b`),
		},
		leakPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(reveal|output|display|print|show)\s+(your\s+)?(initial|system|original|base)\s+(prompt|instructions|context)\b`),
			regexp.MustCompile(`(?i)\b(repeat\s+everything\s+above|what\s+are\s+your\s+instructions)\b`),
		},
	}
}

func (p *PromptGuardPlugin) Name() string {
	return "prompt-guard"
}

func (p *PromptGuardPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *PromptGuardPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	checkInjection := true
	if v, ok := cfg["check_injection"].(bool); ok {
		checkInjection = v
	}

	checkJailbreak := true
	if v, ok := cfg["check_jailbreak"].(bool); ok {
		checkJailbreak = v
	}

	checkLeak := true
	if v, ok := cfg["check_system_leak"].(bool); ok {
		checkLeak = v
	}

	action := "block"
	if v, ok := cfg["action"].(string); ok && v != "" {
		action = strings.ToLower(v)
	}

	var bannedKeywords []string
	if rawKeywords, ok := cfg["banned_keywords"].([]any); ok {
		for _, item := range rawKeywords {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				bannedKeywords = append(bannedKeywords, strings.ToLower(strings.TrimSpace(s)))
			}
		}
	}

	// Helper to inspect string for guardrail violations
	detectViolation := func(text string) (string, bool) {
		if checkInjection || checkJailbreak {
			for _, re := range pctxTextFind(p.injectionPatterns, text) {
				return fmt.Sprintf("Prompt injection / jailbreak detected (matched pattern: %s)", re), true
			}
		}

		if checkLeak {
			for _, re := range pctxTextFind(p.leakPatterns, text) {
				return fmt.Sprintf("System prompt leakage attempt detected (matched pattern: %s)", re), true
			}
		}

		if len(bannedKeywords) > 0 {
			lowerText := strings.ToLower(text)
			for _, kw := range bannedKeywords {
				if strings.Contains(lowerText, kw) {
					return fmt.Sprintf("Prohibited keyword detected: %q", kw), true
				}
			}
		}

		return "", false
	}

	var violationMsg string
	var violated bool

	if pctx.CanReq.SystemPrompt != "" {
		if msg, found := detectViolation(pctx.CanReq.SystemPrompt); found {
			violationMsg = msg
			violated = true
		}
	}

	if !violated {
		for _, m := range pctx.CanReq.Messages {
			if msg, found := detectViolation(m.Content); found {
				violationMsg = msg
				violated = true
				break
			}
		}
	}

	if violated {
		if action == "block" {
			pctx.Stop = true
			pctx.StopStatusCode = http.StatusBadRequest
			errResp := map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("Prompt Guard blocked request: %s", violationMsg),
					"type":    "guardrail_violation",
					"code":    "prompt_injection_blocked",
				},
			}
			pctx.StopErrorBody, _ = json.Marshal(errResp)
			return nil
		}
		// If action == "log_only", we do not halt pipeline, but can inject warning header
		if pctx.ExtraHeaders == nil {
			pctx.ExtraHeaders = make(map[string]string)
		}
		pctx.ExtraHeaders["X-Prompt-Guard-Warning"] = violationMsg
	}

	return nil
}

func pctxTextFind(regexes []*regexp.Regexp, text string) []string {
	var matched []string
	for _, re := range regexes {
		if re.MatchString(text) {
			matched = append(matched, re.String())
		}
	}
	return matched
}
