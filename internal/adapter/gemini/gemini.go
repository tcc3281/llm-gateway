package gemini

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"llm-gateway/internal/adapter/canonical"

	"github.com/google/uuid"
)

type GeminiRequest struct {
	Contents          []GeminiContent      `json:"contents"`
	SystemInstruction *GeminiSystemContent `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenConfig     `json:"generationConfig,omitempty"`
	// Tools declares callable functions; ToolConfig carries the calling policy
	// (the functional equivalent of OpenAI tool_choice / Anthropic tool_choice).
	Tools      []GeminiTool      `json:"tools,omitempty"`
	ToolConfig *GeminiToolConfig `json:"toolConfig,omitempty"`
}

// GeminiTool wraps the functionDeclarations list. Gemini's only tool type is
// the function declaration.
type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiFunctionCallingConfig is Gemini's tool_choice. Mode is "AUTO" | "ANY" |
// "NONE"; AllowedFunctionNames restricts which tools the model may call.
type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role"` // "user", "model"
	Parts []GeminiPart `json:"parts"`
}

type GeminiSystemContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiFunctionCall is a model-requested tool invocation. Args carries the
// argument object (not a string, unlike the OpenAI wire form).
type GeminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// GeminiFunctionResponse is a user-supplied tool result. Gemini matches
// responses to calls by name (it has no tool_call_id), so the encoder must
// recover the name from the preceding assistant tool call.
type GeminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

type GeminiGenConfig struct {
	Temperature     *float64              `json:"temperature,omitempty"`
	TopP            *float64              `json:"topP,omitempty"`
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// GeminiThinkingConfig asks Gemini to surface its internal reasoning as
// thought parts (thought:true) interleaved with the visible text.
type GeminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates"`
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

	if canReq.ThinkingEnabled {
		req.GenerationConfig.ThinkingConfig = &GeminiThinkingConfig{
			IncludeThoughts: true,
		}
	}

	if canReq.SystemPrompt != "" {
		req.SystemInstruction = &GeminiSystemContent{
			Parts: []GeminiPart{{Text: canReq.SystemPrompt}},
		}
	}

	for _, t := range canReq.Tools {
		req.Tools = append(req.Tools, GeminiTool{
			FunctionDeclarations: []GeminiFunctionDeclaration{
				{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  sanitizeGeminiSchema(t.Parameters),
				},
			},
		})
	}
	req.ToolConfig = encodeGeminiToolConfig(canReq.ToolChoice)

	for _, msg := range canReq.Messages {
		role := "user"
		if msg.Role == "assistant" || msg.Role == "model" {
			role = "model"
		}
		content := GeminiContent{Role: role}
		switch msg.Role {
		case "tool":
			// Canonical "tool" role → user content with a functionResponse part.
			// Gemini has no tool_call_id, so the function name is recovered by
			// scanning backward for the assistant call with the matching ID.
			name := findToolCallName(canReq.Messages, msg.ToolCallID)
			if name == "" {
				log.Printf("gemini encoder: no assistant tool call found for tool_call_id %q; dropping tool result", msg.ToolCallID)
				continue
			}
			respJSON, _ := json.Marshal(map[string]string{"result": msg.Content})
			content.Parts = append(content.Parts, GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{Name: name, Response: respJSON},
			})
		case "assistant", "model":
			if msg.Content != "" {
				content.Parts = append(content.Parts, GeminiPart{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args json.RawMessage
				if len(tc.Arguments) > 0 {
					args = json.RawMessage(tc.Arguments)
				}
				content.Parts = append(content.Parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{Name: tc.Name, Args: args},
				})
			}
		default:
			// Skip empty user turns — an empty text part marshals to {} which
			// Gemini rejects ("required oneof field 'data'").
			if msg.Content != "" {
				content.Parts = append(content.Parts, GeminiPart{Text: msg.Content})
			}
		}
		// Gemini rejects consecutive turns of the same role; merge parts into the
		// previous content when roles match (common for multiple tool results).
		if len(req.Contents) > 0 && req.Contents[len(req.Contents)-1].Role == content.Role {
			req.Contents[len(req.Contents)-1].Parts = append(req.Contents[len(req.Contents)-1].Parts, content.Parts...)
			continue
		}
		req.Contents = append(req.Contents, content)
	}

	return json.Marshal(req)
}

// encodeGeminiToolConfig maps a canonical tool choice to Gemini's
// functionCallingConfig. auto→AUTO, none→NONE, required/any→ANY, and an
// explicit function name → AUTO restricted to that name.
func encodeGeminiToolConfig(tc *canonical.ToolChoice) *GeminiToolConfig {
	if tc == nil {
		return nil
	}
	cfg := &GeminiToolConfig{}
	switch tc.Mode {
	case "auto", "":
		cfg.FunctionCallingConfig.Mode = "AUTO"
	case "none":
		cfg.FunctionCallingConfig.Mode = "NONE"
	case "required", "any":
		cfg.FunctionCallingConfig.Mode = "ANY"
	case "tool", "function":
		cfg.FunctionCallingConfig.Mode = "AUTO"
		if tc.FunctionName != "" {
			cfg.FunctionCallingConfig.AllowedFunctionNames = []string{tc.FunctionName}
		}
	}
	return cfg
}

