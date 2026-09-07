package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"llm-gateway/internal/adapter/canonical"
)

func TestFormatCanonicalToAnthropicSSE_TextDelta(t *testing.T) {
	chunk := &canonical.CanonicalStreamChunk{
		DeltaContent: "Hello, \"World\"!\nLine 2 \u2764",
	}
	formatted, err := FormatCanonicalToAnthropicSSE(chunk, 1, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(formatted, "event: content_block_delta\ndata: ") {
		t.Errorf("missing SSE prefix in: %s", formatted)
	}
	if !strings.HasSuffix(formatted, "\n\n") {
		t.Errorf("missing SSE suffix in: %s", formatted)
	}

	rawJSON := strings.TrimPrefix(formatted, "event: content_block_delta\ndata: ")
	rawJSON = strings.TrimSuffix(rawJSON, "\n\n")

	var parsed struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}

	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		t.Fatalf("failed to unmarshal formatted SSE JSON: %v", err)
	}

	if parsed.Type != "content_block_delta" {
		t.Errorf("expected type content_block_delta, got %s", parsed.Type)
	}
	if parsed.Index != 1 {
		t.Errorf("expected index 1, got %d", parsed.Index)
	}
	if parsed.Delta.Type != "text_delta" {
		t.Errorf("expected delta.type text_delta, got %s", parsed.Delta.Type)
	}
	if parsed.Delta.Text != chunk.DeltaContent {
		t.Errorf("expected text %q, got %q", chunk.DeltaContent, parsed.Delta.Text)
	}
}

func TestFormatCanonicalToAnthropicSSE_ThinkingDelta(t *testing.T) {
	chunk := &canonical.CanonicalStreamChunk{
		DeltaReasoning: "Let me think through this step by step...",
	}
	formatted, err := FormatCanonicalToAnthropicSSE(chunk, 0, "thinking")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rawJSON := strings.TrimPrefix(formatted, "event: content_block_delta\ndata: ")
	rawJSON = strings.TrimSuffix(rawJSON, "\n\n")

	var parsed struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	}

	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		t.Fatalf("failed to unmarshal thinking JSON: %v", err)
	}

	if parsed.Delta.Type != "thinking_delta" {
		t.Errorf("expected delta.type thinking_delta, got %s", parsed.Delta.Type)
	}
	if parsed.Delta.Thinking != chunk.DeltaReasoning {
		t.Errorf("expected thinking content %q, got %q", chunk.DeltaReasoning, parsed.Delta.Thinking)
	}
}

func TestDecodeStreamChunk_AnthropicEvents(t *testing.T) {
	// message_stop
	stopChunk, err := DecodeStreamChunk("message_stop", []byte(`{}`), "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopChunk.IsLast {
		t.Errorf("expected IsLast true for message_stop")
	}

	// content_block_delta text
	deltaPayload := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}`
	chunk, err := DecodeStreamChunk("content_block_delta", []byte(deltaPayload), "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunk.DeltaContent != "hello world" {
		t.Errorf("expected DeltaContent 'hello world', got %q", chunk.DeltaContent)
	}

	// message_delta with usage
	usagePayload := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`
	uChunk, err := DecodeStreamChunk("message_delta", []byte(usagePayload), "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uChunk.CompletionTokens != 42 {
		t.Errorf("expected 42 output tokens, got %d", uChunk.CompletionTokens)
	}
	if uChunk.FinishReason != "end_turn" {
		t.Errorf("expected finish reason end_turn, got %q", uChunk.FinishReason)
	}
}
