package canonical

import (
	"encoding/json"
	"testing"
)

func TestCanonicalRequestSerialization(t *testing.T) {
	temp := 0.5
	req := CanonicalRequest{
		InboundFormat: "openai",
		Model:         "gpt-4o",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Call tool hello"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:        "call_123",
						Name:      "get_weather",
						Arguments: `{"location":"Hanoi"}`,
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_123",
				Content:    `{"temperature": 28}`,
			},
		},
		Temperature: &temp,
		MaxTokens:   200,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CanonicalRequest: %v", err)
	}

	var decoded CanonicalRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CanonicalRequest: %v", err)
	}

	if decoded.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got %s", decoded.Model)
	}

	if len(decoded.Messages) != 4 {
		t.Fatalf("Expected 4 messages, got %d", len(decoded.Messages))
	}

	if len(decoded.Messages[2].ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call on message 2, got %d", len(decoded.Messages[2].ToolCalls))
	}

	if decoded.Messages[2].ToolCalls[0].Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got %s", decoded.Messages[2].ToolCalls[0].Name)
	}
}
