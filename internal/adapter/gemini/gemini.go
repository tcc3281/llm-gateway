package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"llm-gateway/internal/adapter/canonical"

	"github.com/google/uuid"
)

type GeminiRequest struct {
	Contents          []GeminiContent       `json:"contents"`
	SystemInstruction *GeminiSystemContent `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenConfig      `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role"` // "user", "model"
	Parts []GeminiPart `json:"parts"`
}

type GeminiSystemContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiGenConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate   `json:"candidates"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// EncodeCanonicalRequest converts CanonicalRequest into Gemini API Request payload
func EncodeCanonicalRequest(canReq *canonical.CanonicalRequest) ([]byte, error) {
	req := GeminiRequest{
		GenerationConfig: &GeminiGenConfig{
			Temperature:     canReq.Temperature,
			TopP:            canReq.TopP,
			MaxOutputTokens: canReq.MaxTokens,
		},
	}

	if canReq.SystemPrompt != "" {
		req.SystemInstruction = &GeminiSystemContent{
			Parts: []GeminiPart{{Text: canReq.SystemPrompt}},
		}
	}

	for _, msg := range canReq.Messages {
		role := "user"
		if msg.Role == "assistant" || msg.Role == "model" {
			role = "model"
		}
		req.Contents = append(req.Contents, GeminiContent{
			Role:  role,
			Parts: []GeminiPart{{Text: msg.Content}},
		})
	}

	return json.Marshal(req)
}

// DecodeUpstreamResponse decodes raw Gemini response bytes to CanonicalResponse
func DecodeUpstreamResponse(body []byte, model string) (*canonical.CanonicalResponse, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	canResp := &canonical.CanonicalResponse{
		ID:    fmt.Sprintf("chatcmpl-gemini-%s", uuid.New().String()[:8]),
		Model: model,
		Role:  "assistant",
	}

	if len(resp.Candidates) > 0 {
		var textParts []string
		for _, p := range resp.Candidates[0].Content.Parts {
			textParts = append(textParts, p.Text)
		}
		canResp.Content = strings.Join(textParts, "")
		canResp.FinishReason = resp.Candidates[0].FinishReason
	}

	if resp.UsageMetadata != nil {
		canResp.PromptTokens = resp.UsageMetadata.PromptTokenCount
		canResp.CompletionTokens = resp.UsageMetadata.CandidatesTokenCount
		canResp.TotalTokens = resp.UsageMetadata.TotalTokenCount
	}

	return canResp, nil
}

// DecodeStreamChunk decodes Gemini Stream SSE chunk bytes into CanonicalStreamChunk
func DecodeStreamChunk(chunkBytes []byte, model string) (*canonical.CanonicalStreamChunk, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(chunkBytes, &resp); err != nil {
		return nil, err
	}

	chunk := &canonical.CanonicalStreamChunk{
		ID:    fmt.Sprintf("chatcmpl-gemini-%s", uuid.New().String()[:8]),
		Model: model,
		Role:  "assistant",
	}

	if len(resp.Candidates) > 0 {
		var textParts []string
		for _, p := range resp.Candidates[0].Content.Parts {
			textParts = append(textParts, p.Text)
		}
		chunk.DeltaContent = strings.Join(textParts, "")
		chunk.FinishReason = resp.Candidates[0].FinishReason
		if chunk.FinishReason == "STOP" {
			chunk.IsLast = true
		}
	}

	if resp.UsageMetadata != nil {
		chunk.PromptTokens = resp.UsageMetadata.PromptTokenCount
		chunk.CompletionTokens = resp.UsageMetadata.CandidatesTokenCount
	}

	return chunk, nil
}
