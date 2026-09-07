// Typed mirrors of the Go admin API responses (internal/model/models.go).
// Fields marked secret in Go (key_hash, provider api_key) are never serialized
// and intentionally absent here.

export interface User {
  id: string;
  email: string;
  name: string;
  role: "admin" | "user";
  created_at: string;
  updated_at: string;
}

export interface APIKey {
  id: string;
  user_id: string;
  key_prefix: string;
  name: string;
  budget_limit: number;
  rate_limit_rpm: number;
  is_active: boolean;
  expires_at: string | null;
  created_at: string;
}

export interface ProviderKey {
  provider: string;
  key_preview: string;
  base_url?: string; // "" = built-in (hardcoded upstream URL)
  protocol?: string; // "openai" | "anthropic" for custom compatible providers
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface ModelPricing {
  id: string;
  provider: string;
  model_name: string;
  prompt_price_per_1m: number;
  completion_price_per_1m: number;
  updated_at: string;
}

export interface UsageTotal {
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  avg_ttft_ms: number;
  avg_latency_ms: number;
  errors: number;
  error_rate: number;
}

export interface UsageSeriesPoint {
  time_bucket: string;
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  errors: number;
  avg_ttft_ms: number;
  avg_latency_ms: number;
}

export interface UsageByModel {
  provider: string;
  model: string;
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  avg_latency_ms: number;
}

export interface UsageByUser {
  user_id: string;
  email: string;
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
}

export interface UsageRecent {
  created_at: string;
  user_id: string;
  provider: string;
  model: string;
  status_code: number;
  error_message?: string;
  cost_usd: number;
  ttft_ms: number;
  latency_ms: number;
}

export interface DashboardStats {
  from: string;
  to: string;
  interval: string;
  totals: UsageTotal;
  time_series: UsageSeriesPoint[];
  by_model: UsageByModel[];
  by_user: UsageByUser[];
  recent: UsageRecent[];
}

export interface UsageLog {
  id: number;
  created_at: string;
  user_id: string;
  key_id: string;
  inbound_format: string;
  provider: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cached_input_tokens: number;
  reasoning_tokens: number;
  prompt_price_used: number;
  completion_price_used: number;
  pricing_source: string;
  pricing_fallback: boolean;
  cost: number;
  ttft_ms: number;
  total_latency_ms: number;
  status_code: number;
  error_message?: string;
}

export interface UsageLogsPage {
  data: UsageLog[];
  total: number;
  limit: number;
  offset: number;
}

export interface SystemInfo {
  env: string;
  port: string;
  admin_port: string;
  db_host: string;
  db_port: string;
  db_name: string;
  db_user: string;
  db_sslmode: string;
  redis_host: string;
  admin_api_key_set: boolean;
  provider_master_key_enabled: boolean;
  admin_allowed_ips: string;
  version: string;
}
