package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm-gateway/internal/adapter/canonical"
)

type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Stop            interface{}     `json:"stop,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	StreamOptions   *StreamOptions  `json:"stream_options,omitempty"`
	// Tools declares the callable functions; ToolChoice picks the invocation
	// policy. ToolChoice is raw so both the string form ("auto") and the object
	// form ({type:"function",function:{name}}) survive decode untouched.
	Tools      []OpenAITool    `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
}

// StreamOptions asks the upstream to include a usage block in the final stream
// chunk, so the gateway can meter prompt/completion/reasoning tokens for
// streamed responses.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// OpenAITool is the modern OpenAI tool declaration (type:"function").
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// OpenAIToolCall is an assistant turn's tool invocation. Arguments is a
// JSON-encoded string on the wire (kept as-is).
type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIMsgContent supports:
// 1. Plain string ("content": "Hello")
// 2. Array of content blocks ("content": [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {...}}])
// 3. Null ("content": null) for assistant tool calls.
type OpenAIMsgContent struct {
	String *string
	Parts  []OpenAIContentPart
}

type OpenAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *OpenAIImageURL `json:"image_url,omitempty"`
}

type OpenAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

func (c *OpenAIMsgContent) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		c.String = nil
		c.Parts = nil
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		c.String = &str
		c.Parts = nil
		return nil
	}
	var parts []OpenAIContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		c.String = nil
		c.Parts = parts
		return nil
	}
	return fmt.Errorf("openai content must be a string or array of content parts")
}

func (c OpenAIMsgContent) MarshalJSON() ([]byte, error) {
	if c.Parts != nil {
		return json.Marshal(c.Parts)
	}
	if c.String != nil {
		return json.Marshal(c.String)
	}
	return []byte("null"), nil
}

func (c OpenAIMsgContent) TextValue() string {
	if c.String != nil {
		return *c.String
	}
	var sb strings.Builder
	for _, p := range c.Parts {
		if p.Type == "text" && p.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

type OpenAIMessage struct {
	Role             string           `json:"role"`
	Name             string           `json:"name,omitempty"`
	Content          OpenAIMsgContent `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
	Images           []string         `json:"images,omitempty"` // Ollama / Go vision compatibility
}

// strPtr / strVal bridge plain strings to *string pointers.
func strPtr(s string) *string { return &s }

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
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
	Role             string                `json:"role,omitempty"`
	Content          string                `json:"content,omitempty"`
	ReasoningContent string                `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIDeltaToolCall `json:"tool_calls,omitempty"`
}

// OpenAIDeltaToolCall is one streaming tool-call fragment. Arguments is the
// incremental partial JSON string accumulated by the gateway service.
type OpenAIDeltaToolCall struct {
	Index    int                  `json:"index"`
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function *OpenAIDeltaFunction `json:"function,omitempty"`
}

type OpenAIDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// DecodeInboundRequest decodes OpenAI /v1/chat/completions request bytes to CanonicalRequest
func DecodeInboundRequest(body []byte) (*canonical.CanonicalRequest, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid openai payload: %w", err)
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 && req.MaxCompletionTokens > 0 {
		maxTokens = req.MaxCompletionTokens
	}

	canReq := &canonical.CanonicalRequest{
		InboundFormat:   "openai",
		Model:           req.Model,
		MaxTokens:       maxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
		ReasoningEffort: req.ReasoningEffort,
		ThinkingEnabled: req.ReasoningEffort != "",
		Tools:           decodeOpenAITools(req.Tools),
		ToolChoice:      decodeOpenAIToolChoice(req.ToolChoice),
	}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			canReq.SystemPrompt = msg.Content.TextValue()
			continue
		}
		role := msg.Role
		if role == "function" {
			role = "tool"
		}
		cm := canonical.ChatMessage{
			Role:       role,
			Name:       msg.Name,
			Content:    msg.Content.TextValue(),
			ToolCallID: msg.ToolCallID,
		}
		for _, p := range msg.Content.Parts {
			cp := canonical.ContentPart{
				Type: p.Type,
				Text: p.Text,
			}
			if p.ImageURL != nil {
				cp.ImageURL = &canonical.ImageURL{
					URL:    p.ImageURL.URL,
					Detail: p.ImageURL.Detail,
				}
			}
			cm.ContentParts = append(cm.ContentParts, cp)
		}
		for _, img := range msg.Images {
			url := img
			if !strings.HasPrefix(url, "data:") && !strings.HasPrefix(url, "http") {
				url = "data:image/jpeg;base64," + img
			}
			cm.ContentParts = append(cm.ContentParts, canonical.ContentPart{
				Type: "image_url",
				ImageURL: &canonical.ImageURL{
					URL: url,
				},
			})
		}
		for _, tc := range msg.ToolCalls {
			tcID := tc.ID
			sig := ""
			if idx := strings.Index(tcID, "__thought__"); idx != -1 {
				sig = tcID[idx+len("__thought__"):]
			}
			cm.ToolCalls = append(cm.ToolCalls, canonical.ToolCall{
				ID:               tcID,
				Name:             tc.Function.Name,
				Arguments:        tc.Function.Arguments,
				ThoughtSignature: sig,
			})
		}
		canReq.Messages = append(canReq.Messages, cm)
	}

	return canReq, nil
}

