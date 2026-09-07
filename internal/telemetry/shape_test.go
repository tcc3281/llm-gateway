package telemetry

import (
	"strings"
	"testing"
)

func TestInspectPayloadShape_OpenAIText(t *testing.T) {
	payload := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "You are a helpful coding assistant."},
			{"role": "user", "content": "Explain Go goroutines in simple words."}
		]
	}`)

	shape := InspectPayloadShape("openai", payload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}

	if shape.MessageCount != 2 {
		t.Errorf("expected MessageCount 2, got %d", shape.MessageCount)
	}
	if len(shape.Roles) != 2 || shape.Roles[0] != "system" || shape.Roles[1] != "user" {
		t.Errorf("unexpected roles: %v", shape.Roles)
	}
	if shape.SystemPromptChars != 35 {
		t.Errorf("expected SystemPromptChars 35, got %d", shape.SystemPromptChars)
	}
	if shape.HasImages || shape.HasAudio || shape.ToolsCount > 0 {
		t.Errorf("expected purely text shape, got: %+v", shape)
	}
	if shape.Modality != "text" {
		t.Errorf("expected modality 'text', got %s", shape.Modality)
	}
	if shape.StructureHash == "" {
		t.Error("expected non-empty StructureHash")
	}
}

func TestInspectPayloadShape_OpenAIVision(t *testing.T) {
	// Base64 image simulation
	b64Image := strings.Repeat("A", 1000)
	payload := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "What is in this image?"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,` + b64Image + `"}}
				]
			}
		]
	}`)

	shape := InspectPayloadShape("openai", payload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}

	if !shape.HasImages {
		t.Error("expected HasImages to be true")
	}
	if shape.ImageCount != 1 {
		t.Errorf("expected ImageCount 1, got %d", shape.ImageCount)
	}
	if shape.ImageTotalBytes <= 0 {
		t.Errorf("expected ImageTotalBytes > 0, got %d", shape.ImageTotalBytes)
	}
	if len(shape.ImageFormats) != 1 || shape.ImageFormats[0] != "image/png" {
		t.Errorf("expected format 'image/png', got %v", shape.ImageFormats)
	}
	if shape.Modality != "vision" {
		t.Errorf("expected modality 'vision', got %s", shape.Modality)
	}
}

func TestInspectPayloadShape_OpenAIAudio(t *testing.T) {
	b64Audio := strings.Repeat("B", 800)
	payload := []byte(`{
		"model": "gpt-4o-audio-preview",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Transcribe this sound"},
					{"type": "input_audio", "input_audio": {"data": "` + b64Audio + `", "format": "wav"}}
				]
			}
		]
	}`)

	shape := InspectPayloadShape("openai", payload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}

	if !shape.HasAudio {
		t.Error("expected HasAudio to be true")
	}
	if shape.AudioCount != 1 {
		t.Errorf("expected AudioCount 1, got %d", shape.AudioCount)
	}
	if shape.AudioTotalBytes <= 0 {
		t.Errorf("expected AudioTotalBytes > 0, got %d", shape.AudioTotalBytes)
	}
	if shape.Modality != "audio" {
		t.Errorf("expected modality 'audio', got %s", shape.Modality)
	}
}

func TestInspectPayloadShape_OpenAIToolsAndReasoning(t *testing.T) {
	payload := []byte(`{
		"model": "o3-mini",
		"reasoning_effort": "high",
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "fetch_weather",
					"parameters": {
						"type": "object",
						"properties": {
							"location": {"type": "string"},
							"unit": {"type": "string"}
						}
					}
				}
			}
		],
		"messages": [
			{
				"role": "assistant",
				"tool_calls": [
					{
						"function": {
							"name": "fetch_weather",
							"arguments": "{\"location\": \"Tokyo\", \"unit\": \"celsius\"}"
						}
					}
				]
			}
		]
	}`)

	shape := InspectPayloadShape("openai", payload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}

	if shape.ToolsCount != 1 {
		t.Errorf("expected ToolsCount 1, got %d", shape.ToolsCount)
	}
	if shape.ToolCallsCount != 1 {
		t.Errorf("expected ToolCallsCount 1, got %d", shape.ToolCallsCount)
	}
	if len(shape.ToolNames) != 1 || shape.ToolNames[0] != "fetch_weather" {
		t.Errorf("expected tool name 'fetch_weather', got %v", shape.ToolNames)
	}
	if !shape.HasReasoning || shape.ReasoningEffort != "high" {
		t.Errorf("expected reasoning effort 'high', got %v", shape.ReasoningEffort)
	}
	if shape.Modality != "tools" {
		t.Errorf("expected modality 'tools', got %s", shape.Modality)
	}
}

