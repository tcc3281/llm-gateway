package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"llm-gateway/internal/model"
)

// InspectPayloadShape parses inbound payload wire structure without retaining
// or allocating prompt text or media blobs in heap memory.
func InspectPayloadShape(inboundFormat string, body []byte) *model.RequestShape {
	if len(body) == 0 {
		return nil
	}

	shape := &model.RequestShape{
		Roles:             make([]string, 0),
		ImageFormats:      make([]string, 0),
		AudioFormats:      make([]string, 0),
		ToolNames:         make([]string, 0),
		ToolParameterKeys: make([]string, 0),
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return shape
	}

	switch strings.ToLower(inboundFormat) {
	case "anthropic":
		inspectAnthropicShape(trimmed, shape)
	default: // "openai" or default
		inspectOpenAIShape(trimmed, shape)
	}

	// Classify Modality
	if shape.HasImages && shape.HasAudio {
		shape.Modality = "multimodal"
	} else if shape.HasImages {
		shape.Modality = "vision"
	} else if shape.HasAudio {
		shape.Modality = "audio"
	} else if shape.ToolCallsCount > 0 || shape.ToolsCount > 0 {
		shape.Modality = "tools"
	} else if shape.HasReasoning {
		shape.Modality = "reasoning"
	} else if shape.MessageCount == 0 && shape.CharCount > 0 {
		shape.Modality = "embeddings"
	} else {
		shape.Modality = "text"
	}

	// Compute deterministic StructureHash
	shape.StructureHash = computeStructureHash(shape)

	return shape
}

// computeStructureHash generates a deterministic SHA-256 fingerprint of the structural shape.
func computeStructureHash(s *model.RequestShape) string {
	h := sha256.New()
	fmt.Fprintf(h, "modality=%s;msgs=%d;roles=%s;img=%d(%d);aud=%d(%d);tools=%d;calls=%d;tnames=%s;reasoning=%t(%s)",
		s.Modality,
		s.MessageCount,
		strings.Join(s.Roles, ","),
		s.ImageCount,
		s.ImageTotalBytes,
		s.AudioCount,
		s.AudioTotalBytes,
		s.ToolsCount,
		s.ToolCallsCount,
		strings.Join(s.ToolNames, ","),
		s.HasReasoning,
		s.ReasoningEffort,
	)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

/* ────────────────────────── OpenAI Wire Inspector ────────────────────────── */

type openAIWirePayload struct {
	Messages            []openAIMessageWire `json:"messages"`
	Tools               []openAIToolWire    `json:"tools"`
	ReasoningEffort     string              `json:"reasoning_effort"`
	Temperature         *float64            `json:"temperature"`
	TopP                *float64            `json:"top_p"`
	MaxTokens           *int                `json:"max_tokens"`
	MaxCompletionTokens *int                `json:"max_completion_tokens"`
	PresencePenalty     *float64            `json:"presence_penalty"`
	FrequencyPenalty    *float64            `json:"frequency_penalty"`
	Seed                *int                `json:"seed"`
	Stop                json.RawMessage     `json:"stop"`
	Stream              *bool               `json:"stream"`
	// Embeddings input format support
	Input json.RawMessage `json:"input"`
}

type openAIMessageWire struct {
	Role      string               `json:"role"`
	Content   openAIContentWire    `json:"content"`
	ToolCalls []openAIToolCallWire `json:"tool_calls"`
}

type openAIToolCallWire struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type openAIToolWire struct {
	Type     string `json:"type"`
	Function struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// openAIContentWire handles string | array of content parts | null without large string allocations.
type openAIContentWire struct {
	TextChars int
	TextWords int
	Images    []mediaMeta
	Audios    []mediaMeta
}

type mediaMeta struct {
	Format    string
	ByteCount int64
}

func (c *openAIContentWire) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}

	if data[0] == '"' {
		// Single string content
		c.TextChars, c.TextWords = scanTextMetrics(data)
		return nil
	}

	if data[0] == '[' {
		// Array of content parts: [{"type":"text","text":"..."}, {"type":"image_url", ...}, {"type":"input_audio", ...}]
		var rawParts []json.RawMessage
		if err := json.Unmarshal(data, &rawParts); err != nil {
			return nil
		}

		for _, p := range rawParts {
			p = bytes.TrimSpace(p)
			if len(p) == 0 || p[0] != '{' {
				continue
			}

			var partType struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(p, &partType)

			switch partType.Type {
			case "text":
				var tPart struct {
					Text rawTextCounter `json:"text"`
				}
				if json.Unmarshal(p, &tPart) == nil {
					c.TextChars += tPart.Text.Chars
					c.TextWords += tPart.Text.Words
				}

			case "image_url":
				var iPart struct {
					ImageURL struct {
						URL byteCounter `json:"url"`
					} `json:"image_url"`
				}
				if json.Unmarshal(p, &iPart) == nil {
					c.Images = append(c.Images, mediaMeta{
						Format:    iPart.ImageURL.URL.Format,
						ByteCount: iPart.ImageURL.URL.ByteLength,
					})
				}

			case "input_audio":
				var aPart struct {
					InputAudio struct {
						Data   byteCounter `json:"data"`
						Format string      `json:"format"`
					} `json:"input_audio"`
				}
				if json.Unmarshal(p, &aPart) == nil {
					fmtStr := aPart.InputAudio.Format
					if fmtStr == "" {
						fmtStr = "audio/wav"
					}
					c.Audios = append(c.Audios, mediaMeta{
						Format:    fmtStr,
						ByteCount: aPart.InputAudio.Data.ByteLength,
					})
				}
			}
		}
	}
	return nil
}

