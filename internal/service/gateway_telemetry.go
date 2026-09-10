package service

import (
	"fmt"
	"time"

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/model"
	"llm-gateway/internal/telemetry"

	"github.com/google/uuid"
)

func (g *GatewayService) recordTelemetry(params ProxyParams, provider, modelName string, promptTokens, completionTokens, cachedInputTokens, reasoningTokens int, costResult analytics.CostResult, ttftMs int64, statusCode int, errMsg string, startTime time.Time) {
	if g.telemetry == nil || params.ApiKey == nil {
		return
	}

	totalLatency := time.Since(startTime).Milliseconds()
	shape := telemetry.InspectPayloadShape(params.InboundFormat, params.RequestBody)

	logEntry := model.UsageLog{
		CreatedAt:           time.Now(),
		TenantID:            params.ApiKey.TenantID,
		UserID:              params.ApiKey.UserID,
		KeyID:               params.ApiKey.ID,
		InboundFormat:       params.InboundFormat,
		ClientIP:            params.ClientIP,
		Provider:            provider,
		Model:               modelName,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		TotalTokens:         promptTokens + completionTokens,
		CachedInputTokens:   cachedInputTokens,
		ReasoningTokens:     reasoningTokens,
		PromptPriceUsed:     costResult.PromptPriceUsed,
		CompletionPriceUsed: costResult.CompletionPriceUsed,
		PricingSource:       costResult.PricingSource,
		PricingFallback:     costResult.PricingFallback,
		Cost:                costResult.Cost,
		TTFTMs:              ttftMs,
		TotalLatencyMs:      totalLatency,
		StatusCode:          statusCode,
		ErrorMessage:        errMsg,
		RequestShape:        shape,
	}

	g.telemetry.Log(logEntry)

	// Deduct prepaid tenant credit if applicable
	if params.ApiKey.TenantID != nil && costResult.Cost > 0 && g.pg != nil {
		go func(tid uuid.UUID, cost float64) {
			_, _ = g.pg.DeductTenantCredit(tid, cost)
		}(*params.ApiKey.TenantID, costResult.Cost)
	}

	// Non-blocking alert evaluation
	if g.alertEngine != nil {
		if statusCode >= 500 {
			g.alertEngine.Emit(model.AlertEvent{
				EventType: "error_rate_surge",
				TenantID:  params.ApiKey.TenantID,
				Value:     1.0,
				Message:   fmt.Sprintf("Upstream failure (HTTP %d) on %s/%s: %s", statusCode, provider, modelName, errMsg),
				Metadata: map[string]interface{}{
					"provider":    provider,
					"model":       modelName,
					"status_code": statusCode,
					"error":       errMsg,
					"key_id":      params.ApiKey.ID.String(),
				},
				Timestamp: time.Now(),
			})
		}
		if totalLatency > 10000 {
			g.alertEngine.Emit(model.AlertEvent{
				EventType: "latency_anomaly",
				TenantID:  params.ApiKey.TenantID,
				Value:     float64(totalLatency),
				Message:   fmt.Sprintf("High latency anomaly on %s/%s: %dms", provider, modelName, totalLatency),
				Metadata: map[string]interface{}{
					"provider":   provider,
					"model":      modelName,
					"latency_ms": totalLatency,
					"key_id":     params.ApiKey.ID.String(),
				},
				Timestamp: time.Now(),
			})
		}
	}
}
