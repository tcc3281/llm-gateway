import type {
  ActionDef,
  AlertHistoryResponse,
  AlertRule,
  AlertRulePatch,
  AnalyticsMembersResponse,
  APIKey,
  AuditLogsPage,
  DashboardStats,
  Group,
  GroupPolicy,
  KeyModelPolicy,
  LoginResponse,
  MemberDetailResponse,
  ModelPricing,
  ProviderKey,
  SystemInfo,
  Tenant,
  TenantCreditTransaction,
  TenantPatch,
  TenantTransactionsPage,
  TestWebhookRequest,
  TestWebhookResponse,
  TopUpCreditRequest,
  UsageLogDetail,
  UsageLogsPage,
  User,
  VirtualModel,
  CreateVirtualModelRequest,
  UpdateVirtualModelRequest,
  Plugin,
  PluginBinding,
  CreatePluginBindingRequest,
  UpdatePluginBindingRequest,
} from "./types";

const KEY_STORAGE = "llmgw_admin_key";
const USER_STORAGE = "llmgw_user_info";

export function getAdminKey(): string | null {
  return localStorage.getItem(KEY_STORAGE);
}
export function setAdminKey(key: string) {
  localStorage.setItem(KEY_STORAGE, key);
}
export function clearAdminKey() {
  localStorage.removeItem(KEY_STORAGE);
  localStorage.removeItem(USER_STORAGE);
}

export function getStoredUser(): LoginResponse["user"] | null {
  const str = localStorage.getItem(USER_STORAGE);
  if (!str) return null;
  try {
    return JSON.parse(str);
  } catch {
    return null;
  }
}