func inspectOpenAIShape(body []byte, shape *model.RequestShape) {
	var payload openAIWirePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	// 1. Embeddings fallback
	if len(payload.Messages) == 0 && len(payload.Input) > 0 {
		inspectEmbeddingsInput(payload.Input, shape)
		return
	}

	// 2. Messages inspection
	shape.MessageCount = len(payload.Messages)
	rolesMap := make(map[string]struct{})
	toolNamesMap := make(map[string]struct{})

	for _, msg := range payload.Messages {
		if msg.Role != "" {
			if _, exists := rolesMap[msg.Role]; !exists {
				rolesMap[msg.Role] = struct{}{}
				shape.Roles = append(shape.Roles, msg.Role)
			}
		}

		shape.CharCount += msg.Content.TextChars
		shape.WordCount += msg.Content.TextWords
		if msg.Role == "system" {
			shape.SystemPromptChars += msg.Content.TextChars
		}

		for _, img := range msg.Content.Images {
			shape.HasImages = true
			shape.ImageCount++
			shape.ImageTotalBytes += img.ByteCount
			if img.Format != "" && !containsStr(shape.ImageFormats, img.Format) {
				shape.ImageFormats = append(shape.ImageFormats, img.Format)
			}
		}

		for _, aud := range msg.Content.Audios {
			shape.HasAudio = true
			shape.AudioCount++
			shape.AudioTotalBytes += aud.ByteCount
			if aud.Format != "" && !containsStr(shape.AudioFormats, aud.Format) {
				shape.AudioFormats = append(shape.AudioFormats, aud.Format)
			}
		}

		// Tool calls in message turn
		for _, tc := range msg.ToolCalls {
			shape.ToolCallsCount++
			if tc.Function.Name != "" {
				if _, exists := toolNamesMap[tc.Function.Name]; !exists {
					toolNamesMap[tc.Function.Name] = struct{}{}
					shape.ToolNames = append(shape.ToolNames, tc.Function.Name)
				}
			}
			if len(tc.Function.Arguments) > 0 {
				extractParamKeys(tc.Function.Arguments, shape)
			}
		}
	}

	// 3. Tools definitions
	shape.ToolsCount = len(payload.Tools)
	for _, t := range payload.Tools {
		if t.Function.Name != "" {
			if _, exists := toolNamesMap[t.Function.Name]; !exists {
				toolNamesMap[t.Function.Name] = struct{}{}
				shape.ToolNames = append(shape.ToolNames, t.Function.Name)
			}
		}
		if len(t.Function.Parameters) > 0 {
			extractSchemaPropertyKeys(t.Function.Parameters, shape)
		}
	}

	// 4. Reasoning effort
	if payload.ReasoningEffort != "" {
		shape.HasReasoning = true
		shape.ReasoningEffort = payload.ReasoningEffort
	}

	// 5. Hyperparameters & Settings
	shape.Temperature = payload.Temperature
	shape.TopP = payload.TopP
	if payload.MaxCompletionTokens != nil {
		shape.MaxTokens = payload.MaxCompletionTokens
	} else if payload.MaxTokens != nil {
		shape.MaxTokens = payload.MaxTokens
	}
	shape.PresencePenalty = payload.PresencePenalty
	shape.FrequencyPenalty = payload.FrequencyPenalty
	shape.Seed = payload.Seed
	shape.Stream = payload.Stream
	shape.StopSequences = parseStopSequences(payload.Stop)
}

