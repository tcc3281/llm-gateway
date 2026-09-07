package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var (
	meter metric.Meter

	tokenUsageCounter    metric.Int64Counter
	operationDuration    metric.Float64Histogram
	timeToFirstToken     metric.Float64Histogram
	timeBetweenTokens    metric.Float64Histogram
	goodputCounter       metric.Int64Counter
	sloViolationCounter  metric.Int64Counter
	costCounter          metric.Float64Counter
	runawayLoopCounter   metric.Int64Counter
	cacheCounter         metric.Int64Counter
	fallbackCounter      metric.Int64Counter
)

// InitMetrics initializes the OpenTelemetry MeterProvider with a Prometheus bridge exporter.
func InitMetrics() error {
	exporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create Prometheus metric exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)
	otel.SetMeterProvider(provider)

	meter = provider.Meter(TracerName)

	// 1. Token Usage Counter: gen_ai.client.token.usage
	tokenUsageCounter, err = meter.Int64Counter(
		"gen_ai.client.token.usage",
		metric.WithDescription("Measures token usage of individual requests broken down by type"),
		metric.WithUnit("tokens"),
	)
	if err != nil {
		return err
	}

	// 2. Request Operation Duration: gen_ai.client.operation.duration (seconds)
	operationDuration, err = meter.Float64Histogram(
		"gen_ai.client.operation.duration",
		metric.WithDescription("Total duration of the GenAI request from receipt to full completion"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0),
	)
	if err != nil {
		return err
	}

	// 3. Time To First Token (TTFT): gen_ai.client.time_to_first_token (seconds)
	timeToFirstToken, err = meter.Float64Histogram(
		"gen_ai.client.time_to_first_token",
		metric.WithDescription("Time elapsed between sending request and receiving first stream chunk"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.2, 0.35, 0.5, 0.75, 1.0, 2.0, 5.0),
	)
	if err != nil {
		return err
	}

	// 4. Time Between Tokens (TBT / TPOT): gen_ai.client.time_between_tokens (seconds)
	timeBetweenTokens, err = meter.Float64Histogram(
		"gen_ai.client.time_between_tokens",
		metric.WithDescription("Average inter-token generation time during streaming response"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.025, 0.05, 0.1, 0.15, 0.25, 0.5),
	)
	if err != nil {
		return err
	}

	// 5. Goodput Counter: llmgw.goodput.requests.total
	goodputCounter, err = meter.Int64Counter(
		"llmgw.goodput.requests.total",
		metric.WithDescription("Count of requests satisfying both TTFT and TBT Service Level Objectives"),
	)
	if err != nil {
		return err
	}

	// 6. SLO Violation Counter: llmgw.slo_violation.total
	sloViolationCounter, err = meter.Int64Counter(
		"llmgw.slo_violation.total",
		metric.WithDescription("Count of requests violating TTFT or TBT thresholds"),
	)
	if err != nil {
		return err
	}

	// 7. Cost Counter: llmgw.cost.usd
	costCounter, err = meter.Float64Counter(
		"llmgw.cost.usd",
		metric.WithDescription("Accumulated estimated cost in USD"),
		metric.WithUnit("USD"),
	)
	if err != nil {
		return err
	}

	// 8. Runaway Loop Counter: llmgw.agent.runaway_loops.total
	runawayLoopCounter, err = meter.Int64Counter(
		"llmgw.agent.runaway_loops.total",
		metric.WithDescription("Count of detected agent infinite tool call loops"),
	)
	if err != nil {
		return err
	}

	// 9. Cache Requests: llmgw.cache.requests.total
	cacheCounter, err = meter.Int64Counter(
		"llmgw.cache.requests.total",
		metric.WithDescription("Total exact-match response cache hits and misses"),
	)
	if err != nil {
		return err
	}

	// 10. Fallback Requests: llmgw.fallback.requests.total
	fallbackCounter, err = meter.Int64Counter(
		"llmgw.fallback.requests.total",
		metric.WithDescription("Count of requests that triggered provider fallback ladder"),
	)
	if err != nil {
		return err
	}

	return nil
}

