package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	"llm-gateway/internal/adapter/canonical"
)

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      *AnthropicSystem   `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Thinking    *AnthropicThinking `json:"thinking,omitempty"`
	// Tools declares the callable functions; ToolChoice picks the invocation
	// policy. ToolChoice is raw so both the string form ("auto"/"any") and the
	// object form ({type:"tool",name} / {type:"none"}) survive decode untouched.
	Tools      []AnthropicTool `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
}

// AnthropicTool is a callable function declaration. InputSchema is the JSON
// schema of the tool's input (Anthropic's "input_schema" wire field).
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// AnthropicSystem is the "system" request field. Anthropic accepts either a
// plain string ("system":"...") or an array of content blocks ("system":
// [{type:"text",text:"..."}]). Claude Code sends the array form (with
// cache_control on the last block), so the decoder must accept both. The
// gateway normalizes to a plain string for canonical and upstream encoding.
type AnthropicSystem struct {
	String string
}

func (s *AnthropicSystem) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.String = str
		return nil
	}
	var blocks []AnthropicContent
	if err := json.Unmarshal(data, &blocks); err != nil {
		return fmt.Errorf("system must be a string or an array of content blocks")
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	s.String = sb.String()
	return nil
}

func (s AnthropicSystem) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String)
}

// anthropicSystemString dereferences the optional system field; nil → "".
func anthropicSystemString(s *AnthropicSystem) string {
	if s == nil {
		return ""
	}
	return s.String
}

// anthropicSystemPtr wraps a non-empty string as the optional system field;
// empty → nil so the field is omitted on the upstream wire.
func anthropicSystemPtr(s string) *AnthropicSystem {
	if s == "" {
		return nil
	}
	return &AnthropicSystem{String: s}
}

// AnthropicThinking requests extended thinking on the upstream. The gateway
// derives budget_tokens from the canonical ThinkingEnabled flag; a client-supplied
// thinking block only needs type:"enabled" to be detected here.
type AnthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// AnthropicMessage.Content is a union: the plain string form ("Hello") used by
// simple turns, or the array form ([]AnthropicContent) used once a message
// carries content blocks (text + tool_use + tool_result). The custom
// UnmarshalJSON/MarshalJSON switch on the wire type so both shapes round-trip.
type AnthropicMessage struct {
	Role    string              `json:"role"`
	Content AnthropicMsgContent `json:"content"`
}

// AnthropicMsgContent holds either a plain string or a list of content blocks.
// Blocks == nil means the string form; Blocks != nil means the array form.
type AnthropicMsgContent struct {
	String string
	Blocks []AnthropicContent
}

func (c *AnthropicMsgContent) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.String = s
		c.Blocks = nil
		return nil
	}
	var blocks []AnthropicContent
	if err := json.Unmarshal(data, &blocks); err != nil {
		return fmt.Errorf("anthropic content must be a string or array of blocks: %w", err)
	}
	c.String = ""
	c.Blocks = blocks
	return nil
}

func (c AnthropicMsgContent) MarshalJSON() ([]byte, error) {
	if c.Blocks != nil {
		return json.Marshal(c.Blocks)
	}
	return json.Marshal(c.String)
}

// HasBlocks reports whether the content uses the array-of-blocks form.
func (c AnthropicMsgContent) HasBlocks() bool { return c.Blocks != nil }

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
	Type      string `json:"type"` // "text" | "thinking" | "redacted_thinking" | "tool_use" | "tool_result"
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	// tool_use block fields
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result block fields
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or text blocks
	IsError   bool            `json:"is_error,omitempty"`
}

type AnthropicUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

// DecodeInboundRequest decodes Anthropic /v1/messages request body to CanonicalRequest
func DecodeInboundRequest(body []byte) (*canonical.CanonicalRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid anthropic payload: %w", err)
	}

	canReq := &canonical.CanonicalRequest{
		InboundFormat:   "anthropic",
		Model:           req.Model,
		SystemPrompt:    anthropicSystemString(req.System),
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
		ThinkingEnabled: req.Thinking != nil && req.Thinking.Type == "enabled",
		Tools:           decodeAnthropicTools(req.Tools),
		ToolChoice:      decodeAnthropicToolChoice(req.ToolChoice),
	}

	if canReq.MaxTokens <= 0 {
		canReq.MaxTokens = 1024 // Default fallback for Anthropic
	}

	for _, msg := range req.Messages {
		cm := canonical.ChatMessage{Role: msg.Role}
		if !msg.Content.HasBlocks() {
			cm.Content = msg.Content.String
			canReq.Messages = append(canReq.Messages, cm)
			continue
		}
		for _, block := range msg.Content.Blocks {
			switch block.Type {
			case "text":
				cm.Content += block.Text
			case "tool_use":
				args, err := json.Marshal(block.Input)
				if err != nil {
					args = []byte("{}")
				}
				cm.ToolCalls = append(cm.ToolCalls, canonical.ToolCall{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: string(args),
				})
			case "tool_result":
				// A tool_result block belongs to a user message but maps to the
				// canonical "tool" role (OpenAI convention). Emit as its own
				// canonical message.
				canReq.Messages = append(canReq.Messages, canonical.ChatMessage{
					Role:       "tool",
					Content:    anthropicContentToText(block.Content),
					ToolCallID: block.ToolUseID,
				})
			}
		}
		// A message whose blocks were entirely tool_result (already emitted as
		// canonical "tool" messages) leaves cm empty — skip it, otherwise an
		// empty user message with no parts reaches the upstream.
		if cm.Content != "" || len(cm.ToolCalls) > 0 {
			canReq.Messages = append(canReq.Messages, cm)
		}
	}

	return canReq, nil
}