/* ──────────────────────── Anthropic Wire Inspector ───────────────────────── */

type anthropicWirePayload struct {
	System        anthropicSystemWire `json:"system"`
	Messages      []anthropicMsgWire  `json:"messages"`
	Tools         []anthropicToolWire `json:"tools"`
	Thinking      *anthropicThinking  `json:"thinking"`
	Temperature   *float64            `json:"temperature"`
	TopP          *float64            `json:"top_p"`
	TopK          *int                `json:"top_k"`
	MaxTokens     *int                `json:"max_tokens"`
	StopSequences []string            `json:"stop_sequences"`
	Stream        *bool               `json:"stream"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicSystemWire struct {
	Chars int
	Words int
}

func (s *anthropicSystemWire) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	if data[0] == '"' {
		s.Chars, s.Words = scanTextMetrics(data)
		return nil
	}
	if data[0] == '[' {
		var parts []struct {
			Type string          `json:"type"`
			Text rawTextCounter `json:"text"`
		}
		if json.Unmarshal(data, &parts) == nil {
			for _, p := range parts {
				if p.Type == "text" {
					s.Chars += p.Text.Chars
					s.Words += p.Text.Words
				}
			}
		}
	}
	return nil
}

type anthropicMsgWire struct {
	Role    string                  `json:"role"`
	Content anthropicMsgContentWire `json:"content"`
}

type anthropicToolWire struct {
	Name        string          `json:"name"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicMsgContentWire struct {
	TextChars int
	TextWords int
	Images    []mediaMeta
	ToolCalls []anthropicToolCallMeta
}

type anthropicToolCallMeta struct {
	Name  string
	Input json.RawMessage
}

func (c *anthropicMsgContentWire) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	if data[0] == '"' {
		c.TextChars, c.TextWords = scanTextMetrics(data)
		return nil
	}
	if data[0] == '[' {
		var rawParts []json.RawMessage
		if json.Unmarshal(data, &rawParts) != nil {
			return nil
		}
		for _, p := range rawParts {
			p = bytes.TrimSpace(p)
			if len(p) == 0 || p[0] != '{' {
				continue
			}

			var base struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(p, &base)

			switch base.Type {
			case "text":
				var tPart struct {
					Text rawTextCounter `json:"text"`
				}
				if json.Unmarshal(p, &tPart) == nil {
					c.TextChars += tPart.Text.Chars
					c.TextWords += tPart.Text.Words
				}

			case "image":
				var imgPart struct {
					Source struct {
						Type      string      `json:"type"`
						MediaType string      `json:"media_type"`
						Data      byteCounter `json:"data"`
					} `json:"source"`
				}
				if json.Unmarshal(p, &imgPart) == nil {
					c.Images = append(c.Images, mediaMeta{
						Format:    imgPart.Source.MediaType,
						ByteCount: imgPart.Source.Data.ByteLength,
					})
				}

			case "tool_use":
				var tuPart struct {
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				}
				if json.Unmarshal(p, &tuPart) == nil {
					c.ToolCalls = append(c.ToolCalls, anthropicToolCallMeta{
						Name:  tuPart.Name,
						Input: tuPart.Input,
					})
				}
			}
		}
	}
	return nil
}

