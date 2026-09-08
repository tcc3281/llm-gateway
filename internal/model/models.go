package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"` // "admin" or "user"
	IsRoot       bool      `json:"is_root"`
	IsActive     bool      `json:"is_active"`
	PasswordHash string    `json:"-"` // secret, never serialized
	GroupRole    string    `json:"group_role,omitempty"` // "member" or "admin" when scoped to group context
	TenantID     *uuid.UUID `json:"tenant_id,omitempty"`
	TenantRole   string    `json:"tenant_role,omitempty"` // "tenant_admin", "member", "viewer"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type APIKey struct {
	ID                  uuid.UUID  `json:"id"`
	UserID              uuid.UUID  `json:"user_id"`
	TenantID            *uuid.UUID `json:"tenant_id,omitempty"`
	KeyPrefix           string     `json:"key_prefix"`
	KeyHash             string     `json:"-"`
	Name                string     `json:"name"`
	BudgetLimit         float64    `json:"budget_limit"`      // USD, 0 = unlimited
	RateLimitRPM        int        `json:"rate_limit_rpm"`    // Requests per minute
	CacheTTLSeconds     int        `json:"cache_ttl_seconds"`  // -1 = disabled, 0 = global default, >0 = custom TTL in seconds
	BudgetPeriod        *string    `json:"budget_period,omitempty"` // nil = lifetime, "daily", "weekly", "monthly"
	BudgetPeriodResetAt *time.Time `json:"budget_period_reset_at,omitempty"`
	IsActive            bool       `json:"is_active"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
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
	CacheReadPricePer1M  *float64  `json:"cache_read_price_per_1m,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UsageLog struct {
	ID                  int64     `json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	UserID              uuid.UUID `json:"user_id"`
	KeyID               uuid.UUID `json:"key_id"`
	TenantID            *uuid.UUID `json:"tenant_id,omitempty"`
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
	StatusCode          int           `json:"status_code"`
	ErrorMessage        string        `json:"error_message,omitempty"`
	RequestShape        *RequestShape `json:"request_shape,omitempty"`
}

// RequestShape captures content-agnostic structural telemetry without retaining raw text, PII, or media blobs.
type RequestShape struct {
	StructureHash     string   `json:"structure_hash"`               // Deterministic SHA-256 fingerprint of request structure
	MessageCount      int      `json:"message_count"`
	Roles             []string `json:"roles"`
	CharCount         int      `json:"char_count"`
	WordCount         int      `json:"word_count"`
	SystemPromptChars int      `json:"system_prompt_chars,omitempty"`
	HasImages         bool     `json:"has_images"`
	ImageCount        int      `json:"image_count"`
	ImageTotalBytes   int64    `json:"image_total_bytes"`
	ImageFormats      []string `json:"image_formats,omitempty"`
	HasAudio          bool     `json:"has_audio"`
	AudioCount        int      `json:"audio_count"`
	AudioTotalBytes   int64    `json:"audio_total_bytes"`
	AudioFormats      []string `json:"audio_formats,omitempty"`
	ToolsCount        int      `json:"tools_count"`
	ToolCallsCount    int      `json:"tool_calls_count"`
	ToolNames         []string `json:"tool_names,omitempty"`
	ToolParameterKeys []string `json:"tool_parameter_keys,omitempty"`
	HasReasoning      bool     `json:"has_reasoning"`
	ReasoningEffort   string   `json:"reasoning_effort,omitempty"`
	Modality          string   `json:"modality"` // "text", "vision", "audio", "multimodal", "tools", "embeddings"

	// Inference Hyperparameters & Execution Settings
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	Seed             *int     `json:"seed,omitempty"`
	StopSequences    []string `json:"stop_sequences,omitempty"`
	Stream           *bool    `json:"stream,omitempty"`
}

// ModalityBreakdown aggregates high-level request traffic types for dashboard analytics.
type ModalityBreakdown struct {
	TextOnly    int64 `json:"text_only"`
	Vision      int64 `json:"vision"`
	Audio       int64 `json:"audio"`
	ToolCalls   int64 `json:"tool_calls"`
	TotalImages int64 `json:"total_images"`
	TotalAudio  int64 `json:"total_audio"`
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
	ID             int64     `json:"id"`
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

// DashboardFilter carries optional criteria to scope the dashboard analytics.
type DashboardFilter struct {
	UserID   *uuid.UUID
	KeyID    *uuid.UUID
	Provider string
	Model    string
}

// UsageLogFilter carries the dynamic WHERE clauses for the paginated
// GET /admin/logs endpoint. Status accepts an exact code ("200") or a
// class ("2xx"/"4xx"/"5xx"); empty values are ignored.
type UsageLogFilter struct {
	From          *time.Time
	To            *time.Time
	UserID        *uuid.UUID
	KeyID         *uuid.UUID
	TenantID      *uuid.UUID
	Provider      string
	Model         string
	Status        string
	InboundFormat string
	Modality      string
	HasImages     *bool
	HasAudio      *bool
	StructureHash string
}

// UsageLogDetail enriches UsageLog with joined user email and api key name
// for high-density log inspection.
type UsageLogDetail struct {
	UsageLog
	UserEmail string `json:"user_email,omitempty"`
	KeyName   string `json:"key_name,omitempty"`
}

// Group represents a team or department for IAM permission evaluation and budget pooling.
type Group struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	BudgetLimitMonthly float64   `json:"budget_limit_monthly"` // USD, 0 = unlimited
	RateLimitRPM       int       `json:"rate_limit_rpm"`       // 0 = no override
	IsActive           bool      `json:"is_active"`
	MemberCount        int       `json:"member_count,omitempty"`
	PolicyCount        int       `json:"policy_count,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// GroupPolicy stores an AWS IAM-style policy JSON document attached to a group.
