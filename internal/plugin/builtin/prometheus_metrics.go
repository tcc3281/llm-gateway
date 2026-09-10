package builtin

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

var (
	promMetricsOnce sync.Once

	promRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "llmgw",
			Subsystem: "plugin",
			Name:      "requests_total",
			Help:      "Total requests processed by LLM Gateway categorized by tenant, model, provider, and HTTP status.",
		},
		[]string{"tenant", "model", "provider", "status"},
	)

	promRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "llmgw",
			Subsystem: "plugin",
			Name:      "request_duration_seconds",
			Help:      "Duration of LLM Gateway requests in seconds.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
		},
		[]string{"tenant", "model", "provider", "status"},
	)

	promTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "llmgw",
			Subsystem: "plugin",
			Name:      "tokens_total",
			Help:      "Total tokens consumed categorized by tenant, model, provider, and token type (input/output/total).",
		},
		[]string{"tenant", "model", "provider", "type"},
	)
)

func registerPrometheusPluginMetrics() {
	promMetricsOnce.Do(func() {
		_ = prometheus.Register(promRequestsTotal)
		_ = prometheus.Register(promRequestDuration)
		_ = prometheus.Register(promTokensTotal)
	})
}

// PrometheusMetricsPlugin observes request latency, token consumption, and status codes.
type PrometheusMetricsPlugin struct{}

func NewPrometheusMetricsPlugin() *PrometheusMetricsPlugin {
	registerPrometheusPluginMetrics()
	return &PrometheusMetricsPlugin{}
}

func (p *PrometheusMetricsPlugin) Name() string {
	return "prometheus-metrics"
}

func (p *PrometheusMetricsPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostUpstream}
}

func (p *PrometheusMetricsPlugin) IsAsyncPostUpstream() bool {
	return true
}

func (p *PrometheusMetricsPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	recordLatency := getBoolConfig(cfg, "record_latency", true)
	recordTokens := getBoolConfig(cfg, "record_tokens", true)
	recordStatusCodes := getBoolConfig(cfg, "record_status_codes", true)
	includeTenant := getBoolConfig(cfg, "include_tenant_label", true)
	includeModel := getBoolConfig(cfg, "include_model_label", true)
	includeProvider := getBoolConfig(cfg, "include_provider_label", true)

	// Resolve Tenant label
	tenant := "anonymous"
	if pctx.Tenant != nil {
		if pctx.Tenant.Name != "" {
			tenant = pctx.Tenant.Name
		} else {
			tenant = pctx.Tenant.ID.String()
		}
	}
	if !includeTenant {
		tenant = "all"
	}

	// Resolve Model label
	model := pctx.Model
	if model == "" && pctx.CanReq != nil {
		model = pctx.CanReq.Model
	}
	if model == "" {
		model = "unknown"
	}
	if !includeModel {
		model = "all"
	}

	// Resolve Provider label
	provider := pctx.Provider
	if provider == "" {
		provider = "unknown"
	}
	if !includeProvider {
		provider = "all"
	}

	// Resolve Status code label
	status := "200"
	if pctx.StatusCode > 0 {
		status = strconv.Itoa(pctx.StatusCode)
	}

	// 1. Record status code counter
	if recordStatusCodes {
		promRequestsTotal.WithLabelValues(tenant, model, provider, status).Inc()
	}

	// 2. Record latency histogram
	if recordLatency && pctx.Duration > 0 {
		promRequestDuration.WithLabelValues(tenant, model, provider, status).Observe(pctx.Duration.Seconds())
	}

	// 3. Record token breakdown counter
	if recordTokens && resp != nil {
		if resp.PromptTokens > 0 {
			promTokensTotal.WithLabelValues(tenant, model, provider, "input").Add(float64(resp.PromptTokens))
		}
		if resp.CompletionTokens > 0 {
			promTokensTotal.WithLabelValues(tenant, model, provider, "output").Add(float64(resp.CompletionTokens))
		}
		if resp.TotalTokens > 0 {
			promTokensTotal.WithLabelValues(tenant, model, provider, "total").Add(float64(resp.TotalTokens))
		}
	}

	return nil
}