// decodeAnthropicTools maps wire tool declarations to canonical definitions.
func decodeAnthropicTools(tools []AnthropicTool) []canonical.ToolDefinition {
	out := make([]canonical.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		out = append(out, canonical.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return out
}

// decodeAnthropicToolChoice accepts both the string form ("auto"/"any") and the
// object form ({type:"tool",name} / {type:"none"}).
func decodeAnthropicToolChoice(raw json.RawMessage) *canonical.ToolChoice {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "any":
			return &canonical.ToolChoice{Mode: s}
		}
		return nil
	}
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		switch obj.Type {
		case "none":
			return &canonical.ToolChoice{Mode: "none"}
		case "tool":
			return &canonical.ToolChoice{Mode: "tool", FunctionName: obj.Name}
		case "auto", "any":
			// Anthropic also accepts the object form {"type":"any"} / {"type":"auto"}.
			return &canonical.ToolChoice{Mode: obj.Type}
		}
	}
	return nil
}

// encodeAnthropicToolChoice renders a canonical tool choice back to the
// Anthropic wire shape. Canonical "required" (from OpenAI inbound) maps to
// Anthropic's "any"; "function" maps to the {type:"tool"} object form.
func encodeAnthropicToolChoice(tc *canonical.ToolChoice) json.RawMessage {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case "auto", "any":
		b, _ := json.Marshal(tc.Mode)
		return b
	case "none":
		b, _ := json.Marshal(map[string]string{"type": "none"})
		return b
	case "required":
		b, _ := json.Marshal("any")
		return b
	case "tool", "function":
		b, _ := json.Marshal(map[string]string{"type": "tool", "name": tc.FunctionName})
		return b
	}
	return nil
}

// anthropicContentToText flattens a tool_result "content" field — which may be
// a JSON string or an array of text blocks — into plain text.
func anthropicContentToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []AnthropicContent
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return string(raw)
}