// PrometheusHandler exposes the standard Prometheus scrape handler.
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// RecordTokenMetrics records prompt, completion, and cached token metrics.
func RecordTokenMetrics(ctx context.Context, provider, model, keyID, userID string, promptTokens, completionTokens, cachedTokens int) {
	if tokenUsageCounter == nil {
		return
	}

	commonAttrs := []attribute.KeyValue{
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.response.model", model),
		attribute.String("llmgw.key_id", keyID),
		attribute.String("llmgw.user_id", userID),
	}

	if promptTokens > 0 {
		attrs := append(commonAttrs, attribute.String("gen_ai.token.type", "input"))
		tokenUsageCounter.Add(ctx, int64(promptTokens), metric.WithAttributes(attrs...))
	}
	if completionTokens > 0 {
		attrs := append(commonAttrs, attribute.String("gen_ai.token.type", "output"))
		tokenUsageCounter.Add(ctx, int64(completionTokens), metric.WithAttributes(attrs...))
	}
	if cachedTokens > 0 {
		attrs := append(commonAttrs, attribute.String("gen_ai.token.type", "cached"))
		tokenUsageCounter.Add(ctx, int64(cachedTokens), metric.WithAttributes(attrs...))
	}
}

// RecordDurationMetric records the total request duration in seconds.
func RecordDurationMetric(ctx context.Context, provider, model, operation string, durationSec float64, statusCode int) {
	if operationDuration == nil {
		return
	}
	operationDuration.Record(ctx, durationSec, metric.WithAttributes(
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.request.model", model),
		attribute.String("gen_ai.operation.name", operation),
		attribute.Int("http.status_code", statusCode),
	))
}

// RecordStreamSLOMetrics records TTFT, TBT, Goodput, and SLO violations for streaming responses.
func RecordStreamSLOMetrics(
	ctx context.Context,
	provider, model, keyID string,
	ttftSec float64,
	tbtSec float64,
	isGoodput bool,
	violationType string,
) {
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.response.model", model),
		attribute.String("llmgw.key_id", keyID),
	}

	if timeToFirstToken != nil && ttftSec > 0 {
		timeToFirstToken.Record(ctx, ttftSec, metric.WithAttributes(attrs...))
	}
	if timeBetweenTokens != nil && tbtSec > 0 {
		timeBetweenTokens.Record(ctx, tbtSec, metric.WithAttributes(attrs...))
	}

	if isGoodput && goodputCounter != nil {
		goodputCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	} else if violationType != "" && sloViolationCounter != nil {
		vAttrs := append(attrs, attribute.String("llmgw.violation_type", violationType))
		sloViolationCounter.Add(ctx, 1, metric.WithAttributes(vAttrs...))
	}
}

// RecordCostMetric records the cost of the request in USD.
func RecordCostMetric(ctx context.Context, provider, model, keyID, userID string, costUSD float64) {
	if costCounter == nil || costUSD <= 0 {
		return
	}
	costCounter.Add(ctx, costUSD, metric.WithAttributes(
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.response.model", model),
		attribute.String("llmgw.key_id", keyID),
		attribute.String("llmgw.user_id", userID),
	))
}

// RecordCacheMetric records a cache hit or miss.
func RecordCacheMetric(ctx context.Context, status string, model string) {
	if cacheCounter == nil {
		return
	}
	cacheCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("llmgw.cache_status", status),
		attribute.String("gen_ai.request.model", model),
	))
}

// RecordFallbackMetric records an invocation of provider fallback.
func RecordFallbackMetric(ctx context.Context, primaryProvider, fallbackProvider, model string) {
	if fallbackCounter == nil {
		return
	}
	fallbackCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("llmgw.primary_provider", primaryProvider),
		attribute.String("llmgw.fallback_provider", fallbackProvider),
		attribute.String("gen_ai.request.model", model),
	))
}

// RecordRunawayLoop records when an agent tool call runaway loop is detected.
func RecordRunawayLoop(ctx context.Context, provider, model, keyID string, count int) {
	if runawayLoopCounter == nil {
		return
	}
	runawayLoopCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("gen_ai.provider.name", provider),
		attribute.String("gen_ai.response.model", model),
		attribute.String("llmgw.key_id", keyID),
		attribute.Int("llmgw.tool_calls_count", count),
	))
}
