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
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	KeyPrefix     string     `json:"key_prefix"`
	KeyHash       string     `json:"-"`
	Name          string     `json:"name"`
	BudgetLimit   float64    `json:"budget_limit"`   // USD, 0 = unlimited
	RateLimitRPM  int        `json:"rate_limit_rpm"` // Requests per minute
	IsActive      bool       `json:"is_active"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ProviderKey struct {
	ID        uuid.UUID `json:"id"`
	Provider  string    `json:"provider"`
	APIKey    string    `json:"-"` // secret, never serialized
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

type ModelPricing struct {
	ID                   uuid.UUID `json:"id"`
	Provider             string    `json:"provider"`
	ModelName            string    `json:"model_name"`
	PromptPricePer1M     float64   `json:"prompt_price_per_1m"`
	CompletionPricePer1M float64   `json:"completion_price_per_1m"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UsageLog struct {
	ID              int64     `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	UserID          uuid.UUID `json:"user_id"`
	KeyID           uuid.UUID `json:"key_id"`
	InboundFormat   string    `json:"inbound_format"` // "openai" or "anthropic"
	Provider        string    `json:"provider"`       // "openai", "anthropic", "gemini"
	Model           string    `json:"model"`
	PromptTokens    int       `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	Cost            float64   `json:"cost"`
	TTFTMs          int64     `json:"ttft_ms"`           // Time to first token
	TotalLatencyMs  int64     `json:"total_latency_ms"`  // Total duration
	StatusCode      int       `json:"status_code"`
	ErrorMessage    string    `json:"error_message,omitempty"`
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