// EncodeCanonicalRequest encodes CanonicalRequest to Anthropic payload format
func EncodeCanonicalRequest(canReq *canonical.CanonicalRequest) ([]byte, error) {
	maxTokens := canReq.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	req := AnthropicRequest{
		Model:       canReq.Model,
		System:      anthropicSystemPtr(canReq.SystemPrompt),
		MaxTokens:   maxTokens,
		Temperature: canReq.Temperature,
		TopP:        canReq.TopP,
		Stream:      canReq.Stream,
	}

	if canReq.ThinkingEnabled {
		req.Thinking = &AnthropicThinking{
			Type:         "enabled",
			BudgetTokens: thinkingBudget(maxTokens, canReq.ReasoningEffort),
		}
	}

	for _, t := range canReq.Tools {
		req.Tools = append(req.Tools, AnthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	req.ToolChoice = encodeAnthropicToolChoice(canReq.ToolChoice)

	for _, msg := range canReq.Messages {
		am := AnthropicMessage{Role: msg.Role}
		switch msg.Role {
		case "assistant":
			// Assistant tool-call turns render as content blocks: any text first,
			// then one tool_use block per call.
			if msg.Content != "" {
				am.Content.Blocks = append(am.Content.Blocks, AnthropicContent{
					Type: "text",
					Text: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				var input json.RawMessage
				if len(tc.Arguments) > 0 {
					input = json.RawMessage(tc.Arguments)
				}
				am.Content.Blocks = append(am.Content.Blocks, AnthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			// No blocks at all → plain empty string so the "content" field stays
			// present.
			if len(am.Content.Blocks) == 0 {
				am.Content.String = ""
			}
		case "tool":
			// Canonical "tool" role → Anthropic user message carrying a
			// tool_result block.
			am.Role = "user"
			contentJSON, _ := json.Marshal(msg.Content)
			am.Content.Blocks = append(am.Content.Blocks, AnthropicContent{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   contentJSON,
			})
		default:
			am.Content.String = msg.Content
		}
		req.Messages = append(req.Messages, am)
	}

	return json.Marshal(req)
}

// thinkingBudget derives an Anthropic thinking budget_tokens from max_tokens
// and an optional OpenAI-style reasoning_effort hint. Effort maps to a share of
// max_tokens (low 25% / medium 50% / high 80%); a bare thinking:enabled with no
// effort uses 50%. Clamped to Anthropic's [1024, maxTokens-1] window.
func thinkingBudget(maxTokens int, effort string) int {
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	ratio := 0.5
	switch effort {
	case "low":
		ratio = 0.25
	case "medium":
		ratio = 0.5
	case "high":
		ratio = 0.8
	}
	budget := int(float64(maxTokens) * ratio)
	if budget < 1024 {
		budget = 1024
	}
	if budget > maxTokens-1 {
		budget = maxTokens - 1
	}
	return budget
}

// DecodeUpstreamResponse decodes raw Anthropic response to CanonicalResponse
func DecodeUpstreamResponse(body []byte) (*canonical.CanonicalResponse, error) {
	var resp AnthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	content := ""
	reasoning := ""
	var toolCalls []canonical.ToolCall
	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			content += c.Text
		case "thinking", "redacted_thinking":
			reasoning += c.Thinking
		case "tool_use":
			args, err := json.Marshal(c.Input)
			if err != nil {
				args = []byte("{}")
			}
			toolCalls = append(toolCalls, canonical.ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: string(args),
			})
		}
	}

	canResp := &canonical.CanonicalResponse{
		ID:               resp.ID,
		Model:            resp.Model,
		Role:             resp.Role,
		Content:          content,
		ReasoningContent: reasoning,
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		FinishReason:     resp.StopReason,
		ToolCalls:        toolCalls,
	}

	if resp.Usage.CacheReadInputTokens != nil {
		canResp.CachedInputTokens = *resp.Usage.CacheReadInputTokens
	}

	return canResp, nil
}

// FormatCanonicalToAnthropicResponse converts CanonicalResponse back to Anthropic format
func FormatCanonicalToAnthropicResponse(canResp *canonical.CanonicalResponse) ([]byte, error) {
	role := canResp.Role
	if role == "" {
		role = "assistant"
	}
	var content []AnthropicContent
	// A tool-call-only assistant response has no text block (mirrors OpenAI's
	// content:null); otherwise a text block is always emitted.
	if canResp.Content != "" || len(canResp.ToolCalls) == 0 {
		content = append(content, AnthropicContent{
			Type: "text",
			Text: canResp.Content,
		})
	}
	for _, tc := range canResp.ToolCalls {
		var input json.RawMessage
		if len(tc.Arguments) > 0 {
			input = json.RawMessage(tc.Arguments)
		}
		content = append(content, AnthropicContent{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if canResp.ReasoningContent != "" {
		content = append([]AnthropicContent{
			{
				Type:     "thinking",
				Thinking: canResp.ReasoningContent,
			},
		}, content...)
	}
	stopReason := canResp.FinishReason
	// Gemini reports finish_reason "STOP"; Anthropic clients expect "end_turn"
	// for a normal completion and "tool_use" for a tool-call response.
	switch {
	case len(canResp.ToolCalls) > 0:
		stopReason = "tool_use"
	case stopReason == "STOP":
		stopReason = "end_turn"
	}
	resp := AnthropicResponse{
		ID:         canResp.ID,
		Type:       "message",
		Role:       role,
		Model:      canResp.Model,
		Content:    content,
		StopReason: stopReason,
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
func FormatCanonicalToAnthropicSSE(chunk *canonical.CanonicalStreamChunk, index int, blockType string) (string, error) {
	var delta map[string]string
	if blockType == "thinking" && chunk.DeltaReasoning != "" {
		delta = map[string]string{
			"type":     "thinking_delta",
			"thinking": chunk.DeltaReasoning,
		}
	} else if blockType == "text" && chunk.DeltaContent != "" {
		delta = map[string]string{
			"type": "text_delta",
			"text": chunk.DeltaContent,
		}
	} else {
		return "", nil
	}

	deltaPayload := map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": delta,
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

	if eventName == "content_block_start" {
		var payload struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil && payload.ContentBlock.Type == "tool_use" {
			chunk.DeltaToolCalls = append(chunk.DeltaToolCalls, canonical.ToolCallDelta{
				Index: payload.Index,
				ID:    payload.ContentBlock.ID,
				Name:  payload.ContentBlock.Name,
			})
		}
		return chunk, nil
	}

	if eventName == "content_block_delta" {
		var payload struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil {
			switch payload.Delta.Type {
			case "thinking_delta":
				chunk.DeltaReasoning = payload.Delta.Thinking
			case "input_json_delta":
				// Incremental JSON fragment of a tool_use input. Index ties it to
				// the content_block_start that opened the block.
				chunk.DeltaToolCalls = append(chunk.DeltaToolCalls, canonical.ToolCallDelta{
					Index:     payload.Index,
					Arguments: payload.Delta.PartialJSON,
				})
			default:
				chunk.DeltaContent = payload.Delta.Text
			}
		}
		return chunk, nil
	}

	if eventName == "message_start" {
		var payload struct {
			Message struct {
				Usage struct {
					InputTokens          int  `json:"input_tokens"`
					CacheReadInputTokens *int `json:"cache_read_input_tokens,omitempty"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(eventData, &payload); err == nil {
			chunk.PromptTokens = payload.Message.Usage.InputTokens
			if payload.Message.Usage.CacheReadInputTokens != nil {
				chunk.CachedInputTokens = *payload.Message.Usage.CacheReadInputTokens
			}
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
