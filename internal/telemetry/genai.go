package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
