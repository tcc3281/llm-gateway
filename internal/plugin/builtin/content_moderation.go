package builtin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"llm-gateway/internal/plugin"
)

var (
	hateRegex     = regexp.MustCompile(`(?i)\b(kill\s+all|genocide|exterminate|hate\s+speech|white\s+supremacy|terrorist\s+attack)\b`)
	violenceRegex = regexp.MustCompile(`(?i)\b(how\s+to\s+build\s+a\s+bomb|make\s+poison|assassinate|synthesize\s+explosives)\b`)
	selfHarmRegex = regexp.MustCompile(`(?i)\b(how\s+to\s+commit\s+suicide|methods\s+of\s+self[- ]harm|end\s+my\s+life)\b`)
)

// ContentModerationPlugin evaluates prompt safety and rejects toxic, violent, or illegal input.
type ContentModerationPlugin struct{}

func NewContentModerationPlugin() *ContentModerationPlugin {
	return &ContentModerationPlugin{}
}

func (p *ContentModerationPlugin) Name() string {
	return "content-moderation"
}

func (p *ContentModerationPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *ContentModerationPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	blockHate := getBoolConfig(cfg, "block_hate", true)
	blockViolence := getBoolConfig(cfg, "block_violence", true)
	blockSelfHarm := getBoolConfig(cfg, "block_self_harm", true)
	action := getStringConfig(cfg, "action", "block")
	customBanned := getStringSliceConfig(cfg, "custom_banned_words")

	// Collect all text from messages and system prompt
	var textParts []string
	if pctx.CanReq.SystemPrompt != "" {
		textParts = append(textParts, pctx.CanReq.SystemPrompt)
	}
	for _, m := range pctx.CanReq.Messages {
		if m.Content != "" {
			textParts = append(textParts, m.Content)
		}
		for _, cp := range m.ContentParts {
			if cp.Type == "text" && cp.Text != "" {
				textParts = append(textParts, cp.Text)
			}
		}
	}
	fullText := strings.Join(textParts, " ")

	// Check moderation rules
	var violation string
	if blockHate && hateRegex.MatchString(fullText) {
		violation = "hate_speech_detected"
	} else if blockViolence && violenceRegex.MatchString(fullText) {
		violation = "violence_or_weapons_detected"
	} else if blockSelfHarm && selfHarmRegex.MatchString(fullText) {
		violation = "self_harm_content_detected"
	} else if len(customBanned) > 0 {
		lowerText := strings.ToLower(fullText)
		for _, word := range customBanned {
			w := strings.ToLower(strings.TrimSpace(word))
			if w != "" && strings.Contains(lowerText, w) {
				violation = fmt.Sprintf("banned_keyword_%s", w)
				break
			}
		}
	}

	if violation != "" {
		if action == "block" {
			pctx.Stop = true
			pctx.StopStatusCode = http.StatusBadRequest
			errResp := map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("Content moderation policy violation: %s", violation),
					"type":    "content_policy_violation",
					"code":    "moderation_rejected",
				},
			}
			pctx.StopErrorBody, _ = json.Marshal(errResp)
			return nil
		}

		// Warn mode
		if pctx.ExtraHeaders == nil {
			pctx.ExtraHeaders = make(map[string]string)
		}
		pctx.ExtraHeaders["X-Moderation-Warning"] = violation
	}

	return nil
}
