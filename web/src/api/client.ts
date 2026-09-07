import type {
  APIKey,
  AuditLog,
  AuditLogsPage,
  DashboardStats,
  Group,
  GroupPolicy,
  KeyModelPolicy,
  ModelPricing,
  ProviderKey,
  SystemInfo,
  UsageLogDetail,
  UsageLogsPage,
  User,
} from "./types";

const KEY_STORAGE = "llmgw_admin_key";

export function getAdminKey(): string | null {
  return localStorage.getItem(KEY_STORAGE);
}
export function setAdminKey(key: string) {
  localStorage.setItem(KEY_STORAGE, key);
}
export function clearAdminKey() {
  localStorage.removeItem(KEY_STORAGE);
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
  headers["X-Admin-API-Key"] = key ?? "";
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

function qs(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
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
  createUser: (u: { email: string; name: string; role?: string; is_root?: boolean }) =>
    request<User>("/admin/users", { method: "POST", body: JSON.stringify(u) }),
  updateUser: (id: string, patch: { name?: string; role?: string; is_root?: boolean }) =>
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

  // Group Members
  listGroupMembers: (groupId: string) =>
    request<User[]>(`/admin/groups/${groupId}/members`),
  addGroupMember: (groupId: string, userId: string) =>
    request<{ message: string }>(`/admin/groups/${groupId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId }),
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

  // System
  systemInfo: () => request<SystemInfo>("/admin/system/info"),
  health: () =>
    fetch("/health")
      .then((r) => r.json())
      .catch(() => ({ status: "down" })),
};
