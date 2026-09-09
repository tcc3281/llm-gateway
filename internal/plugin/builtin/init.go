package builtin

import (
	"llm-gateway/internal/plugin"
)

func init() {
	// Traffic & Quota Management
	plugin.RegisterGlobal("rate-limiting", NewRateLimitingPlugin())
	plugin.RegisterGlobal("budget-control", NewBudgetControlPlugin())
	plugin.RegisterGlobal("canary-traffic-splitter", NewCanaryTrafficSplitterPlugin())

	// Security & Guardrails
	plugin.RegisterGlobal("pii-masking", NewPIIMaskingPlugin())
	plugin.RegisterGlobal("prompt-guard", NewPromptGuardPlugin())
	plugin.RegisterGlobal("ip-restriction", NewIPRestrictionPlugin())
	plugin.RegisterGlobal("content-moderation", NewContentModerationPlugin())

	// Performance & Caching
	plugin.RegisterGlobal("semantic-cache", NewSemanticCachePlugin())
	plugin.RegisterGlobal("request-hedging", NewRequestHedgingPlugin())
	plugin.RegisterGlobal("prompt-compression", NewPromptCompressionPlugin())

	// Resilience & Routing
	plugin.RegisterGlobal("header-transform", NewHeaderTransformPlugin())
	plugin.RegisterGlobal("retry-circuit-breaker", NewRetryCircuitBreakerPlugin())

	// Observability & Analytics
	plugin.RegisterGlobal("langfuse-exporter", NewLangfuseExporterPlugin())
	plugin.RegisterGlobal("audit-trail", NewAuditTrailPlugin())
	plugin.RegisterGlobal("prometheus-metrics", NewPrometheusMetricsPlugin())
	plugin.RegisterGlobal("webhook-notifier", NewWebhookNotifierPlugin())

	// Governance & Prompt Engineering
	plugin.RegisterGlobal("system-prompt-injector", NewSystemPromptInjectorPlugin())
	plugin.RegisterGlobal("response-watermarking", NewResponseWatermarkingPlugin())
}