// sanitizeGeminiSchema reduces a tool's parameter schema to the JSON-schema
// subset Gemini's Schema proto actually accepts, recursively. Anthropic/OpenAI
// clients (notably Claude Code) emit rich draft-07 schemas ("$schema",
// additionalProperties, const, propertyNames, exclusiveMinimum, ...) which
// Gemini rejects with "Unknown name ... Cannot find field" if forwarded
// verbatim. A whitelist — rather than a blacklist — is future-proof: anything
// Gemini cannot represent is dropped, never forwarded.
func sanitizeGeminiSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	cleaned := sanitizeSchemaValue(v)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return out
}

// geminiSchemaKeys are the fields Gemini's Schema proto supports. Everything
// else in a parameter schema is dropped.
var geminiSchemaKeys = map[string]bool{
	"type":          true,
	"format":        true,
	"description":   true,
	"nullable":      true,
	"enum":          true,
	"items":         true,
	"properties":    true,
	"required":      true,
	"minItems":      true,
	"maxItems":      true,
	"minProperties": true,
	"maxProperties": true,
	"minLength":     true,
	"maxLength":     true,
	"pattern":       true,
	"minimum":       true,
	"maximum":       true,
	"anyOf":         true,
}

func sanitizeSchemaValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			if !geminiSchemaKeys[k] {
				continue
			}
			out[k] = sanitizeSchemaValue(val)
		}
		// Gemini rejects `required` names that do not appear in the sibling
		// `properties` map ("required[1]: property is not defined"). This
		// happens when a property's definition was dropped by the whitelist
		// (e.g. it was a $ref into dropped $defs). Filter to present names and
		// drop `required` entirely when there is no properties map.
		if req, ok := out["required"].([]interface{}); ok {
			props, hasProps := out["properties"].(map[string]interface{})
			if !hasProps {
				delete(out, "required")
			} else {
				var kept []interface{}
				for _, name := range req {
					if s, isStr := name.(string); isStr {
						if _, exists := props[s]; exists {
							kept = append(kept, name)
						}
					}
				}
				if len(kept) == 0 {
					delete(out, "required")
				} else {
					out["required"] = kept
				}
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = sanitizeSchemaValue(item)
		}
		return out
	default:
		return v
	}
}

// findToolCallName scans the canonical messages backward for an assistant tool
// call whose ID matches toolCallID, returning its function name.
func findToolCallName(messages []canonical.ChatMessage, toolCallID string) string {
	if toolCallID == "" {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		for _, tc := range messages[i].ToolCalls {
			if tc.ID == toolCallID {
				return tc.Name
			}
		}
	}
	return ""
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
		var thoughtParts []string
		var toolCalls []canonical.ToolCall
		for i, p := range resp.Candidates[0].Content.Parts {
			switch {
			case p.FunctionCall != nil:
				// Gemini has no tool_call_id; synthesize "call_<n>" for the
				// canonical/OpenAI wire. Encode-to-Gemini ignores the id and
				// matches by name, so the synthesized value round-trips safely.
				args, err := json.Marshal(p.FunctionCall.Args)
				if err != nil {
					args = []byte("{}")
				}
				toolCalls = append(toolCalls, canonical.ToolCall{
					ID:        fmt.Sprintf("call_%d", i),
					Name:      p.FunctionCall.Name,
					Arguments: string(args),
				})
			case p.Thought:
				thoughtParts = append(thoughtParts, p.Text)
			default:
				textParts = append(textParts, p.Text)
			}
		}
		canResp.Content = strings.Join(textParts, "")
		canResp.ReasoningContent = strings.Join(thoughtParts, "")
		canResp.ToolCalls = toolCalls
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
		var thoughtParts []string
		for i, p := range resp.Candidates[0].Content.Parts {
			switch {
			case p.FunctionCall != nil:
				// Gemini functionCall parts arrive atomically (full args object
				// in one chunk). The gateway service keys Gemini deltas by name
				// ("g:"+name) rather than index, since part position is not
				// stable across chunks.
				args, err := json.Marshal(p.FunctionCall.Args)
				if err != nil {
					args = []byte("{}")
				}
				chunk.DeltaToolCalls = append(chunk.DeltaToolCalls, canonical.ToolCallDelta{
					Index:     i,
					ID:        fmt.Sprintf("call_%d", i),
					Name:      p.FunctionCall.Name,
					Arguments: string(args),
				})
			case p.Thought:
				thoughtParts = append(thoughtParts, p.Text)
			default:
				textParts = append(textParts, p.Text)
			}
		}
		chunk.DeltaContent = strings.Join(textParts, "")
		chunk.DeltaReasoning = strings.Join(thoughtParts, "")
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
