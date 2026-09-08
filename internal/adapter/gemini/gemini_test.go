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
