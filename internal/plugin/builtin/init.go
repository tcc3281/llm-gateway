package builtin

import (
	"llm-gateway/internal/plugin"
)

func init() {
	// Traffic & Quota Management
	plugin.RegisterGlobal("rate-limiting", NewRateLimitingPlugin())
	plugin.RegisterGlobal("budget-control", NewBudgetControlPlugin())

	// Security & Guardrails
	plugin.RegisterGlobal("pii-masking", NewPIIMaskingPlugin())
	plugin.RegisterGlobal("prompt-guard", NewPromptGuardPlugin())

	// Performance & Caching
	plugin.RegisterGlobal("semantic-cache", NewSemanticCachePlugin())
	plugin.RegisterGlobal("request-hedging", NewRequestHedgingPlugin())

	// Resilience & Routing
	plugin.RegisterGlobal("header-transform", NewHeaderTransformPlugin())
	plugin.RegisterGlobal("retry-circuit-breaker", NewRetryCircuitBreakerPlugin())

	// Observability & Analytics
	plugin.RegisterGlobal("langfuse-exporter", NewLangfuseExporterPlugin())
	plugin.RegisterGlobal("audit-trail", NewAuditTrailPlugin())
}
