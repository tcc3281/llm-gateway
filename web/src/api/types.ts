// Typed mirrors of the Go admin API responses (internal/model/models.go).
// Fields marked secret in Go (key_hash, provider api_key) are never serialized
// and intentionally absent here.

export interface User {
  id: string;
  email: string;
  name: string;
  role: "admin" | "user";
  is_root?: boolean;
  group_role?: "member" | "admin";
  tenant_id?: string | null;
  tenant_role?: string;
  created_at: string;
  updated_at: string;
}

export interface APIKey {
  id: string;
  user_id: string;
  tenant_id?: string | null;
  key_prefix: string;
  name: string;
  budget_limit: number;
  rate_limit_rpm: number;
  cache_ttl_seconds: number;
  budget_period?: string | null;
  budget_period_reset_at?: string | null;
  is_active: boolean;
  expires_at: string | null;
  created_at: string;
}

export interface ProviderKey {
  provider: string;
  key_preview: string;
  base_url?: string; // "" = built-in (hardcoded upstream URL)
  protocol?: string; // "openai" | "anthropic" for custom compatible providers
  fallback_priority?: number | null; // lower = tried first in fallback ladder
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
  cache_read_price_per_1m?: number | null;
  updated_at: string;
}

export interface UsageTotal {
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  reasoning_tokens: number;
  cache_savings_usd: number;
  cache_hit_rate: number;
  streaming_requests: number;
  avg_ttft_ms: number;
  avg_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  errors: number;
  error_rate: number;
}

