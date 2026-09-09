package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/telemetry"
	"llm-gateway/pkg/sse"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func (g *GatewayService) handleStreamResponse(ctx context.Context, params ProxyParams, target ResolvedTarget, canReq *canonical.CanonicalRequest, upResp *provider.UpstreamResponse, startTime time.Time, fallbackUsed bool, inputToolCalls int) error {
	flusher, ok := params.ResponseWriter.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported by response writer")
	}

	params.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	params.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	params.ResponseWriter.Header().Set("Connection", "keep-alive")
	params.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
	params.ResponseWriter.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Anthropic clients require the full message lifecycle (message_start →
	// content_block_start/delta/stop → message_delta → message_stop). That is
	// reconstructed lazily from the canonical deltas below so thinking blocks
	// (which precede the text block) are surfaced correctly and message_start
	// can carry the real input token counts gathered from the upstream.
	var anthroMsgID = "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
	anthroMessageStarted := false
	anthroOpenBlock := "" // "" | "thinking" | "text"
	anthroBlockIndex := -1

	sseReader := sse.NewReader(upResp.Body)
	defer sseReader.Close()
	var ttftMs int64 = 0
	var totalPromptTokens, totalCompletionTokens int
	var totalCachedInputTokens, totalReasoningTokens int
	var lastFinishReason string
	isFirstChunk := true
	var chunkCount int
	var streamContent strings.Builder

	// emitAnthropicMessageStart lazily writes the message_start opener once,
	// using the real input/cache counts accumulated so far from the upstream.
	emitAnthropicMessageStart := func() {
		if anthroMessageStarted {
			return
		}
		anthroMessageStarted = true
		usage := map[string]interface{}{"input_tokens": totalPromptTokens, "output_tokens": 1}
		if totalCachedInputTokens > 0 {
			usage["cache_read_input_tokens"] = totalCachedInputTokens
		}
		msg := map[string]interface{}{
			"type": "message", "id": anthroMsgID, "role": "assistant", "model": canReq.Model,
			"content": []interface{}{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage,
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "message_start", "message": msg}); err == nil {
			params.ResponseWriter.Write([]byte("event: message_start\ndata: " + string(data) + "\n\n"))
		}
	}

	// startAnthropicBlock opens a content block, closing any currently-open
	// block first so the client sees a strict thinking → text ordering.
	startAnthropicBlock := func(blockType string) {
		if anthroOpenBlock == blockType {
			return
		}
		if anthroOpenBlock != "" {
			if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
				params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
			}
		}
		anthroBlockIndex++
		block := map[string]interface{}{"type": blockType}
		if blockType == "thinking" {
			block["thinking"] = ""
		} else {
			block["text"] = ""
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_start", "index": anthroBlockIndex, "content_block": block}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_start\ndata: " + string(data) + "\n\n"))
		}
		anthroOpenBlock = blockType
	}

	// Tool-call fragments accumulate over the stream (OpenAI delta.tool_calls,
	// Anthropic content_block_start/input_json_delta, Gemini functionCall parts)
	// and are reconstructed as complete calls at stream end. They cannot be
	// streamed incrementally because fragments arrive split across chunks; the
	// client receives the full calls in one terminal block.
	type pendingToolCall struct {
		key              string
		id               string
		name             string
		thoughtSignature string
		args             strings.Builder
	}
	acc := map[string]*pendingToolCall{}
	order := []string{} // insertion order → stable call ordering

	// emitAnthropicToolBlock writes one reconstructed tool_use block (start →
	// input_json_delta → stop) for an Anthropic-bound client, mirroring how
	// Anthropic streams a complete tool_use.
	emitAnthropicToolBlock := func(tc canonical.ToolCall) {
		anthroBlockIndex++
		block := map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": map[string]interface{}{},
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_start", "index": anthroBlockIndex, "content_block": block}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_start\ndata: " + string(data) + "\n\n"))
		}
		deltaPayload := map[string]interface{}{
			"type":  "content_block_delta",
			"index": anthroBlockIndex,
			"delta": map[string]string{"type": "input_json_delta", "partial_json": tc.Arguments},
		}
		if data, err := json.Marshal(deltaPayload); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_delta\ndata: " + string(data) + "\n\n"))
		}
		if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
			params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
		}
		anthroOpenBlock = ""
	}

	// The upstream body read (sseReader.ReadEvent) blocks until a full SSE event
	// arrives; reasoning models can sit silent for tens of seconds, so a plain
	// blocking loop would leave the downstream connection idle long enough for a
	// proxy/load-balancer to reap it mid-thought. Drive the read from a goroutine
	// so the relay loop stays selectable: it emits flushed keep-alive comment
	// lines during those gaps and aborts promptly when the client disconnects
	// instead of draining (and paying for) the rest of the upstream stream. The
	// channel buffer caps the read-ahead at one event.
	eventC := make(chan *sse.Event, 1)
	readDoneC := make(chan error, 1)
	go func() {
		for {
			ev, rErr := sseReader.ReadEvent()
			if rErr != nil {
				readDoneC <- rErr
				return
			}
			select {
			case eventC <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Comment frames are ignored by SSE clients but reset idle timers on
	// intermediaries. 15s stays well under the common 60s LB/proxy idle timeout
	// (nginx, AWS ALB), so an upstream that is thinking — not dead — is not
	// mistaken for a hung connection.
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	// streamErr records an abnormal end (mid-stream read failure or client
	// cancellation); the terminal block below then emits an explicit error marker
	// instead of a clean [DONE]/message_stop.
	var streamErr error

relayLoop:
	for {
		var event *sse.Event
		var err error
		select {
		case event = <-eventC:
			// a full SSE event from the upstream to relay
		case err = <-readDoneC:
			if err == io.EOF {
				break relayLoop
			}
			streamErr = err
			break relayLoop
		case <-keepAlive.C:
			params.ResponseWriter.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
			continue relayLoop
		case <-ctx.Done():
			streamErr = ctx.Err()
			break relayLoop
		}

		if isFirstChunk {
			ttftMs = time.Since(startTime).Milliseconds()
			isFirstChunk = false
		}
		chunkCount++

		// Convert SSE event to Canonical Stream Chunk
		var chunk *canonical.CanonicalStreamChunk

		switch target.Provider {
		case provider.ProviderOpenAI:
			if string(event.Data) == "[DONE]" {
				chunk = &canonical.CanonicalStreamChunk{IsLast: true}
			} else {
				c, cErr := openai.DecodeStreamChunk(event.Data, canReq.Model)
				if cErr == nil && c != nil {
					chunk = c
					if c.PromptTokens > totalPromptTokens {
						totalPromptTokens = c.PromptTokens
					}
					if c.CompletionTokens > totalCompletionTokens {
						totalCompletionTokens = c.CompletionTokens
					}
					if c.CachedInputTokens > totalCachedInputTokens {
						totalCachedInputTokens = c.CachedInputTokens
					}
					if c.ReasoningTokens > totalReasoningTokens {
						totalReasoningTokens = c.ReasoningTokens
					}
				}
			}

		case provider.ProviderAnthropic:
			c, cErr := anthropic.DecodeStreamChunk(event.Event, event.Data, canReq.Model)
			if cErr == nil && c != nil {
				chunk = c
				if c.PromptTokens > 0 {
					totalPromptTokens = c.PromptTokens
				}
				if c.CompletionTokens > 0 {
					totalCompletionTokens = c.CompletionTokens
				}
				if c.CachedInputTokens > 0 {
					totalCachedInputTokens = c.CachedInputTokens
				}
				if c.ReasoningTokens > 0 {
					totalReasoningTokens = c.ReasoningTokens
				}
			}

		case provider.ProviderGemini:
			c, cErr := gemini.DecodeStreamChunk(event.Data, canReq.Model)
			if cErr == nil && c != nil {
				chunk = c
				if c.CompletionTokens > 0 {
					totalCompletionTokens = c.CompletionTokens
					totalPromptTokens = c.PromptTokens
				}
				if c.CachedInputTokens > 0 {
					totalCachedInputTokens = c.CachedInputTokens
				}
				if c.ReasoningTokens > 0 {
					totalReasoningTokens = c.ReasoningTokens
				}
			}
		}

		if chunk == nil {
			continue
		}
		if chunk.FinishReason != "" {
			lastFinishReason = chunk.FinishReason
		}
		if chunk.DeltaContent != "" && strings.EqualFold(g.contentCapturePolicy, "event_only") {
			streamContent.WriteString(chunk.DeltaContent)
		}

		// Feed tool-call fragments into the accumulator (never streamed
		// incrementally). OpenAI/Anthropic key by the stable chunk index; Gemini
		// parts are atomic with no stable cross-chunk index, so key by name.
		for _, dtc := range chunk.DeltaToolCalls {
			key := strconv.Itoa(dtc.Index)
			if target.Provider == provider.ProviderGemini {
				key = "g:" + dtc.Name
			}
			p, ok := acc[key]
			if !ok {
				p = &pendingToolCall{key: key}
				acc[key] = p
				order = append(order, key)
			}
			if p.id == "" {
				p.id = dtc.ID
			}
			if p.name == "" {
				p.name = dtc.Name
			}
			if p.thoughtSignature == "" && dtc.ThoughtSignature != "" {
				p.thoughtSignature = dtc.ThoughtSignature
				if !strings.Contains(p.id, "__thought__") {
					p.id = p.id + "__thought__" + dtc.ThoughtSignature
				}
			}
			if dtc.Arguments != "" {
				p.args.WriteString(dtc.Arguments)
			}
		}

		if params.InboundFormat == "openai" {
			// Skip the bare terminal marker; the [DONE] line is written below.
			if chunk.IsLast && chunk.DeltaContent == "" && chunk.FinishReason == "" {
				continue
			}
			// Pure tool-call fragments carry no text/reasoning to emit — they are
			// reconstructed at stream end.
			if len(chunk.DeltaToolCalls) > 0 && chunk.DeltaContent == "" && chunk.DeltaReasoning == "" {
				continue
			}
			// Any upstream finish_reason chunk is suppressed while tool calls are
			// being accumulated; the reconstructed terminal block carries the authoritative finish.
			if chunk.FinishReason != "" && len(acc) > 0 {
				if chunk.DeltaContent == "" {
					continue
				}
				chunkCopy := *chunk
				chunkCopy.FinishReason = ""
				chunk = &chunkCopy
			}
			sseBytes, fErr := openai.FormatCanonicalToOpenAISSEBytes(chunk)
			if fErr == nil && len(sseBytes) > 0 {
				params.ResponseWriter.Write(sseBytes)
				flusher.Flush()
			}
		} else {
			// Anthropic inbound: rebuild the block lifecycle from canonical
			// deltas. Thinking deltas open the thinking block (index 0) before
			// the text block (index 1); message_start is emitted lazily ahead
			// of the first content event.
			if chunk.DeltaReasoning != "" {
				emitAnthropicMessageStart()
				startAnthropicBlock("thinking")
				sseFormatted, fErr := anthropic.FormatCanonicalToAnthropicSSE(chunk, anthroBlockIndex, "thinking")
				if fErr == nil && sseFormatted != "" {
					params.ResponseWriter.Write([]byte(sseFormatted))
					flusher.Flush()
				}
			}
			if chunk.DeltaContent != "" {
				emitAnthropicMessageStart()
				startAnthropicBlock("text")
				sseFormatted, fErr := anthropic.FormatCanonicalToAnthropicSSE(chunk, anthroBlockIndex, "text")
				if fErr == nil && sseFormatted != "" {
					params.ResponseWriter.Write([]byte(sseFormatted))
					flusher.Flush()
				}
			}
		}
	}

	// Reconstruct complete tool calls from the accumulated fragments, preserving
	// insertion order.
	var toolCalls []canonical.ToolCall
	for _, key := range order {
		p := acc[key]
		if p.name == "" {
			continue
		}
		toolCalls = append(toolCalls, canonical.ToolCall{
			ID:               p.id,
			Name:             p.name,
			Arguments:        p.args.String(),
			ThoughtSignature: p.thoughtSignature,
		})
	}
	hasToolCalls := len(toolCalls) > 0

	// Final stream completion signal. A mid-stream failure must not be masked as
	// a clean end: emit an explicit error event instead of [DONE]/message_stop so
	// clients surface the interruption rather than treating the response as
	// complete (and, for Anthropic, so they do not hang waiting for message_stop).
	if streamErr != nil {
		if params.InboundFormat == "openai" {
			// OpenAI SSE has no spec'd terminal error event; the de-facto
			// convention (also used by OpenAI itself) is a final data frame
			// carrying an "error" object, which official SDKs translate into a
			// raised error. No [DONE] is sent, so truncation is unambiguous.
			errObj, _ := json.Marshal(map[string]interface{}{
				"error": map[string]interface{}{
					"message": truncateStreamError(streamErr.Error()),
					"type":    "upstream_stream_error",
				},
			})
			params.ResponseWriter.Write([]byte("data: " + string(errObj) + "\n\n"))
		} else {
			// Anthropic streams a first-class error event. Close any open content
			// block first so the lifecycle stays balanced for strict clients.
			if anthroOpenBlock != "" {
				if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
					params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
				}
				anthroOpenBlock = ""
			}
			errObj, _ := json.Marshal(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": truncateStreamError(streamErr.Error()),
				},
			})
			params.ResponseWriter.Write([]byte("event: error\ndata: " + string(errObj) + "\n\n"))
		}
		flusher.Flush()
	} else if params.InboundFormat == "openai" {
		if hasToolCalls {
			// Emit the reconstructed tool calls, then the standard [DONE] marker.
			if sse, fErr := openai.FormatOpenAIToolCallSSE(toolCalls, canReq.Model, anthroMsgID, time.Now().Unix()); fErr == nil && sse != "" {
				params.ResponseWriter.Write([]byte(sse))
			}
		}
		params.ResponseWriter.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	} else {
		// Ensure message_start is on the wire even if the upstream produced no
		// content deltas (e.g. it errored mid-stream), then close any still-open
		// block before ending the message lifecycle.
		emitAnthropicMessageStart()
		if anthroOpenBlock != "" {
			if data, err := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": anthroBlockIndex}); err == nil {
				params.ResponseWriter.Write([]byte("event: content_block_stop\ndata: " + string(data) + "\n\n"))
			}
			anthroOpenBlock = ""
		}
		// Map the upstream finish reason to Anthropic's stop_reason vocabulary.
		// Gemini reports raw "STOP"/"MAX_TOKENS"; Anthropic expects
		// end_turn/max_tokens/stop_sequence/tool_use.
		stopReason := lastFinishReason
		switch stopReason {
		case "STOP":
			stopReason = "end_turn"
		case "MAX_TOKENS":
			stopReason = "max_tokens"
		}
		if stopReason == "" {
			stopReason = "end_turn"
		}
		if hasToolCalls {
			// Reconstructed tool_use blocks, then stop_reason:"tool_use" (the
			// Anthropic analogue of OpenAI's finish_reason:"tool_calls").
			for _, tc := range toolCalls {
				emitAnthropicToolBlock(tc)
			}
			stopReason = "tool_use"
		}
		messageDelta := fmt.Sprintf(
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":%q,\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n",
			stopReason, totalCompletionTokens,
		)
		messageStop := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		params.ResponseWriter.Write([]byte(messageDelta))
		params.ResponseWriter.Write([]byte(messageStop))
		flusher.Flush()
	}

	var keyID, userID string
	if params.ApiKey != nil {
		keyID = params.ApiKey.ID.String()
		userID = params.ApiKey.UserID.String()
	}

	totalDurationMs := time.Since(startTime).Milliseconds()
	totalDurationSec := float64(totalDurationMs) / 1000.0
	ttftSec := float64(ttftMs) / 1000.0

	var tbtMs float64
	var tbtSec float64
	if totalCompletionTokens > 1 {
		tbtMs = float64(totalDurationMs-ttftMs) / float64(totalCompletionTokens-1)
		tbtSec = tbtMs / 1000.0
	}

	isGoodput := true
	var violationType string
	if g.sloMaxTTFTMs > 0 && int(ttftMs) > g.sloMaxTTFTMs {
		isGoodput = false
		violationType = "ttft"
	}
	if totalCompletionTokens > 1 && g.sloMaxTBTMs > 0 && int(tbtMs) > g.sloMaxTBTMs {
		isGoodput = false
		if violationType == "" {
			violationType = "tbt"
		} else {
			violationType = "both"
		}
	}

	costResult := g.costCalc.CalculateCost(target.Name, canReq.Model, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens, totalReasoningTokens)

	// Record mid-stream truncation in the usage log so it is distinguishable
	// from a healthy completion.
	errMsg := ""
	if streamErr != nil {
		errMsg = streamErr.Error()
	}
	g.recordTelemetry(params, target.Name, canReq.Model, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens, totalReasoningTokens, costResult, ttftMs, http.StatusOK, errMsg, startTime)

	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		telemetry.SetGenAIAttributes(span, target.Name, canReq.Model, "chat")
		telemetry.SetGenAIUsage(span, totalPromptTokens, totalCompletionTokens)
		telemetry.SetLLMGWAttributes(span, target.Name, false, fallbackUsed, ttftMs)
		if streamContent.Len() > 0 {
			telemetry.RecordCompletionEvent(span, g.contentCapturePolicy, streamContent.String())
		}
	}

	// Agent runaway loop detection
	totalToolCalls := inputToolCalls + len(toolCalls)
	if g.agentMaxToolCalls > 0 && totalToolCalls > g.agentMaxToolCalls {
		telemetry.RecordRunawayLoop(ctx, target.Name, canReq.Model, keyID, totalToolCalls)
		if span != nil && span.IsRecording() {
			telemetry.RecordRunawayLoopEvent(span, totalToolCalls, g.agentMaxToolCalls)
		}
	}

	// Metrics recording
	telemetry.RecordStreamSLOMetrics(ctx, target.Name, canReq.Model, keyID, ttftSec, tbtSec, isGoodput, violationType)
	telemetry.RecordDurationMetric(ctx, target.Name, canReq.Model, "chat", totalDurationSec, http.StatusOK)
	telemetry.RecordTokenMetrics(ctx, target.Name, canReq.Model, keyID, userID, totalPromptTokens, totalCompletionTokens, totalCachedInputTokens)
	telemetry.RecordCostMetric(ctx, target.Name, canReq.Model, keyID, userID, costResult.Cost)

	log.Printf("[stream] finished %s/%s (%s): ttft=%dms chunks=%d prompt_tokens=%d completion_tokens=%d duration=%s",
		target.Name, canReq.Model, params.InboundFormat, ttftMs, chunkCount, totalPromptTokens, totalCompletionTokens, time.Since(startTime).Round(time.Millisecond))

	return nil
}

// truncateStreamError bounds an upstream error string before it is echoed into a
// terminal SSE error frame, so a huge raw error body cannot bloat the frame.
func truncateStreamError(s string) string {
	const maxLen = 400
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