func inspectAnthropicShape(body []byte, shape *model.RequestShape) {
	var payload anthropicWirePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	shape.MessageCount = len(payload.Messages)
	rolesMap := make(map[string]struct{})
	toolNamesMap := make(map[string]struct{})

	// 1. System prompt
	if payload.System.Chars > 0 {
		shape.SystemPromptChars = payload.System.Chars
		shape.CharCount += payload.System.Chars
		shape.WordCount += payload.System.Words
		rolesMap["system"] = struct{}{}
		shape.Roles = append(shape.Roles, "system")
	}

	// 2. Messages
	for _, msg := range payload.Messages {
		if msg.Role != "" {
			if _, exists := rolesMap[msg.Role]; !exists {
				rolesMap[msg.Role] = struct{}{}
				shape.Roles = append(shape.Roles, msg.Role)
			}
		}

		shape.CharCount += msg.Content.TextChars
		shape.WordCount += msg.Content.TextWords

		for _, img := range msg.Content.Images {
			shape.HasImages = true
			shape.ImageCount++
			shape.ImageTotalBytes += img.ByteCount
			if img.Format != "" && !containsStr(shape.ImageFormats, img.Format) {
				shape.ImageFormats = append(shape.ImageFormats, img.Format)
			}
		}

		for _, tu := range msg.Content.ToolCalls {
			shape.ToolCallsCount++
			if tu.Name != "" {
				if _, exists := toolNamesMap[tu.Name]; !exists {
					toolNamesMap[tu.Name] = struct{}{}
					shape.ToolNames = append(shape.ToolNames, tu.Name)
				}
			}
			if len(tu.Input) > 0 {
				extractParamKeys(tu.Input, shape)
			}
		}
	}

	// 3. Tools definitions
	shape.ToolsCount = len(payload.Tools)
	for _, t := range payload.Tools {
		if t.Name != "" {
			if _, exists := toolNamesMap[t.Name]; !exists {
				toolNamesMap[t.Name] = struct{}{}
				shape.ToolNames = append(shape.ToolNames, t.Name)
			}
		}
		if len(t.InputSchema) > 0 {
			extractSchemaPropertyKeys(t.InputSchema, shape)
		}
	}

	// 4. Thinking
	if payload.Thinking != nil && (payload.Thinking.Type == "enabled" || payload.Thinking.BudgetTokens > 0) {
		shape.HasReasoning = true
		if payload.Thinking.BudgetTokens > 0 {
			shape.ReasoningEffort = fmt.Sprintf("budget:%d", payload.Thinking.BudgetTokens)
		} else {
			shape.ReasoningEffort = "enabled"
		}
	}

	// 5. Hyperparameters & Settings
	shape.Temperature = payload.Temperature
	shape.TopP = payload.TopP
	shape.TopK = payload.TopK
	shape.MaxTokens = payload.MaxTokens
	shape.Stream = payload.Stream
	shape.StopSequences = payload.StopSequences
}

func parseStopSequences(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return []string{s}
		}
	} else if len(raw) >= 2 && raw[0] == '[' {
		var list []string
		if json.Unmarshal(raw, &list) == nil {
			return list
		}
	}
	return nil
}

/* ────────────────────────── Helpers & Byte Scanning ──────────────────────── */