export interface UsageSeriesPoint {
  time_bucket: string;
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  reasoning_tokens: number;
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
  cached_tokens: number;
  reasoning_tokens: number;
  avg_latency_ms: number;
  avg_ttft_ms: number;
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
  id?: number;
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

export interface ModalityBreakdown {
  text_only: number;
  vision: number;
  audio: number;
  tool_calls: number;
  total_images: number;
  total_audio: number;
}

export interface RequestShape {
  structure_hash: string;
  message_count: number;
  roles: string[];
  char_count: number;
  word_count: number;
  system_prompt_chars?: number;
  has_images: boolean;
  image_count: number;
  image_total_bytes: number;
  image_formats?: string[];
  has_audio: boolean;
  audio_count: number;
  audio_total_bytes: number;
  audio_formats?: string[];
  tools_count: number;
  tool_calls_count: number;
  tool_names?: string[];
  tool_parameter_keys?: string[];
  has_reasoning: boolean;
  reasoning_effort?: string;
  modality: string;
  temperature?: number;
  top_p?: number;
  top_k?: number;
  max_tokens?: number;
  presence_penalty?: number;
  frequency_penalty?: number;
  seed?: number;
  stop_sequences?: string[];
  stream?: boolean;
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
  status_breakdown?: Record<number, number>;
  modality_breakdown?: ModalityBreakdown;
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
  client_ip?: string;
  request_shape?: RequestShape;
}

export interface UsageLogDetail extends UsageLog {
  user_email?: string;
  key_name?: string;
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

export interface Group {
  id: string;
  name: string;
  description: string;
  budget_limit_monthly: number;
  rate_limit_rpm: number;
  is_active: boolean;
  member_count?: number;
  policy_count?: number;
  created_at: string;
  updated_at: string;
}

export interface GroupPolicy {
  id: string;
  group_id: string;
  name: string;
  policy: PolicyDocument | any;
  priority: number;
  is_active: boolean;
  created_at: string;
}

export interface ActionDef {
  action: string;
  namespace: string;
  label: string;
  description: string;
}

export interface PolicyStatement {
  Sid?: string;
  Effect: "Allow" | "Deny";
  Action: string[];
  Resource: string[];
}

export interface PolicyDocument {
  Version?: string;
  Statement: PolicyStatement[];
}

export interface KeyModelPolicy {
  id: string;
  key_id: string;
  model_pattern: string;
  is_allowlist: boolean;
  created_at: string;
}

export interface AuditLog {
  id: number;
  actor_id?: string;
  actor_type: string;
  actor_ip?: string;
  action: string;
  target_type?: string;
  target_id?: string;
  details?: any;
  created_at: string;
}

export interface AuditLogsPage {
  data: AuditLog[];
  total: number;
  limit: number;
  offset: number;
}

export interface UserModelUsage {
  model: string;
  requests: number;
  cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  tokens?: number;
}

export interface MemberUsageSummary {
  user_id: string;
  email: string;
  name: string;
  role: string;
  is_root: boolean;
  is_active: boolean;
  groups: string[];
  top_models: UserModelUsage[];
  requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost_usd: number;
  budget_limit: number;
  budget_used_percent: number;
}

export interface TeamUsageSummary {
  group_id: string;
  group_name: string;
  description: string;
  member_count: number;
  requests: number;
  cost_usd: number;
  budget_limit_monthly: number;
  utilization_percent: number;
}

export interface AnalyticsMembersResponse {
  summary: {
    active_members: number;
    total_spend: number;
    budget_limit: number;
    top_model: string;
    total_tokens: number;
  };
  members: MemberUsageSummary[];
  teams: TeamUsageSummary[];
}

export interface MemberDetailResponse {
  member: MemberUsageSummary;
  model_details: UserModelUsage[];
  daily_trends: UsageSeriesPoint[];
}

export interface LoginResponse {
  token: string;
  user: {
    id: string;
    email: string;
    name: string;
    role: string;
    is_root: boolean;
    groups: string[];
  };
}

export interface Tenant {
  id: string;
  name: string;
  type: "internal" | "external";
  billing_mode: "prepaid" | "postpaid";
  credit_balance: number;
  budget_limit_monthly: number;
  allowed_models: string[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface TenantCreditTransaction {
  id: number;
  tenant_id: string;
  amount: number;
  balance_after: number;
  type: "contract_deposit" | "admin_adjustment" | "usage_deduction" | "refund";
  contract_ref?: string;
  notes?: string;
  created_by?: string;
  created_at: string;
}

export interface TenantTransactionsPage {
  data: TenantCreditTransaction[];
  total: number;
  limit: number;
  offset: number;
}

export interface TopUpCreditRequest {
  amount: number;
  contract_ref?: string;
  notes?: string;
}

export interface TenantPatch {
  name?: string;
  type?: "internal" | "external";
  billing_mode?: "prepaid" | "postpaid";
  budget_limit_monthly?: number;
  allowed_models?: string[];
  is_active?: boolean;
}

export type AlertEventType = 
  | "budget_threshold"
  | "rate_limit_surge"
  | "error_rate_surge"
  | "latency_anomaly"
  | "iam_deny_surge";

export type AlertSeverity = "info" | "warning" | "critical";

export type AlertChannelType = "generic" | "slack" | "discord" | "pagerduty";

export interface AlertChannelConfig {
  channel_type: AlertChannelType;
  channel_name?: string;
  webhook_url: string;
  webhook_secret?: string;
  is_enabled: boolean;
}

export interface AlertRule {
  id: string;
  name: string;
  event_type: AlertEventType;
  severity?: AlertSeverity;
  channel_type: AlertChannelType;
  webhook_url: string;
  webhook_secret?: string;
  channels?: AlertChannelConfig[];
  threshold_value: number;
  window_seconds?: number;
  threshold_window_sec?: number;
  cooldown_seconds: number;
  cooldown_sec?: number;
  tenant_id?: string;
  tenant_name?: string;
  filter_provider?: string;
  filter_model?: string;
  is_active: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface AlertRulePatch {
  name?: string;
  event_type?: AlertEventType;
  severity?: AlertSeverity;
  channel_type?: AlertChannelType;
  webhook_url?: string;
  webhook_secret?: string;
  channels?: AlertChannelConfig[];
  threshold_value?: number;
  window_seconds?: number;
  threshold_window_sec?: number;
  cooldown_seconds?: number;
  cooldown_sec?: number;
  tenant_id?: string | null;
  filter_provider?: string;
  filter_model?: string;
  is_active?: boolean;
  metadata?: Record<string, unknown>;
}

export interface AlertHistory {
  id: string | number;
  rule_id?: string;
  rule_name?: string;
  event_type: string;
  tenant_id?: string;
  tenant_name?: string;
  title?: string;
  message?: string;
  triggered_at: string;
  status: "delivered" | "failed" | "suppressed_cooldown";
  payload?: Record<string, unknown>;
  payload_sent?: Record<string, unknown>;
  status_code?: number;
  response_code?: number;
  response_body?: string;
  error_message?: string;
  latency_ms?: number;
  retry_count?: number;
}

export interface AlertHistoryResponse {
  history: AlertHistory[];
  total?: number;
  dropped_events: number;
}

export interface TestWebhookRequest {
  webhook_url: string;
  channel_type: AlertChannelType;
  webhook_secret?: string;
  secret?: string;
}

export interface TestWebhookResponse {
  status_code: number;
  success: boolean;
  error?: string;
}

// ---------------------------------------------------------------------------
// Virtual Models & Dynamic Routing Types (v0.3.0)
// ---------------------------------------------------------------------------

export interface VirtualModelTarget {
  id?: number;
  virtual_model_id?: string;
  provider: string;
  model_name: string;
  weight: number;
  priority: number;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface VirtualModel {
  id: string;
  name: string;
  description: string;
  tenant_id?: string | null;
  routing_strategy: "weighted" | "fallback" | "round_robin";
  is_active: boolean;
  created_at: string;
  updated_at: string;
  targets: VirtualModelTarget[];
}

export interface CreateVirtualModelRequest {
  name: string;
  description?: string;
  tenant_id?: string | null;
  routing_strategy?: "weighted" | "fallback" | "round_robin";
  is_active?: boolean;
  targets: Array<{
    provider: string;
    model_name: string;
    weight: number;
    priority: number;
    is_active?: boolean;
  }>;
}

export interface UpdateVirtualModelRequest {
  name?: string;
  description?: string;
  tenant_id?: string | null;
  routing_strategy?: "weighted" | "fallback" | "round_robin";
  is_active?: boolean;
  targets?: Array<{
    provider: string;
    model_name: string;
    weight: number;
    priority: number;
    is_active?: boolean;
  }>;
}

export interface Plugin {
  id: string;
  name: string;
  display_name: string;
  description: string;
  category: "traffic" | "security" | "performance" | "observability" | string;
  hooks: string[];
  config_schema: Record<string, any>;
  is_builtin: boolean;
  version: string;
  created_at: string;
  updated_at: string;
}

export interface PluginBinding {
  id: string;
  plugin_name: string;
  scope_level: "global" | "provider" | "model" | "tenant" | "role" | "user" | "api_key";
  scope_provider?: string | null;
  scope_model?: string | null;
  scope_tenant_id?: string | null;
  scope_role?: string | null;
  scope_user_id?: string | null;
  scope_key_id?: string | null;
  config: Record<string, any>;
  is_active: boolean;
  priority: number;
  created_by?: string | null;
  created_at: string;
  updated_at: string;
  plugin_display_name?: string;
  category?: string;
}

export interface CreatePluginBindingRequest {
  plugin_name: string;
  scope_level: "global" | "provider" | "model" | "tenant" | "role" | "user" | "api_key";
  scope_provider?: string | null;
  scope_model?: string | null;
  scope_tenant_id?: string | null;
  scope_role?: string | null;
  scope_user_id?: string | null;
  scope_key_id?: string | null;
  config: Record<string, any>;
  is_active?: boolean;
  priority?: number;
}

export interface UpdatePluginBindingRequest {
  config?: Record<string, any>;
  is_active?: boolean;
  priority?: number;
}
