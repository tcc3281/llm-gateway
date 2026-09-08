package gemini

import (
	"encoding/json"
	"testing"

	"llm-gateway/internal/adapter/canonical"
)

func TestEncodeCanonicalRequest_Gemini3ThoughtSignature(t *testing.T) {
	canReq := &canonical.CanonicalRequest{
		Model: "gemini-3.1-flash-lite",
		Messages: []canonical.ChatMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role: "assistant",
				ToolCalls: []canonical.ToolCall{
					{
						ID:        "toolu_01",
						Name:      "default_api:Read",
						Arguments: `{"file_path":"README.md"}`,
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"content":"file contents"}`,
				ToolCallID: "toolu_01",
			},
		},
	}

	payload, err := EncodeCanonicalRequest(canReq)
	if err != nil {
		t.Fatalf("unexpected error encoding request: %v", err)
	}

	var geminiReq GeminiRequest
	if err := json.Unmarshal(payload, &geminiReq); err != nil {
		t.Fatalf("failed to unmarshal generated payload: %v", err)
	}

	if len(geminiReq.Contents) < 2 {
		t.Fatalf("expected at least 2 contents, got %d", len(geminiReq.Contents))
	}

	// Find the model turn with the functionCall
	var modelTurn *GeminiContent
	for _, c := range geminiReq.Contents {
		if c.Role == "model" {
			modelTurn = &c
			break
		}
	}

	if modelTurn == nil {
		t.Fatal("expected to find a model content turn")
	}

	if len(modelTurn.Parts) == 0 {
		t.Fatal("expected parts in model turn")
	}

	part := modelTurn.Parts[0]
	if part.FunctionCall == nil {
		t.Fatal("expected functionCall in model turn part")
	}

	if part.FunctionCall.Name != "default_api:Read" {
		t.Errorf("expected functionCall name default_api:Read, got %q", part.FunctionCall.Name)
	}

	if part.ThoughtSignature != GeminiDummyThoughtSignature {
		t.Errorf("expected dummy thought signature %q, got %q", GeminiDummyThoughtSignature, part.ThoughtSignature)
	}
}

func TestDecodeUpstreamResponse_PreservesThoughtSignature(t *testing.T) {
	rawJSON := []byte(`{
		"candidates": [
			{
				"content": {
					"parts": [
						{
							"functionCall": {
								"id": "call_123",
								"name": "default_api:Read",
								"args": {"path": "main.go"}
							},
							"thoughtSignature": "real_signature_xyz"
						}
					]
				},
				"finishReason": "STOP"
			}
		]
	}`)

	canResp, err := DecodeUpstreamResponse(rawJSON, "gemini-3.1-flash-lite")
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(canResp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(canResp.ToolCalls))
	}

	tc := canResp.ToolCalls[0]
	if tc.Name != "default_api:Read" {
		t.Errorf("expected default_api:Read, got %s", tc.Name)
	}

	if tc.ThoughtSignature != "real_signature_xyz" {
		t.Errorf("expected thought signature 'real_signature_xyz', got %q", tc.ThoughtSignature)
	}
}

func TestEncodeCanonicalRequest_NoTrailingModelTurn(t *testing.T) {
	// Scenario 1: conversation ending with assistant text turn
	canReq := &canonical.CanonicalRequest{
		Model: "gemini-3.5-flash-lite",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "I am ready to help."},
		},
	}

	payload, err := EncodeCanonicalRequest(canReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiReq GeminiRequest
	if err := json.Unmarshal(payload, &geminiReq); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(geminiReq.Contents) != 3 {
		t.Fatalf("expected 3 turns (user, model, synthetic user), got %d", len(geminiReq.Contents))
	}
	lastTurn := geminiReq.Contents[len(geminiReq.Contents)-1]
	if lastTurn.Role != "user" {
		t.Errorf("expected last turn to be user, got %s", lastTurn.Role)
	}
	if len(lastTurn.Parts) == 0 || lastTurn.Parts[0].Text != "Continue" {
		t.Errorf("expected last turn to have 'Continue' text part")
	}

	// Scenario 2: conversation ending with assistant tool call
	canReqTools := &canonical.CanonicalRequest{
		Model: "gemini-3.5-flash-lite",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Search project"},
			{
				Role: "assistant",
				ToolCalls: []canonical.ToolCall{
					{ID: "call_abc", Name: "search", Arguments: `{"q":"go"}`},
				},
			},
		},
	}

	payload2, err := EncodeCanonicalRequest(canReqTools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiReq2 GeminiRequest
	if err := json.Unmarshal(payload2, &geminiReq2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	lastTurn2 := geminiReq2.Contents[len(geminiReq2.Contents)-1]
	if lastTurn2.Role != "user" {
		t.Errorf("expected last turn to be user, got %s", lastTurn2.Role)
	}
	if len(lastTurn2.Parts) == 0 || lastTurn2.Parts[0].Text != "Continue" {
		t.Fatalf("expected last turn to have 'Continue' text part")
	}
}

func TestEncodeCanonicalRequest_PreservesEmbeddedThoughtSignature(t *testing.T) {
	canReq := &canonical.CanonicalRequest{
		Model: "gemini-3.5-flash-lite",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Run tool"},
			{
				Role: "assistant",
				ToolCalls: []canonical.ToolCall{
					{ID: "call_abc__thought__real_sig_12345", Name: "search", Arguments: `{"q":"test"}`},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_abc__thought__real_sig_12345",
				Content:    `{"output":"found"}`,
			},
		},
	}

	payload, err := EncodeCanonicalRequest(canReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiReq GeminiRequest
	if err := json.Unmarshal(payload, &geminiReq); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var modelTurn *GeminiContent
	for _, c := range geminiReq.Contents {
		if c.Role == "model" {
			modelTurn = &c
			break
		}
	}
	if modelTurn == nil {
		t.Fatal("expected model turn")
	}
	if modelTurn.Parts[0].FunctionCall.ID != "call_abc" {
		t.Errorf("expected raw ID 'call_abc', got %q", modelTurn.Parts[0].FunctionCall.ID)
	}
	if modelTurn.Parts[0].ThoughtSignature != "real_sig_12345" {
		t.Errorf("expected thought signature 'real_sig_12345', got %q", modelTurn.Parts[0].ThoughtSignature)
	}

	userTurn := geminiReq.Contents[len(geminiReq.Contents)-1]
	if userTurn.Parts[0].FunctionResponse.ID != "call_abc" {
		t.Errorf("expected response ID 'call_abc', got %q", userTurn.Parts[0].FunctionResponse.ID)
	}
	if userTurn.Parts[0].FunctionResponse.Name != "search" {
		t.Errorf("expected response name 'search', got %q", userTurn.Parts[0].FunctionResponse.Name)
	}
}

func TestEncodeCanonicalRequest_ToolResultFallback(t *testing.T) {
	// Tool message with unmatched tool_call_id should NOT be dropped
	canReq := &canonical.CanonicalRequest{
		Model: "gemini-3.5-flash-lite",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Hello"},
			{
				Role: "assistant",
				ToolCalls: []canonical.ToolCall{
					{ID: "call_1", Name: "my_tool", Arguments: `{}`},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "different_id_999", // mismatch
				Content:    `{"status":"done"}`,
			},
		},
	}

	payload, err := EncodeCanonicalRequest(canReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var geminiReq GeminiRequest
	if err := json.Unmarshal(payload, &geminiReq); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	lastTurn := geminiReq.Contents[len(geminiReq.Contents)-1]
	if lastTurn.Role != "user" {
		t.Errorf("expected last turn to be user, got %s", lastTurn.Role)
	}
	if len(lastTurn.Parts) == 0 || lastTurn.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected last turn to have FunctionResponse")
	}
	// Fallback should have picked up "my_tool"
	if lastTurn.Parts[0].FunctionResponse.Name != "my_tool" {
		t.Errorf("expected function response name 'my_tool', got %s", lastTurn.Parts[0].FunctionResponse.Name)
	}
}