// byteCounter inspects base64 or URL data length without retaining strings in heap.
type byteCounter struct {
	ByteLength int64
	Format     string
	IsURL      bool
}

func (bc *byteCounter) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		unquoted := data[1 : len(data)-1]

		if bytes.HasPrefix(unquoted, []byte("data:")) {
			commaIdx := bytes.IndexByte(unquoted, ',')
			if commaIdx != -1 {
				header := string(unquoted[5:commaIdx])
				b64DataLen := len(unquoted) - commaIdx - 1
				bc.ByteLength = int64((b64DataLen * 3) / 4)
				if semiIdx := strings.IndexByte(header, ';'); semiIdx != -1 {
					bc.Format = header[:semiIdx]
				} else {
					bc.Format = header
				}
				return nil
			}
		} else if bytes.HasPrefix(unquoted, []byte("http://")) || bytes.HasPrefix(unquoted, []byte("https://")) {
			bc.IsURL = true
			bc.Format = "url"
			bc.ByteLength = 0
			return nil
		}

		// Raw base64 string
		bc.ByteLength = int64((len(unquoted) * 3) / 4)
		bc.Format = "base64"
	}
	return nil
}

// rawTextCounter computes character and word counts without heap allocations.
type rawTextCounter struct {
	Chars int
	Words int
}

func (tc *rawTextCounter) UnmarshalJSON(data []byte) error {
	tc.Chars, tc.Words = scanTextMetrics(data)
	return nil
}

func scanTextMetrics(data []byte) (chars int, words int) {
	data = bytes.TrimSpace(data)
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return 0, 0
	}
	content := data[1 : len(data)-1]
	chars = len(content)

	inWord := false
	for _, b := range content {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\\' {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return chars, words
}

func inspectEmbeddingsInput(input json.RawMessage, shape *model.RequestShape) {
	input = bytes.TrimSpace(input)
	if len(input) == 0 {
		return
	}
	if input[0] == '"' {
		c, w := scanTextMetrics(input)
		shape.CharCount += c
		shape.WordCount += w
		shape.MessageCount = 1
	} else if input[0] == '[' {
		var list []rawTextCounter
		if json.Unmarshal(input, &list) == nil {
			shape.MessageCount = len(list)
			for _, item := range list {
				shape.CharCount += item.Chars
				shape.WordCount += item.Words
			}
		}
	}
}

func extractParamKeys(argsRaw json.RawMessage, shape *model.RequestShape) {
	argsRaw = bytes.TrimSpace(argsRaw)
	if len(argsRaw) == 0 {
		return
	}

	var rawObj map[string]interface{}
	if argsRaw[0] == '"' {
		// OpenAI passes arguments as an escaped JSON string: "{\"city\": \"Hanoi\"}"
		var str string
		if json.Unmarshal(argsRaw, &str) == nil {
			_ = json.Unmarshal([]byte(str), &rawObj)
		}
	} else if argsRaw[0] == '{' {
		_ = json.Unmarshal(argsRaw, &rawObj)
	}

	for k := range rawObj {
		if !containsStr(shape.ToolParameterKeys, k) {
			shape.ToolParameterKeys = append(shape.ToolParameterKeys, k)
		}
	}
}

func extractSchemaPropertyKeys(schemaRaw json.RawMessage, shape *model.RequestShape) {
	var schema struct {
		Properties map[string]interface{} `json:"properties"`
	}
	if json.Unmarshal(schemaRaw, &schema) == nil {
		for k := range schema.Properties {
			if !containsStr(shape.ToolParameterKeys, k) {
				shape.ToolParameterKeys = append(shape.ToolParameterKeys, k)
			}
		}
	}
}

func containsStr(slice []string, s string) bool {
	for _, it := range slice {
		if it == s {
			return true
		}
	}
	return false
}

// FormatBytes formats a byte count into a human-readable string (KB, MB).
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// IsASCIIWord reports whether r is an ASCII alphanumeric character.
func IsASCIIWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
