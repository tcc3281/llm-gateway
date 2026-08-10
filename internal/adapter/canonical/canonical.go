package canonical

type ChatMessage struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Text content
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
}

type CanonicalResponse struct {
	ID               string `json:"id"`
	Model            string `json:"model"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	FinishReason     string `json:"finish_reason"`
}

type CanonicalStreamChunk struct {
	ID               string `json:"id"`
	Model            string `json:"model"`
	DeltaContent     string `json:"delta_content"`
	Role             string `json:"role,omitempty"`
	FinishReason     string `json:"finish_reason,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	IsLast           bool   `json:"is_last"`
}
