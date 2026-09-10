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

	checkInjection := getBoolConfig(cfg, "check_injection", true)
	checkJailbreak := getBoolConfig(cfg, "check_jailbreak", true)
	checkLeak := getBoolConfig(cfg, "check_system_leak", true)
	action := strings.ToLower(getStringConfig(cfg, "action", "block"))

	var bannedKeywords []string
	for _, kw := range getStringSliceConfig(cfg, "banned_keywords") {
		if trimmed := strings.ToLower(strings.TrimSpace(kw)); trimmed != "" {
			bannedKeywords = append(bannedKeywords, trimmed)
		}
	}

	// Helper to inspect string for guardrail violations
	detectViolation := func(text string) (string, bool) {
		if checkInjection || checkJailbreak {
			if pattern, matched := findFirstPatternMatch(p.injectionPatterns, text); matched {
				return fmt.Sprintf("Prompt injection / jailbreak detected (matched pattern: %s)", pattern), true
			}
		}

		if checkLeak {
			if pattern, matched := findFirstPatternMatch(p.leakPatterns, text); matched {
				return fmt.Sprintf("System prompt leakage attempt detected (matched pattern: %s)", pattern), true
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

func findFirstPatternMatch(regexes []*regexp.Regexp, text string) (string, bool) {
	for _, re := range regexes {
		if re.MatchString(text) {
			return re.String(), true
		}
	}
	return "", false
}
