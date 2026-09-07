package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"` // "admin" or "user"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIKey struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	KeyPrefix       string     `json:"key_prefix"`
	KeyHash         string     `json:"-"`
	Name            string     `json:"name"`
	BudgetLimit     float64    `json:"budget_limit"`     // USD, 0 = unlimited
	RateLimitRPM    int        `json:"rate_limit_rpm"`   // Requests per minute
	CacheTTLSeconds int        `json:"cache_ttl_seconds"` // -1 = disabled, 0 = global default, >0 = custom TTL in seconds
	IsActive        bool       `json:"is_active"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ProviderKey struct {
	ID               uuid.UUID `json:"id"`
	Provider         string    `json:"provider"`
	APIKey           string    `json:"-"` // secret, never serialized
	BaseURL          string    `json:"base_url,omitempty"`          // "" = built-in (hardcoded upstream)
	Protocol         string    `json:"protocol,omitempty"`          // "openai" | "anthropic" for custom providers
	FallbackPriority *int      `json:"fallback_priority,omitempty"` // lower = higher priority, nil = excluded from fallback
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ProviderModel is a normalized entry for the public GET /v1/models endpoint.
// It serializes in the OpenAI-compatible shape; Provider is metadata only.
type ProviderModel struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Created  int64  `json:"created"`
	OwnedBy  string `json:"owned_by"`
	Provider string `json:"-"`
}

// CatalogModel is one (provider, model) row of the discovered catalog
// (provider_models). It is the routing source of truth for models the gateway
// has fetched from a provider's /models API — distinct from ModelPricing, which
// only carries explicit prices for cost calculation.
type CatalogModel struct {
	Provider  string    `json:"provider"`
	ModelName string    `json:"model_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelPricing struct {
	ID                   uuid.UUID `json:"id"`
	Provider             string    `json:"provider"`
	ModelName            string    `json:"model_name"`
	PromptPricePer1M     float64   `json:"prompt_price_per_1m"`
	CompletionPricePer1M float64   `json:"completion_price_per_1m"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UsageLog struct {
	ID                  int64     `json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	UserID              uuid.UUID `json:"user_id"`
	KeyID               uuid.UUID `json:"key_id"`
	InboundFormat       string    `json:"inbound_format"` // "openai" or "anthropic"
	Provider            string    `json:"provider"`       // "openai", "anthropic", "gemini"
	Model               string    `json:"model"`
	PromptTokens        int       `json:"prompt_tokens"`
	CompletionTokens    int       `json:"completion_tokens"`
	TotalTokens         int       `json:"total_tokens"`
	CachedInputTokens   int       `json:"cached_input_tokens"`
	ReasoningTokens     int       `json:"reasoning_tokens"`
	PromptPriceUsed     float64   `json:"prompt_price_used"`
	CompletionPriceUsed float64   `json:"completion_price_used"`
	PricingSource       string    `json:"pricing_source"`
	PricingFallback     bool      `json:"pricing_fallback"`
	Cost                float64   `json:"cost"`
	TTFTMs              int64     `json:"ttft_ms"`          // Time to first token
	TotalLatencyMs      int64     `json:"total_latency_ms"` // Total duration
	StatusCode          int       `json:"status_code"`
	ErrorMessage        string    `json:"error_message,omitempty"`
}

type UsageStats struct {
	TimeBucket       string  `json:"time_bucket"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	TotalRequests    int64   `json:"total_requests"`
	SumPromptTokens  int64   `json:"sum_prompt_tokens"`
	SumCompletionTokens int64 `json:"sum_completion_tokens"`
	TotalCost        float64 `json:"total_cost"`
	AvgTTFTMs        float64 `json:"avg_ttft_ms"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
}

// Dashboard analytics payloads, served by GET /admin/analytics/dashboard so a
// custom dashboard can render usage without Prometheus/Grafana.

type UsageTotal struct {
	TotalRequests       int64   `json:"requests"`
	TotalCost           float64 `json:"cost_usd"`
	SumPromptTokens     int64   `json:"prompt_tokens"`
	SumCompletionTokens int64   `json:"completion_tokens"`
	SumCachedTokens     int64   `json:"cached_tokens"`
	SumReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheSavingsUSD     float64 `json:"cache_savings_usd"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
	StreamingRequests   int64   `json:"streaming_requests"`
	AvgTTFTMs           float64 `json:"avg_ttft_ms"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	P50LatencyMs        float64 `json:"p50_latency_ms"`
	P95LatencyMs        float64 `json:"p95_latency_ms"`
	P99LatencyMs        float64 `json:"p99_latency_ms"`
	ErrorCount          int64   `json:"errors"`
	ErrorRate           float64 `json:"error_rate"`
}

type UsageSeriesPoint struct {
	TimeBucket          string  `json:"time_bucket"`
	TotalRequests       int64   `json:"requests"`
	TotalCost           float64 `json:"cost_usd"`
	SumPromptTokens     int64   `json:"prompt_tokens"`
	SumCompletionTokens int64   `json:"completion_tokens"`
	SumCachedTokens     int64   `json:"cached_tokens"`
	SumReasoningTokens  int64   `json:"reasoning_tokens"`
	ErrorCount          int64   `json:"errors"`
	AvgTTFTMs           float64 `json:"avg_ttft_ms"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
}

type UsageByModel struct {
	Provider            string  `json:"provider"`
	Model               string  `json:"model"`
	TotalRequests       int64   `json:"requests"`
	TotalCost           float64 `json:"cost_usd"`
	SumPromptTokens     int64   `json:"prompt_tokens"`
	SumCompletionTokens int64   `json:"completion_tokens"`
	SumCachedTokens     int64   `json:"cached_tokens"`
	SumReasoningTokens  int64   `json:"reasoning_tokens"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	AvgTTFTMs           float64 `json:"avg_ttft_ms"`
}

type UsageByUser struct {
	UserID              string  `json:"user_id"`
	Email               string  `json:"email"`
	TotalRequests       int64   `json:"requests"`
	TotalCost           float64 `json:"cost_usd"`
	SumPromptTokens     int64   `json:"prompt_tokens"`
	SumCompletionTokens int64   `json:"completion_tokens"`
}

type UsageRecent struct {
	CreatedAt      time.Time `json:"created_at"`
	UserID         uuid.UUID `json:"user_id"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	StatusCode     int       `json:"status_code"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	Cost           float64   `json:"cost_usd"`
	TTFTMs         int64     `json:"ttft_ms"`
	TotalLatencyMs int64     `json:"latency_ms"`
}

// UsageLogFilter carries the dynamic WHERE clauses for the paginated
// GET /admin/logs endpoint. Status accepts an exact code ("200") or a
// class ("2xx"/"4xx"/"5xx"); empty values are ignored.
type UsageLogFilter struct {
	From          *time.Time
	To            *time.Time
	UserID        *uuid.UUID
	Provider      string
	Model         string
	Status        string
	InboundFormat string
}
