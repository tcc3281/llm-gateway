package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"llm-gateway/internal/model"
)

// SetGenAIAttributes sets OpenTelemetry GenAI semantic conventions on the given span.
func SetGenAIAttributes(span trace.Span, system, model, operation string) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("gen_ai.system", system),
		attribute.String("gen_ai.request.model", model),
		attribute.String("gen_ai.operation.name", operation),
	)
}

// SetGenAIUsage sets token counts using GenAI semantic conventions.
func SetGenAIUsage(span trace.Span, inputTokens, outputTokens int) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.Int("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int("gen_ai.usage.output_tokens", outputTokens),
	)
}

// SetLLMGWAttributes sets custom LLM Gateway attributes on the span.
func SetLLMGWAttributes(span trace.Span, provider string, cacheHit, fallbackUsed bool, ttftMs int64) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("llmgw.provider", provider),
		attribute.Bool("llmgw.cache_hit", cacheHit),
		attribute.Bool("llmgw.fallback_used", fallbackUsed),
		attribute.Int64("llmgw.ttft_ms", ttftMs),
	)
}

// RecordRunawayLoopEvent records a warning event on the span when tool calls exceed the threshold.
func RecordRunawayLoopEvent(span trace.Span, toolCallsCount, threshold int) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent("gen_ai.agent.runaway_loop_detected", trace.WithAttributes(
		attribute.Int("gen_ai.tool_calls.count", toolCallsCount),
		attribute.Int("gen_ai.tool_calls.threshold", threshold),
		attribute.String("gen_ai.agent.warning", "Agent tool calls exceeded configured loop threshold"),
	))
}

// SetGenAIShapeAttributes sets OpenTelemetry GenAI semantic conventions for content-agnostic structural tracing.
func SetGenAIShapeAttributes(span trace.Span, shape *model.RequestShape) {
	if span == nil || !span.IsRecording() || shape == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.Int("gen_ai.request.message_count", shape.MessageCount),
		attribute.Int("gen_ai.request.char_count", shape.CharCount),
		attribute.Int("gen_ai.request.word_count", shape.WordCount),
		attribute.StringSlice("gen_ai.request.roles", shape.Roles),
		attribute.Bool("gen_ai.request.has_images", shape.HasImages),
		attribute.Int("gen_ai.request.image_count", shape.ImageCount),
		attribute.Int64("gen_ai.request.image_bytes", shape.ImageTotalBytes),
		attribute.Bool("gen_ai.request.has_audio", shape.HasAudio),
		attribute.Int("gen_ai.request.audio_count", shape.AudioCount),
		attribute.Int64("gen_ai.request.audio_bytes", shape.AudioTotalBytes),
		attribute.Int("gen_ai.request.tools_count", shape.ToolsCount),
		attribute.Int("gen_ai.request.tool_calls_count", shape.ToolCallsCount),
		attribute.StringSlice("gen_ai.request.tool_names", shape.ToolNames),
		attribute.String("gen_ai.request.modality", shape.Modality),
		attribute.String("gen_ai.request.structure_hash", shape.StructureHash),
	}
	if shape.SystemPromptChars > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.request.system_prompt_chars", shape.SystemPromptChars))
	}
	if shape.HasReasoning {
		attrs = append(attrs, attribute.Bool("gen_ai.request.has_reasoning", true))
		if shape.ReasoningEffort != "" {
			attrs = append(attrs, attribute.String("gen_ai.request.reasoning_effort", shape.ReasoningEffort))
		}
	}
	if shape.Temperature != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.temperature", *shape.Temperature))
	}
	if shape.TopP != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.top_p", *shape.TopP))
	}
	if shape.TopK != nil {
		attrs = append(attrs, attribute.Int("gen_ai.request.top_k", *shape.TopK))
	}
	if shape.MaxTokens != nil {
		attrs = append(attrs, attribute.Int("gen_ai.request.max_tokens", *shape.MaxTokens))
	}
	if shape.PresencePenalty != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.presence_penalty", *shape.PresencePenalty))
	}
	if shape.FrequencyPenalty != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.frequency_penalty", *shape.FrequencyPenalty))
	}
	if shape.Seed != nil {
		attrs = append(attrs, attribute.Int("gen_ai.request.seed", *shape.Seed))
	}
	if len(shape.StopSequences) > 0 {
		attrs = append(attrs, attribute.StringSlice("gen_ai.request.stop_sequences", shape.StopSequences))
	}
	span.SetAttributes(attrs...)
}