func TestInspectPayloadShape_AnthropicVisionAndThinking(t *testing.T) {
	payload := []byte(`{
		"model": "claude-3-7-sonnet-20250219",
		"system": "System instructions here",
		"thinking": {
			"type": "enabled",
			"budget_tokens": 2048
		},
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Analyze this chart"},
					{
						"type": "image",
						"source": {
							"type": "base64",
							"media_type": "image/jpeg",
							"data": "aW1hZ2VkYXRh"
						}
					}
				]
			}
		]
	}`)

	shape := InspectPayloadShape("anthropic", payload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}

	if !shape.HasImages || shape.ImageCount != 1 {
		t.Errorf("expected 1 image, got %d", shape.ImageCount)
	}
	if len(shape.ImageFormats) != 1 || shape.ImageFormats[0] != "image/jpeg" {
		t.Errorf("expected format 'image/jpeg', got %v", shape.ImageFormats)
	}
	if !shape.HasReasoning || shape.ReasoningEffort != "budget:2048" {
		t.Errorf("expected reasoning 'budget:2048', got %s", shape.ReasoningEffort)
	}
	if shape.SystemPromptChars <= 0 {
		t.Errorf("expected SystemPromptChars > 0, got %d", shape.SystemPromptChars)
	}
	if shape.Modality != "vision" {
		t.Errorf("expected modality 'vision', got %s", shape.Modality)
	}
}

func TestInspectPayloadShape_StructureHashDeterminism(t *testing.T) {
	payload1 := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hello world"}]}`)
	payload2 := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "Different text with same role and count"}]}`)

	shape1 := InspectPayloadShape("openai", payload1)
	shape2 := InspectPayloadShape("openai", payload2)

	// Structural fingerprint should be identical because roles and counts are identical!
	if shape1.StructureHash != shape2.StructureHash {
		t.Errorf("expected identical structure hash for same structural shape, got %s vs %s", shape1.StructureHash, shape2.StructureHash)
	}
}

func TestInspectPayloadShape_Hyperparameters(t *testing.T) {
	// OpenAI payload with full parameters
	openAIPayload := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.7,
		"top_p": 0.95,
		"max_tokens": 2048,
		"seed": 1234,
		"presence_penalty": 0.5,
		"frequency_penalty": -0.2,
		"stop": ["END", "STOP"]
	}`)

	shape := InspectPayloadShape("openai", openAIPayload)
	if shape == nil {
		t.Fatal("expected non-nil shape")
	}
	if shape.Temperature == nil || *shape.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", shape.Temperature)
	}
	if shape.TopP == nil || *shape.TopP != 0.95 {
		t.Errorf("expected top_p 0.95, got %v", shape.TopP)
	}
	if shape.MaxTokens == nil || *shape.MaxTokens != 2048 {
		t.Errorf("expected max_tokens 2048, got %v", shape.MaxTokens)
	}
	if shape.Seed == nil || *shape.Seed != 1234 {
		t.Errorf("expected seed 1234, got %v", shape.Seed)
	}
	if shape.PresencePenalty == nil || *shape.PresencePenalty != 0.5 {
		t.Errorf("expected presence_penalty 0.5, got %v", shape.PresencePenalty)
	}
	if len(shape.StopSequences) != 2 || shape.StopSequences[0] != "END" {
		t.Errorf("expected stop sequences ['END', 'STOP'], got %v", shape.StopSequences)
	}

	// Anthropic payload with top_k
	anthropicPayload := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.3,
		"top_k": 40,
		"max_tokens": 1024,
		"stop_sequences": ["\n\nHuman:"]
	}`)

	anthropicShape := InspectPayloadShape("anthropic", anthropicPayload)
	if anthropicShape == nil {
		t.Fatal("expected non-nil anthropic shape")
	}
	if anthropicShape.TopK == nil || *anthropicShape.TopK != 40 {
		t.Errorf("expected top_k 40, got %v", anthropicShape.TopK)
	}
	if anthropicShape.Temperature == nil || *anthropicShape.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %v", anthropicShape.Temperature)
	}
	if anthropicShape.MaxTokens == nil || *anthropicShape.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %v", anthropicShape.MaxTokens)
	}
}

