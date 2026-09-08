package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"llm-gateway/internal/adapter/canonical"
)

func TestFormatCanonicalToOpenAISSE(t *testing.T) {
	finish := "stop"
	chunk := &canonical.CanonicalStreamChunk{
		ID:           "chatcmpl-123",
		Model:        "gpt-4o",
		DeltaContent: "Hello from OpenAI adapter",
		FinishReason: finish,
	}

	formatted, err := FormatCanonicalToOpenAISSE(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(formatted, "data: ") || !strings.HasSuffix(formatted, "\n\n") {
		t.Fatalf("malformed OpenAI SSE line: %s", formatted)
	}

	raw := strings.TrimSuffix(strings.TrimPrefix(formatted, "data: "), "\n\n")
	var parsed OpenAIStreamChunk
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("failed to decode OpenAI stream chunk JSON: %v", err)
	}

	if len(parsed.Choices) == 0 {
		t.Fatalf("expected at least 1 choice")
	}
	if parsed.Choices[0].Delta.Content != chunk.DeltaContent {
		t.Errorf("expected delta content %q, got %q", chunk.DeltaContent, parsed.Choices[0].Delta.Content)
	}
	if *parsed.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason stop, got %v", parsed.Choices[0].FinishReason)
	}
}

func TestClampMaxTokens(t *testing.T) {
	// GPT-4o clamp to 16,384
	if clamped := clampMaxTokens("gpt-4o", 32000); clamped != 16384 {
		t.Errorf("expected gpt-4o clamped to 16384, got %d", clamped)
	}
	if clamped := clampMaxTokens("gpt-4o-mini", 8000); clamped != 8000 {
		t.Errorf("expected 8000 to stay unchanged, got %d", clamped)
	}

	// GPT-4 / 3.5 clamp to 4,096
	if clamped := clampMaxTokens("gpt-4-turbo", 8192); clamped != 4096 {
		t.Errorf("expected gpt-4-turbo clamped to 4096, got %d", clamped)
	}

	// o1/o3 clamp to 100,000
	if clamped := clampMaxTokens("o1-preview", 120000); clamped != 100000 {
		t.Errorf("expected o1-preview clamped to 100000, got %d", clamped)
	}
}

func TestDecodeInboundRequest_OpenAIVision(t *testing.T) {
	// Official OpenAI Vision format: array of content parts
	payload := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "What is in this image?"},
					{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."}}
				]
			}
		]
	}`)

	canReq, err := DecodeInboundRequest(payload)
	if err != nil {
		t.Fatalf("failed to decode OpenAI Vision payload: %v", err)
	}

	if len(canReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(canReq.Messages))
	}

	msg := canReq.Messages[0]
	if msg.Content != "What is in this image?" {
		t.Errorf("expected text content 'What is in this image?', got %q", msg.Content)
	}

	if len(msg.ContentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.ContentParts))
	}

	if msg.ContentParts[0].Type != "text" || msg.ContentParts[0].Text != "What is in this image?" {
		t.Errorf("unexpected part 0: %+v", msg.ContentParts[0])
	}

	if msg.ContentParts[1].Type != "image_url" || msg.ContentParts[1].ImageURL == nil || msg.ContentParts[1].ImageURL.URL != "data:image/jpeg;base64,/9j/4AAQSkZJRg..." {
		t.Errorf("unexpected part 1: %+v", msg.ContentParts[1])
	}

	// Verify encoding back to OpenAI JSON preserves array format
	encoded, err := EncodeCanonicalRequest(canReq)
	if err != nil {
		t.Fatalf("failed to re-encode request: %v", err)
	}

	var reReq OpenAIRequest
	if err := json.Unmarshal(encoded, &reReq); err != nil {
		t.Fatalf("failed to unmarshal re-encoded JSON: %v", err)
	}

	if len(reReq.Messages[0].Content.Parts) != 2 {
		t.Errorf("expected 2 parts in re-encoded message, got %d", len(reReq.Messages[0].Content.Parts))
	}
}

func TestDecodeInboundRequest_OllamaImagesFallback(t *testing.T) {
	// Ollama / Go vision fallback format: content string + images array
	payload := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{
				"role": "user",
				"content": "Extract all text from this image accurately...",
				"images": ["/9j/4AAQSkZJRg..."]
			}
		]
	}`)

	canReq, err := DecodeInboundRequest(payload)
	if err != nil {
		t.Fatalf("failed to decode Ollama images payload: %v", err)
	}

	msg := canReq.Messages[0]
	if msg.Content != "Extract all text from this image accurately..." {
		t.Errorf("unexpected content: %q", msg.Content)
	}

	if len(msg.ContentParts) != 1 {
		t.Fatalf("expected 1 image content part, got %d", len(msg.ContentParts))
	}

	if msg.ContentParts[0].ImageURL.URL != "data:image/jpeg;base64,/9j/4AAQSkZJRg..." {
		t.Errorf("expected data URI prefix added, got %q", msg.ContentParts[0].ImageURL.URL)
	}
}

func TestEncodeCanonicalRequest_MaxCompletionTokens(t *testing.T) {
	// Standard model (gpt-4o) should emit max_tokens
	stdReq := &canonical.CanonicalRequest{
		Model:     "gpt-4o",
		MaxTokens: 2048,
	}
	stdBytes, err := EncodeCanonicalRequest(stdReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(stdBytes), `"max_tokens":2048`) {
		t.Errorf("expected gpt-4o to have max_tokens, got: %s", string(stdBytes))
	}
	if strings.Contains(string(stdBytes), `"max_completion_tokens"`) {
		t.Errorf("did not expect max_completion_tokens for gpt-4o, got: %s", string(stdBytes))
	}

	// Reasoning model (gpt-5.5 / o1 / o3) MUST emit max_completion_tokens and omit max_tokens
	reasoningModels := []string{"gpt-5.5", "o1", "o3-mini"}
	for _, m := range reasoningModels {
		rReq := &canonical.CanonicalRequest{
			Model:     m,
			MaxTokens: 4096,
		}
		rBytes, err := EncodeCanonicalRequest(rReq)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", m, err)
		}
		if !strings.Contains(string(rBytes), `"max_completion_tokens":4096`) {
			t.Errorf("expected %s to have max_completion_tokens, got: %s", m, string(rBytes))
		}
		if strings.Contains(string(rBytes), `"max_tokens"`) && !strings.Contains(string(rBytes), `"max_completion_tokens"`) {
			t.Errorf("did not expect max_tokens for %s, got: %s", m, string(rBytes))
		}
	}
}