export function setStoredUser(u: LoginResponse["user"]) {
  localStorage.setItem(USER_STORAGE, JSON.stringify(u));
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const key = getAdminKey();
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> | undefined),
  };
  if (key) {
    headers["X-Admin-API-Key"] = key;
    headers["Authorization"] = `Bearer ${key}`;
  }
  if (typeof options.body === "string") {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(path, { ...options, headers });
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function qs(params?: Record<string, string | number | undefined | null>): string {
  if (!params) return "";
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== "") q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

export interface CreateKeyResult {
  api_key: string;
  key_info: APIKey;
}

export const api = {
  // Users
  listUsers: () => request<User[]>("/admin/users"),
  createUser: (u: { email: string; name: string; role?: string; is_root?: boolean; tenant_id?: string | null; tenant_role?: string }) =>
    request<User>("/admin/users", { method: "POST", body: JSON.stringify(u) }),
  updateUser: (id: string, patch: { name?: string; role?: string; is_root?: boolean; tenant_id?: string | null; tenant_role?: string }) =>
    request<User>(`/admin/users?id=${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteUser: (id: string) =>
    request<{ message: string }>(`/admin/users?id=${id}`, { method: "DELETE" }),

  // API Keys
  listKeys: (userId?: string) =>
    request<APIKey[]>(`/admin/keys${qs({ user_id: userId })}`),
  createKey: (k: {
    user_id: string;
    name: string;
    budget_limit?: number;
    rate_limit_rpm?: number;
    cache_ttl_seconds?: number;
  }) =>
    request<CreateKeyResult>("/admin/keys", {
      method: "POST",
      body: JSON.stringify(k),
    }),
  updateKey: (
    id: string,
    patch: {
      name?: string;
      budget_limit?: number;
      rate_limit_rpm?: number;
      cache_ttl_seconds?: number;
      expires_at?: string | null;
    },
  ) =>
    request<{ message: string }>(`/admin/keys?id=${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  revokeKey: (id: string) =>
    request<{ message: string }>(`/admin/keys?id=${id}`, { method: "DELETE" }),
  reactivateKey: (id: string) =>
    request<{ message: string }>(`/admin/keys/reactivate?id=${id}`, {
      method: "POST",
    }),

  // Providers
  listProviders: () => request<ProviderKey[]>("/admin/providers"),
  setProvider: (p: {
    provider: string;
    api_key: string;
    base_url?: string;
    protocol?: string;
    fallback_priority?: number | null;
  }) =>
    request<{ message: string }>("/admin/providers", {
      method: "POST",
      body: JSON.stringify(p),
    }),
  deleteProvider: (provider: string) =>
    request<{ message: string }>(
      `/admin/providers?provider=${encodeURIComponent(provider)}`,
      { method: "DELETE" },
    ),

  // Models & pricing
  listModels: () => request<ModelPricing[]>("/admin/models"),
  upsertModel: (m: {
    provider: string;
    model_name: string;
    prompt_price_per_1m: number;
    completion_price_per_1m: number;
    cache_read_price_per_1m?: number | null;
  }) =>
    request<ModelPricing>("/admin/models", {
      method: "POST",
      body: JSON.stringify(m),
    }),
  deleteModel: (id: string) =>
    request<{ message: string }>(`/admin/models?id=${id}`, { method: "DELETE" }),
  syncModels: () =>
    request<{ status: string; synced_models: number }>("/admin/models/sync", {
      method: "POST",
    }),
  discoverModels: () =>
    request<{ status: string; synced_models: number; errors: Record<string, string> }>(
      "/admin/models/discover",
      { method: "POST" },
    ),

  // Analytics
  dashboard: (params: {
    from?: string;
    to?: string;
    interval?: string;
    user_id?: string;
    key_id?: string;
    provider?: string;
    model?: string;
    limit?: number;
  }) =>
    request<DashboardStats>(
      `/admin/analytics/dashboard${qs(params as Record<string, string | number | undefined>)}`,
    ),

  // Usage logs
  logs: (params: {
    from?: string;
    to?: string;
    user_id?: string;
    key_id?: string;
    provider?: string;
    model?: string;
    status?: string;
    inbound?: string;
    modality?: string;
    has_images?: boolean;
    has_audio?: boolean;
    structure_hash?: string;
    limit?: number;
    offset?: number;
  }) =>
    request<UsageLogsPage>(
      `/admin/logs${qs(params as Record<string, string | number | undefined>)}`,
    ),
  getLog: (id: number | string) => request<UsageLogDetail>(`/admin/logs/${id}`),

  // Key Model Policies
  listKeyModelPolicies: (keyId: string) =>
    request<KeyModelPolicy[]>(`/admin/keys/${keyId}/models`),
  createKeyModelPolicy: (keyId: string, p: { model_pattern: string; is_allowlist?: boolean }) =>
    request<KeyModelPolicy>(`/admin/keys/${keyId}/models`, {
      method: "POST",
      body: JSON.stringify(p),
    }),
  deleteKeyModelPolicy: (keyId: string, policyId: string) =>
    request<{ message: string }>(`/admin/keys/${keyId}/models/${policyId}`, {
      method: "DELETE",
    }),

  // Groups & IAM
  listGroups: () => request<Group[]>("/admin/groups"),
  createGroup: (g: {
    name: string;
    description?: string;
    budget_limit_monthly?: number;
    rate_limit_rpm?: number;
    is_active?: boolean;
  }) =>
    request<Group>("/admin/groups", {
      method: "POST",
      body: JSON.stringify(g),
    }),
  getGroup: (id: string) => request<Group>(`/admin/groups/${id}`),
  updateGroup: (id: string, patch: Partial<Group>) =>
    request<Group>(`/admin/groups/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteGroup: (id: string) =>
    request<{ message: string }>(`/admin/groups/${id}`, { method: "DELETE" }),

  // Group Policies
  listGroupPolicies: (groupId: string) =>
    request<GroupPolicy[]>(`/admin/groups/${groupId}/policies`),
  createGroupPolicy: (
    groupId: string,
    p: { name: string; policy: any; priority?: number; is_active?: boolean },
  ) =>
    request<GroupPolicy>(`/admin/groups/${groupId}/policies`, {
      method: "POST",
      body: JSON.stringify(p),
    }),
  updateGroupPolicy: (
    groupId: string,
    policyId: string,
    p: { name?: string; policy: any; priority?: number; is_active?: boolean },
  ) =>
    request<{ message: string }>(`/admin/groups/${groupId}/policies/${policyId}`, {
      method: "PATCH",
      body: JSON.stringify(p),
    }),
  deleteGroupPolicy: (groupId: string, policyId: string) =>
    request<{ message: string }>(`/admin/groups/${groupId}/policies/${policyId}`, {
      method: "DELETE",
    }),
  getIAMActions: () => request<ActionDef[]>("/admin/iam/actions"),

  // Group Members
  listGroupMembers: (groupId: string) =>
    request<User[]>(`/admin/groups/${groupId}/members`),
  addGroupMember: (groupId: string, userId: string, role?: "member" | "admin") =>
    request<{ message: string }>(`/admin/groups/${groupId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId, role: role || "member" }),
    }),
  setGroupMemberRole: (groupId: string, userId: string, role: "member" | "admin") =>
    request<{ message: string }>(`/admin/groups/${groupId}/members/${userId}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    }),
  removeGroupMember: (groupId: string, userId: string) =>
    request<{ message: string }>(`/admin/groups/${groupId}/members/${userId}`, {
      method: "DELETE",
    }),

  // Audit Logs
  auditLogs: (params: {
    actor_type?: string;
    action?: string;
    target_type?: string;
    limit?: number;
    offset?: number;
  }) =>
    request<AuditLogsPage>(
      `/admin/audit-logs${qs(params as Record<string, string | number | undefined>)}`,
    ),

  // Auth & Profile
  login: (email: string, password: string) =>
    request<LoginResponse>("/admin/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  getCurrentUser: () => request<User>("/admin/auth/me"),
  setPassword: (userId: string, password: string) =>
    request<{ message: string }>("/admin/auth/password", {
      method: "POST",
      body: JSON.stringify({ user_id: userId, password }),
    }),

  // Member & Team Analytics ("ai dùng gì, dùng bao nhiêu")
  getMemberAnalytics: (params?: {
    from?: string;
    to?: string;
    group_id?: string;
    search?: string;
  }) =>
    request<AnalyticsMembersResponse>(
      `/admin/analytics/members${qs(params as Record<string, string | number | undefined>)}`,
    ),
  getMemberDetail: (userId: string, params?: { from?: string; to?: string }) =>
    request<MemberDetailResponse>(
      `/admin/analytics/members/${userId}${qs(params as Record<string, string | number | undefined>)}`,
    ),

  // Tenants (Multi-Tenant B2B & Pre-paid Credits)
  listTenants: () => request<Tenant[]>("/admin/tenants"),
  createTenant: (data: {
    name: string;
    type?: "internal" | "external";
    billing_mode?: "prepaid" | "postpaid";
    budget_limit_monthly?: number;
    allowed_models?: string[];
    is_active?: boolean;
  }) =>
    request<Tenant>("/admin/tenants", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  getTenant: (id: string) => request<Tenant>(`/admin/tenants/${id}`),
  updateTenant: (id: string, patch: TenantPatch) =>
    request<Tenant>(`/admin/tenants/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteTenant: (id: string) =>
    request<{ status: string }>(`/admin/tenants/${id}`, {
      method: "DELETE",
    }),
  topUpTenantCredit: (id: string, req: TopUpCreditRequest) =>
    request<TenantCreditTransaction>(`/admin/tenants/${id}/topup`, {
      method: "POST",
      body: JSON.stringify(req),
    }),
  listTenantTransactions: (id: string, params?: { limit?: number; offset?: number }) =>
    request<TenantTransactionsPage>(
      `/admin/tenants/${id}/transactions${qs(params as Record<string, string | number | undefined>)}`,
    ),

  // Alerting & Webhooks
  listAlertRules: (params?: { tenant_id?: string; event_type?: string }) =>
    request<AlertRule[]>(
      `/admin/alerts/rules${qs(params as Record<string, string | number | undefined>)}`,
    ),
  createAlertRule: (data: Partial<AlertRule>) =>
    request<AlertRule>("/admin/alerts/rules", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateAlertRule: (id: string, patch: AlertRulePatch) =>
    request<AlertRule>(`/admin/alerts/rules/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteAlertRule: (id: string) =>
    request<{ message: string }>(`/admin/alerts/rules/${id}`, {
      method: "DELETE",
    }),
  listAlertHistory: (params?: { rule_id?: string; limit?: number; offset?: number }) =>
    request<AlertHistoryResponse>(
      `/admin/alerts/history${qs(params as Record<string, string | number | undefined>)}`,
    ),
  testAlertWebhook: (data: TestWebhookRequest) =>
    request<TestWebhookResponse>("/admin/alerts/test", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Virtual Models & Dynamic Routing (v0.3.0)
  listVirtualModels: (params?: { tenant_id?: string }) =>
    request<VirtualModel[]>(`/admin/virtual-models${qs(params as Record<string, string | number | undefined>)}`),
  getVirtualModel: (id: string) =>
    request<VirtualModel>(`/admin/virtual-models/${id}`),
  createVirtualModel: (data: CreateVirtualModelRequest) =>
    request<VirtualModel>("/admin/virtual-models", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateVirtualModel: (id: string, data: UpdateVirtualModelRequest) =>
    request<VirtualModel>(`/admin/virtual-models/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteVirtualModel: (id: string) =>
    request<{ status: string }>(`/admin/virtual-models/${id}`, {
      method: "DELETE",
    }),

  // Plugins & Extensibility (v0.3.0)
  listPlugins: () => request<Plugin[]>("/admin/plugins"),
  getPlugin: (name: string) => request<Plugin>(`/admin/plugins/${name}`),
  listPluginBindings: (params?: { tenant_id?: string; plugin_name?: string }) =>
    request<PluginBinding[]>(
      `/admin/plugin-bindings${qs(params as Record<string, string | number | undefined>)}`,
    ),
  getPluginBinding: (id: string) => request<PluginBinding>(`/admin/plugin-bindings/${id}`),
  createPluginBinding: (data: CreatePluginBindingRequest) =>
    request<PluginBinding>("/admin/plugin-bindings", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updatePluginBinding: (id: string, data: UpdatePluginBindingRequest) =>
    request<PluginBinding>(`/admin/plugin-bindings/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deletePluginBinding: (id: string) =>
    request<{ status: string }>(`/admin/plugin-bindings/${id}`, {
      method: "DELETE",
    }),

  // System
  systemInfo: () => request<SystemInfo>("/admin/system/info"),
  health: () =>
    fetch("/health")
      .then((r) => r.json())
      .catch(() => ({ status: "down" })),
};

