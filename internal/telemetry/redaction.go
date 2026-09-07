package telemetry

import (
	"encoding/json"
	"regexp"
	"strings"

	"llm-gateway/internal/adapter/canonical"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	emailRegex  = regexp.MustCompile(`(?i)[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	cardRegex   = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)
	apiKeyRegex = regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9_\-]{16,}|Bearer\s+[a-zA-Z0-9_\-\.]{19,}[a-zA-Z0-9_\-])`)
	phoneRegex  = regexp.MustCompile(`(?:\+?\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`)
)

// SanitizeText masks known sensitive PII patterns (emails, credit card numbers,
// API keys, and phone numbers) from the given text string.
func SanitizeText(text string) string {
	if text == "" {
		return text
	}
	s := emailRegex.ReplaceAllString(text, "[REDACTED_EMAIL]")
	s = cardRegex.ReplaceAllString(s, "[REDACTED_CARD]")
	s = apiKeyRegex.ReplaceAllString(s, "[REDACTED_KEY]")
	s = phoneRegex.ReplaceAllString(s, "[REDACTED_PHONE]")
	return s
}

// RecordPromptEvent records an inference prompt event on the span if policy is "event_only".
// If policy is "none" or empty, no message content is ever attached to the span.
func RecordPromptEvent(span trace.Span, policy string, messages []canonical.ChatMessage) {
	if span == nil || !span.IsRecording() || !strings.EqualFold(policy, "event_only") {
		return
	}

	type promptItem struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	items := make([]promptItem, 0, len(messages))
	for _, m := range messages {
		items = append(items, promptItem{
			Role:    m.Role,
			Content: SanitizeText(m.Content),
		})
	}

	b, err := json.Marshal(items)
	if err != nil {
		return
	}

	span.AddEvent("gen_ai.client.inference.prompt", trace.WithAttributes(
		attribute.String("gen_ai.prompt", string(b)),
	))
}

// RecordCompletionEvent records an inference completion event on the span if policy is "event_only".
func RecordCompletionEvent(span trace.Span, policy string, content string) {
	if span == nil || !span.IsRecording() || !strings.EqualFold(policy, "event_only") {
		return
	}

	sanitized := SanitizeText(content)
	span.AddEvent("gen_ai.client.inference.completion", trace.WithAttributes(
		attribute.String("gen_ai.completion", sanitized),
	))
}
