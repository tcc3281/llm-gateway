package canonical

import "encoding/json"

// ToolDefinition is the provider-neutral declaration of a callable tool.
// Parameters carries the JSON-schema object describing the tool's input.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolChoice is the provider-neutral policy for whether/how a tool is invoked.
// Mode is one of "auto" | "none" | "required" | "any" | "tool" | "function".
// FunctionName is set for the explicit-selection modes ("tool"/"function").
type ToolChoice struct {
	Mode         string `json:"mode"`
	FunctionName string `json:"function_name,omitempty"`
}

// ToolCall is an assistant turn's request to invoke one tool. Arguments is the
// JSON-encoded argument object as a string (the OpenAI wire representation).
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallDelta is one streaming fragment of a tool call (OpenAI
// delta.tool_calls, Anthropic content_block_start/input_json_delta, Gemini
// functionCall parts). Arguments holds the incremental partial JSON.
type ToolCallDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ContentPart represents a multimodal part in a chat message (text, image, etc.)
type ContentPart struct {
	Type     string    `json:"type"` // "text" | "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ChatMessage is one message in the conversation. Text lives in Content; an
// assistant message may additionally carry ToolCalls, and a role=="tool"
// message (a tool result) carries ToolCallID identifying which call it answers.
// ToolCallID doubles as the Anthropic tool_use_id and the key used to resolve a
// Gemini functionResponse name from the preceding assistant tool call.
// Multimodal parts (images, structured content) are stored in ContentParts.
type ChatMessage struct {
	Role         string        `json:"role"` // "system" | "user" | "assistant" | "tool"
	Content      string        `json:"content"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

type CanonicalRequest struct {
	InboundFormat string        `json:"inbound_format"` // "openai" or "anthropic"
	Model         string        `json:"model"`
	Messages      []ChatMessage `json:"messages"`
	SystemPrompt  string        `json:"system_prompt,omitempty"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	Temperature   *float64      `json:"temperature,omitempty"`
	TopP          *float64      `json:"top_p,omitempty"`
	Stream        bool          `json:"stream"`
	Stop          []string      `json:"stop,omitempty"`
	// Tools declares the callable tools forwarded to the upstream. ToolChoice
	// picks the invocation policy when the client requested a specific tool.
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice *ToolChoice      `json:"tool_choice,omitempty"`
	// ThinkingEnabled asks the upstream to produce reasoning/thinking content.
	// Set from the inbound provider's native field: OpenAI `reasoning_effort`
	// (any value) or Anthropic `thinking:{type:"enabled"}`. Each adapter maps it
	// to its own wire field (OpenAI reasoning_effort, Anthropic thinking,
	// Gemini thinkingConfig).
	ThinkingEnabled bool
	// ReasoningEffort carries the OpenAI "reasoning_effort" value verbatim
	// ("" | "low" | "medium" | "high"). It doubles as the thinking-budget hint
	// for Anthropic/Gemini upstreams when the client used an OpenAI-style
	// request.
	ReasoningEffort string
}

type CanonicalResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Role    string `json:"role"`
	Content string `json:"content"`
	// ReasoningContent is the reasoning/thinking text the upstream produced
	// before the visible answer (OpenAI message.reasoning_content, Anthropic
	// thinking blocks, Gemini thought parts). Forwarded to the client by the
	// adapters so apps can display it.
	ReasoningContent  string `json:"reasoning_content,omitempty"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	TotalTokens       int    `json:"total_tokens"`
	CachedInputTokens int    `json:"cached_input_tokens,omitempty"`
	ReasoningTokens   int    `json:"reasoning_tokens,omitempty"`
	FinishReason      string `json:"finish_reason"`
	// ToolCalls carries any assistant tool calls the upstream requested. The
	// non-streaming equivalent of the DeltaToolCalls stream fragments.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type CanonicalStreamChunk struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	DeltaContent string `json:"delta_content"`
	// DeltaReasoning is the streaming equivalent of ReasoningContent: the
	// reasoning delta in a chunk (OpenAI delta.reasoning_content, Anthropic
	// thinking_delta, Gemini thought parts).
	DeltaReasoning    string `json:"delta_reasoning,omitempty"`
	Role              string `json:"role,omitempty"`
	FinishReason      string `json:"finish_reason,omitempty"`
	PromptTokens      int    `json:"prompt_tokens,omitempty"`
	CompletionTokens  int    `json:"completion_tokens,omitempty"`
	CachedInputTokens int    `json:"cached_input_tokens,omitempty"`
	ReasoningTokens   int    `json:"reasoning_tokens,omitempty"`
	IsLast            bool   `json:"is_last"`
	// DeltaToolCalls carries streaming fragments of assistant tool calls. The
	// gateway service accumulates them and reconstructs the complete calls when
	// the stream ends (Arguments is incremental partial JSON).
	DeltaToolCalls []ToolCallDelta `json:"delta_tool_calls,omitempty"`
}
