package anthropic

import (
	"encoding/json"
	"fmt"

	"llm-gateway/internal/adapter/canonical"
)

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Model        string             `json:"model"`
	Content      []AnthropicContent `json:"content"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
	Usage        AnthropicUsage     `json:"usage"`
}

type AnthropicContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// DecodeInboundRequest decodes Anthropic /v1/messages request body to CanonicalRequest
func DecodeInboundRequest(body []byte) (*canonical.CanonicalRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid anthropic payload: %w", err)
	}

	canReq := &canonical.CanonicalRequest{
		InboundFormat: "anthropic",
		Model:         req.Model,
		SystemPrompt:  req.System,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		Stream:        req.Stream,
	}

	if canReq.MaxTokens <= 0 {
		canReq.MaxTokens = 1024 // Default fallback for Anthropic
	}

	for _, msg := range req.Messages {
		canReq.Messages = append(canReq.Messages, canonical.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return canReq, nil
}

// EncodeCanonicalRequest encodes CanonicalRequest to Anthropic payload format
func EncodeCanonicalRequest(canReq *canonical.CanonicalRequest) ([]byte, error) {
	maxTokens := canReq.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	req := AnthropicRequest{
		Model:       canReq.Model,
		System:      canReq.SystemPrompt,
		MaxTokens:   maxTokens,
		Temperature: canReq.Temperature,
		TopP:        canReq.TopP,
		Stream:      canReq.Stream,
	}

	for _, msg := range canReq.Messages {
		req.Messages = append(req.Messages, AnthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return json.Marshal(req)
}

// DecodeUpstreamResponse decodes raw Anthropic response to CanonicalResponse
func DecodeUpstreamResponse(body []byte) (*canonical.CanonicalResponse, error) {
	var resp AnthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	content := ""
	for _, c := range resp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	canResp := &canonical.CanonicalResponse{
		ID:               resp.ID,
		Model:            resp.Model,
		Role:             resp.Role,
		Content:          content,
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		FinishReason:     resp.StopReason,
	}

	return canResp, nil
}

// FormatCanonicalToAnthropicResponse converts CanonicalResponse back to Anthropic format
func FormatCanonicalToAnthropicResponse(canResp *canonical.CanonicalResponse) ([]byte, error) {
	role := canResp.Role
	if role == "" {
		role = "assistant"
	}
	resp := AnthropicResponse{
		ID:    canResp.ID,
		Type:  "message",
		Role:  role,
		Model: canResp.Model,
		Content: []AnthropicContent{
			{
				Type: "text",
				Text: canResp.Content,
			},
		},
		StopReason: canResp.FinishReason,
		Usage: AnthropicUsage{
			InputTokens:  canResp.PromptTokens,
			OutputTokens: canResp.CompletionTokens,
		},
	}
	return json.Marshal(resp)
}

// FormatCanonicalToAnthropicSSE renders one content_block_delta event.
// The message_start / content_block_start / message_delta / message_stop
// lifecycle events are emitted by the gateway service so the full Anthropic
// streaming protocol is honored.
func FormatCanonicalToAnthropicSSE(chunk *canonical.CanonicalStreamChunk) (string, error) {
	if chunk.DeltaContent == "" {
		return "", nil
	}
	deltaPayload := map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]string{
			"type": "text_delta",
			"text": chunk.DeltaContent,
		},
	}
	data, err := json.Marshal(deltaPayload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", string(data)), nil
}

// DecodeStreamChunk decodes Anthropic SSE stream event to CanonicalStreamChunk
func DecodeStreamChunk(eventName string, eventData []byte, model string) (*canonical.CanonicalStreamChunk, error) {
	if eventName == "message_stop" {
		return &canonical.CanonicalStreamChunk{IsLast: true}, nil
	}

	chunk := &canonical.CanonicalStreamChunk{
		Model: model,
	}

	if eventName == "content_block_delta" {
		var payload struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil {
			chunk.DeltaContent = payload.Delta.Text
		}
		return chunk, nil
	}

	if eventName == "message_start" {
		var payload struct {
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil {
			chunk.PromptTokens = payload.Message.Usage.InputTokens
		}
		return chunk, nil
	}

	if eventName == "message_delta" {
		var payload struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil {
			chunk.CompletionTokens = payload.Usage.OutputTokens
			chunk.FinishReason = payload.Delta.StopReason
			if chunk.FinishReason != "" {
				chunk.IsLast = true
			}
		}
		return chunk, nil
	}

	return nil, nil
}