// decodeOpenAITools maps wire tools to canonical definitions, dropping any
// non-function tool types (OpenAI currently only defines "function").
func decodeOpenAITools(tools []OpenAITool) []canonical.ToolDefinition {
	out := make([]canonical.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		out = append(out, canonical.ToolDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	return out
}

// decodeOpenAIToolChoice accepts both the string form ("auto"/"none"/"required")
// and the object form ({type:"function",function:{name}}).
func decodeOpenAIToolChoice(raw json.RawMessage) *canonical.ToolChoice {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "none", "required":
			return &canonical.ToolChoice{Mode: s}
		}
		return nil
	}
	var obj struct {
		Type     string `json:"type"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type == "function" && obj.Function != nil {
		return &canonical.ToolChoice{Mode: "function", FunctionName: obj.Function.Name}
	}
	return nil
}

// encodeOpenAIToolChoice renders a canonical tool choice back to the OpenAI
// wire shape. Canonical "any"/"tool" (from Anthropic inbound) map to OpenAI's
// closest equivalents "required"/"function".
func encodeOpenAIToolChoice(tc *canonical.ToolChoice) json.RawMessage {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case "auto", "none", "required":
		b, _ := json.Marshal(tc.Mode)
		return b
	case "any":
		b, _ := json.Marshal("required")
		return b
	case "function", "tool":
		b, _ := json.Marshal(map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": tc.FunctionName},
		})
		return b
	}
	return nil
}

// clampMaxTokens ensures requested max_tokens does not exceed provider limits
func clampMaxTokens(model string, maxTokens int) int {
	if maxTokens <= 0 {
		return maxTokens
	}
	m := strings.ToLower(model)
	if strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4") || strings.Contains(m, "gpt-5") {
		if maxTokens > 100000 {
			return 100000
		}
		return maxTokens
	}
	if strings.Contains(m, "gpt-4o") {
		if maxTokens > 16384 {
			return 16384
		}
		return maxTokens
	}
	if strings.Contains(m, "gpt-4") || strings.Contains(m, "gpt-3.5") {
		if maxTokens > 4096 {
			return 4096
		}
		return maxTokens
	}
	return maxTokens
}

// isReasoningOrNewOpenAIModel detects models that reject 'max_tokens' in favor of 'max_completion_tokens'.
func isReasoningOrNewOpenAIModel(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.Contains(m, "gpt-5") ||
		strings.Contains(m, "-o1") ||
		strings.Contains(m, "-o3")
}

// EncodeCanonicalRequest converts CanonicalRequest to OpenAI Request payload
func EncodeCanonicalRequest(canReq *canonical.CanonicalRequest) ([]byte, error) {
	tokens := clampMaxTokens(canReq.Model, canReq.MaxTokens)
	req := OpenAIRequest{
		Model:           canReq.Model,
		Stream:          canReq.Stream,
		ReasoningEffort: canReq.ReasoningEffort,
		ToolChoice:      encodeOpenAIToolChoice(canReq.ToolChoice),
	}

	if isReasoningOrNewOpenAIModel(canReq.Model) {
		req.MaxCompletionTokens = tokens
		m := strings.ToLower(canReq.Model)
		// Early o1 models (o1-preview, o1-mini) reject custom temperature & top_p
		if !strings.Contains(m, "o1-mini") && !strings.Contains(m, "o1-preview") {
			req.Temperature = canReq.Temperature
			req.TopP = canReq.TopP
		}
	} else {
		req.MaxTokens = tokens
		req.Temperature = canReq.Temperature
		req.TopP = canReq.TopP
	}

	for _, t := range canReq.Tools {
		req.Tools = append(req.Tools, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	// For streamed responses ask the upstream to attach usage to the final
	// chunk so the gateway can meter prompt/completion/reasoning tokens.
	if canReq.Stream {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	if canReq.SystemPrompt != "" {
		req.Messages = append(req.Messages, OpenAIMessage{
			Role:    "system",
			Content: OpenAIMsgContent{String: strPtr(canReq.SystemPrompt)},
		})
	}

	for _, msg := range canReq.Messages {
		om := OpenAIMessage{
			Role:       msg.Role,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ContentParts) > 0 {
			var parts []OpenAIContentPart
			for _, cp := range msg.ContentParts {
				part := OpenAIContentPart{
					Type: cp.Type,
					Text: cp.Text,
				}
				if cp.ImageURL != nil {
					part.ImageURL = &OpenAIImageURL{
						URL:    cp.ImageURL.URL,
						Detail: cp.ImageURL.Detail,
					}
				}
				parts = append(parts, part)
			}
			om.Content = OpenAIMsgContent{Parts: parts}
		} else if msg.Content != "" || len(msg.ToolCalls) == 0 {
			om.Content = OpenAIMsgContent{String: strPtr(msg.Content)}
		}
		for _, tc := range msg.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, OpenAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		req.Messages = append(req.Messages, om)
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

	if resp.Usage.PromptTokensDetails != nil {
		canResp.CachedInputTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}
	if resp.Usage.CompletionTokensDetails != nil {
		canResp.ReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
	}

	if len(resp.Choices) > 0 {
		canResp.Role = resp.Choices[0].Message.Role
		canResp.Content = resp.Choices[0].Message.Content.TextValue()
		canResp.ReasoningContent = resp.Choices[0].Message.ReasoningContent
		canResp.FinishReason = resp.Choices[0].FinishReason
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			canResp.ToolCalls = append(canResp.ToolCalls, canonical.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	return canResp, nil
}

// FormatCanonicalToOpenAIResponse converts CanonicalResponse back to OpenAI response format
func FormatCanonicalToOpenAIResponse(canResp *canonical.CanonicalResponse) ([]byte, error) {
	msg := OpenAIMessage{
		Role:             canResp.Role,
		ReasoningContent: canResp.ReasoningContent,
	}
	// Tool-call-only assistant messages use content:null; otherwise carry the
	// text (even when empty).
	if canResp.Content != "" || len(canResp.ToolCalls) == 0 {
		msg.Content = OpenAIMsgContent{String: strPtr(canResp.Content)}
	}
	for _, tc := range canResp.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: OpenAIFunctionCall{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		})
	}
	finishReason := mapToOpenAIFinishReason(canResp.FinishReason)
	if len(canResp.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	resp := OpenAIResponse{
		ID:      canResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   canResp.Model,
		Choices: []OpenAIChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
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
		c.DeltaReasoning = chunk.Choices[0].Delta.ReasoningContent
		c.Role = chunk.Choices[0].Delta.Role
		if chunk.Choices[0].FinishReason != nil {
			c.FinishReason = *chunk.Choices[0].FinishReason
		}
		for _, dtc := range chunk.Choices[0].Delta.ToolCalls {
			d := canonical.ToolCallDelta{Index: dtc.Index, ID: dtc.ID}
			if dtc.Function != nil {
				d.Name = dtc.Function.Name
				d.Arguments = dtc.Function.Arguments
			}
			c.DeltaToolCalls = append(c.DeltaToolCalls, d)
		}
	}
	if chunk.Usage != nil {
		c.PromptTokens = chunk.Usage.PromptTokens
		c.CompletionTokens = chunk.Usage.CompletionTokens
		if chunk.Usage.PromptTokensDetails != nil {
			c.CachedInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if chunk.Usage.CompletionTokensDetails != nil {
			c.ReasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
		}
	}
	return c, nil
}

// FormatCanonicalToOpenAISSEBytes converts CanonicalStreamChunk directly to OpenAI SSE byte format,
// eliminating intermediate string conversions and re-allocations on the streaming hot path.
func FormatCanonicalToOpenAISSEBytes(chunk *canonical.CanonicalStreamChunk) ([]byte, error) {
	streamChunk := OpenAIStreamChunk{
		ID:      chunk.ID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   chunk.Model,
		Choices: []OpenAIStreamChoice{
			{
				Index: 0,
				Delta: OpenAIDeltaChoice{
					Role:             chunk.Role,
					Content:          chunk.DeltaContent,
					ReasoningContent: chunk.DeltaReasoning,
				},
			},
		},
	}

	if chunk.FinishReason != "" {
		fr := mapToOpenAIFinishReason(chunk.FinishReason)
		streamChunk.Choices[0].FinishReason = &fr
	}

	data, err := json.Marshal(streamChunk)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, len(data)+8)
	out = append(out, "data: "...)
	out = append(out, data...)
	out = append(out, "\n\n"...)
	return out, nil
}

// FormatCanonicalToOpenAISSE converts CanonicalStreamChunk to OpenAI SSE string format
func FormatCanonicalToOpenAISSE(chunk *canonical.CanonicalStreamChunk) (string, error) {
	b, err := FormatCanonicalToOpenAISSEBytes(chunk)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FormatOpenAIToolCallSSE renders reconstructed assistant tool calls as two
// OpenAI SSE chunks: one carrying role + all complete tool_calls, then a
// terminal chunk with finish_reason "tool_calls". The gateway service calls
// this at the end of a stream after accumulating the incremental arguments.
func FormatOpenAIToolCallSSE(toolCalls []canonical.ToolCall, model, id string, created int64) (string, error) {
	deltaTools := make([]OpenAIDeltaToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		fn := OpenAIDeltaFunction{Name: tc.Name, Arguments: tc.Arguments}
		deltaTools = append(deltaTools, OpenAIDeltaToolCall{
			Index:    i,
			ID:       tc.ID,
			Type:     "function",
			Function: &fn,
		})
	}

	mk := func(delta OpenAIDeltaChoice) OpenAIStreamChunk {
		return OpenAIStreamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []OpenAIStreamChoice{{Index: 0, Delta: delta}},
		}
	}

	chunks := []OpenAIStreamChunk{
		mk(OpenAIDeltaChoice{Role: "assistant", ToolCalls: deltaTools}),
	}
	fr := "tool_calls"
	chunks = append(chunks, mk(OpenAIDeltaChoice{}))
	chunks[1].Choices[0].FinishReason = &fr

	var sb strings.Builder
	for _, c := range chunks {
		data, err := json.Marshal(c)
		if err != nil {
			return "", err
		}
		sb.WriteString("data: " + string(data) + "\n\n")
	}
	return sb.String(), nil
}

// mapToOpenAIFinishReason translates provider-specific finish reasons (such as
// Gemini's "STOP", "MAX_TOKENS" or Anthropic's "end_turn") into standard OpenAI
// vocabulary ("stop", "length", "tool_calls", "content_filter").
func mapToOpenAIFinishReason(raw string) string {
	switch strings.ToLower(raw) {
	case "stop", "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens", "length":
		return "length"
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "content_filter", "safety", "recitation":
		return "content_filter"
	default:
		return raw
	}
}