type GroupPolicy struct {
	ID        uuid.UUID       `json:"id"`
	GroupID   uuid.UUID       `json:"group_id"`
	Name      string          `json:"name"`
	Policy    json.RawMessage `json:"policy"`
	Priority  int             `json:"priority"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
}

// UserGroup links a user to a group.
type UserGroup struct {
	UserID    uuid.UUID `json:"user_id"`
	GroupID   uuid.UUID `json:"group_id"`
	GroupRole string    `json:"group_role"` // "member" or "admin"
	CreatedAt time.Time `json:"created_at"`
}

// KeyModelPolicy specifies per-key model allowlist/denylist rules.
type KeyModelPolicy struct {
	ID           uuid.UUID `json:"id"`
	KeyID        uuid.UUID `json:"key_id"`
	ModelPattern string    `json:"model_pattern"`
	IsAllowlist  bool      `json:"is_allowlist"` // true = allowlist, false = denylist
	CreatedAt    time.Time `json:"created_at"`
}

// AuditLog tracks administrative actions, mutations, and security events.
type AuditLog struct {
	ID         int64           `json:"id"`
	ActorID    *uuid.UUID      `json:"actor_id,omitempty"`
	ActorType  string          `json:"actor_type"` // "root", "admin", "user", "system"
	ActorIP    string          `json:"actor_ip,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type,omitempty"`
	TargetID   string          `json:"target_id,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuditLogFilter specifies query filters for audit log retrieval.
type AuditLogFilter struct {
	ActorType  string
	Action     string
	TargetType string
	Limit      int
	Offset     int
}

// Member & Team Usage Analytics models
type UserModelUsage struct {
	Model               string  `json:"model"`
	TotalRequests       int64   `json:"requests"`
	TotalCost           float64 `json:"cost_usd"`
	SumPromptTokens     int64   `json:"prompt_tokens"`
	SumCompletionTokens int64   `json:"completion_tokens"`
}

type MemberUsageSummary struct {
	UserID              uuid.UUID        `json:"user_id"`
	Email               string           `json:"email"`
	Name                string           `json:"name"`
	Role                string           `json:"role"`
	IsRoot              bool             `json:"is_root"`
	IsActive            bool             `json:"is_active"`
	Groups              []string         `json:"groups"`
	TopModels           []UserModelUsage `json:"top_models"`
	TotalRequests       int64            `json:"requests"`
	SumPromptTokens     int64            `json:"prompt_tokens"`
	SumCompletionTokens int64            `json:"completion_tokens"`
	TotalCost           float64          `json:"cost_usd"`
	BudgetLimit         float64          `json:"budget_limit"`
	BudgetUsedPercent   float64          `json:"budget_used_percent"`
}

type TeamUsageSummary struct {
	GroupID            uuid.UUID `json:"group_id"`
	GroupName          string    `json:"group_name"`
	Description        string    `json:"description"`
	MemberCount        int       `json:"member_count"`
	TotalRequests      int64     `json:"requests"`
	TotalCost          float64   `json:"cost_usd"`
	BudgetLimitMonthly float64   `json:"budget_limit_monthly"`
	UtilizationPercent float64   `json:"utilization_percent"`
}

type AnalyticsMembersResponse struct {
	Summary struct {
		ActiveMembers int64   `json:"active_members"`
		TotalSpend    float64 `json:"total_spend"`
		BudgetLimit   float64 `json:"budget_limit"`
		TopModel      string  `json:"top_model"`
		TotalTokens   int64   `json:"total_tokens"`
	} `json:"summary"`
	Members []MemberUsageSummary `json:"members"`
	Teams   []TeamUsageSummary   `json:"teams"`
}

type MemberDetailResponse struct {
	Member       MemberUsageSummary `json:"member"`
	ModelDetails []UserModelUsage   `json:"model_details"`
	DailyTrends  []UsageSeriesPoint `json:"daily_trends"`
}

// Tenant represents an external client organization or an internal department tenant.
type Tenant struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Type               string    `json:"type"` // "internal" | "external"
	BillingMode        string    `json:"billing_mode"` // "prepaid" | "postpaid"
	CreditBalance      float64   `json:"credit_balance"`
	BudgetLimitMonthly float64   `json:"budget_limit_monthly"`
	AllowedModels      []string  `json:"allowed_models"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TenantCreditTransaction records balance top-ups, contract deposits, and adjustments.
type TenantCreditTransaction struct {
	ID           int64      `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	Amount       float64    `json:"amount"`
	BalanceAfter float64    `json:"balance_after"`
	Type         string     `json:"type"` // "contract_deposit", "admin_adjustment", "usage_deduction", "refund"
	ContractRef  string     `json:"contract_ref,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type TopUpCreditRequest struct {
	Amount      float64 `json:"amount"`
	ContractRef string  `json:"contract_ref"`
	Notes       string  `json:"notes"`
}

type TenantPatch struct {
	Name               *string   `json:"name,omitempty"`
	Type               *string   `json:"type,omitempty"`
	BillingMode        *string   `json:"billing_mode,omitempty"`
	BudgetLimitMonthly *float64  `json:"budget_limit_monthly,omitempty"`
	AllowedModels      *[]string `json:"allowed_models,omitempty"`
	IsActive           *bool     `json:"is_active,omitempty"`
}



