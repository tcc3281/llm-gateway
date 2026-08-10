package openai

import (
	"encoding/json"
	"fmt"
	"time"

	"llm-gateway/internal/adapter/canonical"
)

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Stop        interface{}     `json:"stop,omitempty"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   *OpenAIUsage         `json:"usage,omitempty"`
}

type OpenAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        OpenAIDeltaChoice `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type OpenAIDeltaChoice struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// DecodeInboundRequest decodes OpenAI /v1/chat/completions request bytes to CanonicalRequest
func DecodeInboundRequest(body []byte) (*canonical.CanonicalRequest, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid openai payload: %w", err)
	}

	canReq := &canonical.CanonicalRequest{
		InboundFormat: "openai",
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		Stream:        req.Stream,
	}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			canReq.SystemPrompt = msg.Content
		} else {
			canReq.Messages = append(canReq.Messages, canonical.ChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return canReq, nil
}

// EncodeCanonicalRequest converts CanonicalRequest to OpenAI Request payload
func EncodeCanonicalRequest(canReq *canonical.CanonicalRequest) ([]byte, error) {
	req := OpenAIRequest{
		Model:       canReq.Model,
		MaxTokens:   canReq.MaxTokens,
		Temperature: canReq.Temperature,
		TopP:        canReq.TopP,
		Stream:      canReq.Stream,
	}

	if canReq.SystemPrompt != "" {
		req.Messages = append(req.Messages, OpenAIMessage{
			Role:    "system",
			Content: canReq.SystemPrompt,
		})
	}

	for _, msg := range canReq.Messages {
		req.Messages = append(req.Messages, OpenAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return json.Marshal(req)
}

// DecodeUpstreamResponse converts raw OpenAI response bytes to CanonicalResponse
func DecodeUpstreamResponse(body []byte) (*canonical.CanonicalResponse, error) {
	var resp OpenAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	canResp := &canonical.CanonicalResponse{
		ID:               resp.ID,
		Model:            resp.Model,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	if len(resp.Choices) > 0 {
		canResp.Role = resp.Choices[0].Message.Role
		canResp.Content = resp.Choices[0].Message.Content
		canResp.FinishReason = resp.Choices[0].FinishReason
	}

	return canResp, nil
}

// FormatCanonicalToOpenAIResponse converts CanonicalResponse back to OpenAI response format
func FormatCanonicalToOpenAIResponse(canResp *canonical.CanonicalResponse) ([]byte, error) {
	resp := OpenAIResponse{
		ID:      canResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   canResp.Model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    canResp.Role,
					Content: canResp.Content,
				},
				FinishReason: canResp.FinishReason,
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     canResp.PromptTokens,
			CompletionTokens: canResp.CompletionTokens,
			TotalTokens:      canResp.TotalTokens,
		},
	}
	return json.Marshal(resp)
}

// DecodeStreamChunk parses a raw OpenAI SSE chunk (choices[].delta) into a
// CanonicalStreamChunk. It differs from DecodeUpstreamResponse because stream
// chunks carry content under "delta", not "message".
func DecodeStreamChunk(data []byte, fallbackModel string) (*canonical.CanonicalStreamChunk, error) {
	var chunk OpenAIStreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("failed to decode openai stream chunk: %w", err)
	}

	model := chunk.Model
	if model == "" {
		model = fallbackModel
	}
	c := &canonical.CanonicalStreamChunk{ID: chunk.ID, Model: model}
	if len(chunk.Choices) > 0 {
		c.DeltaContent = chunk.Choices[0].Delta.Content
		c.Role = chunk.Choices[0].Delta.Role
		if chunk.Choices[0].FinishReason != nil {
			c.FinishReason = *chunk.Choices[0].FinishReason
		}
	}
	if chunk.Usage != nil {
		c.PromptTokens = chunk.Usage.PromptTokens
		c.CompletionTokens = chunk.Usage.CompletionTokens
	}
	return c, nil
}

// FormatCanonicalToOpenAISSE converts CanonicalStreamChunk to OpenAI SSE string format
func FormatCanonicalToOpenAISSE(chunk *canonical.CanonicalStreamChunk) (string, error) {
	streamChunk := OpenAIStreamChunk{
		ID:      chunk.ID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   chunk.Model,
		Choices: []OpenAIStreamChoice{
			{
				Index: 0,
				Delta: OpenAIDeltaChoice{
					Role:    chunk.Role,
					Content: chunk.DeltaContent,
				},
			},
		},
	}

	if chunk.FinishReason != "" {
		fr := chunk.FinishReason
		streamChunk.Choices[0].FinishReason = &fr
	}

	data, err := json.Marshal(streamChunk)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("data: %s\n\n", string(data)), nil
}
