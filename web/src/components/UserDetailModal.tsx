import { useCallback, useEffect, useState } from "react";
import { api, type CreateKeyResult } from "../api/client";
import type { APIKey, MemberDetailResponse, Tenant, UsageLog, User } from "../api/types";
import { fmtDate, fmtMoney, fmtNum, shortTs } from "../lib/format";
import Modal from "./Modal";
import LogDetailDrawer, { getStatusInfo } from "./LogDetailDrawer";
import { useToast } from "./Toast";
import { useLanguage } from "../i18n";

interface UserDetailModalProps {
  user: User;
  tenant?: Tenant;
  onClose: () => void;
  onUserUpdated?: () => void;
}

type SubTab = "dashboard" | "keys" | "logs" | "profile";
type TimeRange = "24h" | "7d" | "30d" | "90d";

export default function UserDetailModal({
  user,
  tenant,
  onClose,
  onUserUpdated,
}: UserDetailModalProps) {
  const { t } = useLanguage();
  const toast = useToast();
  const [activeTab, setActiveTab] = useState<SubTab>("dashboard");
  const [timeRange, setTimeRange] = useState<TimeRange>("7d");

  // Dashboard & Models state
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [detail, setDetail] = useState<MemberDetailResponse | null>(null);

  // Keys state
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loadingKeys, setLoadingKeys] = useState(false);
  const [newKeyModalOpen, setNewKeyModalOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [newKeyBudget, setNewKeyBudget] = useState("");
  const [newKeyRate, setNewKeyRate] = useState("");
  const [newKeyCacheTtl, setNewKeyCacheTtl] = useState("");
  const [createdKeyResult, setCreatedKeyResult] = useState<CreateKeyResult | null>(null);
  const [savingKey, setSavingKey] = useState(false);

  // Logs state
  const [logs, setLogs] = useState<UsageLog[]>([]);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [inspectLog, setInspectLog] = useState<UsageLog | null>(null);

  // General state
  const [copiedText, setCopiedText] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    setCopiedText(label);
    toast.info(`${label} copied to clipboard`, "[COPIED]");
    setTimeout(() => setCopiedText(null), 2000);
  };

  const getTimeDates = (r: TimeRange) => {
    const now = new Date();
    const to = now.toISOString();
    const fromDate = new Date();
    if (r === "24h") fromDate.setHours(now.getHours() - 24);
    else if (r === "7d") fromDate.setDate(now.getDate() - 7);
    else if (r === "30d") fromDate.setDate(now.getDate() - 30);
    else fromDate.setDate(now.getDate() - 90);
    return { from: fromDate.toISOString(), to };
  };

  // Load Dashboard Analytics
  const loadDetail = useCallback(async () => {
    setLoadingDetail(true);
    setError(null);
    try {
      const { from, to } = getTimeDates(timeRange);
      const res = await api.getMemberDetail(user.id, { from, to });
      setDetail(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load user analytics");
    } finally {
      setLoadingDetail(false);
    }
  }, [user.id, timeRange]);

  // Load API Keys
  const loadKeys = useCallback(async () => {
    setLoadingKeys(true);
    try {
      const res = await api.listKeys(user.id);
      setKeys(res);
    } catch (err) {
      console.error("Failed to load user keys", err);
    } finally {
      setLoadingKeys(false);
    }
  }, [user.id]);

  // Load Logs
  const loadLogs = useCallback(async () => {
    setLoadingLogs(true);
    try {
      const res = await api.logs({ user_id: user.id, limit: 50 });
      setLogs(res.data || []);
    } catch (err) {
      console.error("Failed to load user logs", err);
    } finally {
      setLoadingLogs(false);
    }
  }, [user.id]);

  useEffect(() => {
    if (activeTab === "dashboard") {
      loadDetail();
    } else if (activeTab === "keys") {
      loadKeys();
    } else if (activeTab === "logs") {
      loadLogs();
    }
  }, [activeTab, loadDetail, loadKeys, loadLogs]);

  // Handle creating a key directly for this user
  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKeyName.trim()) return;
    setSavingKey(true);
    setError(null);
    try {
      const res = await api.createKey({
        user_id: user.id,
        name: newKeyName.trim(),
        budget_limit: parseFloat(newKeyBudget) || 0,
        rate_limit_rpm: parseInt(newKeyRate, 10) || 0,
        cache_ttl_seconds: parseInt(newKeyCacheTtl, 10) || 0,
      });
      setCreatedKeyResult(res);
      toast.success(`Virtual key "${res.key_info?.name || newKeyName.trim()}" generated`, "[200 OK]");
      setNewKeyName("");
      setNewKeyBudget("");
      setNewKeyRate("");
      setNewKeyCacheTtl("");
      await loadKeys();
      if (onUserUpdated) onUserUpdated();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create API key";
      setError(msg);
      toast.error(msg, "[KEY GEN ERROR]");
    } finally {
      setSavingKey(false);
    }
  };

  const handleRevokeKey = async (keyId: string) => {
    if (!window.confirm("Are you sure you want to revoke this API key?")) return;
    try {
      await api.revokeKey(keyId);
      toast.info("Virtual API key revoked", "[200 OK]");
      await loadKeys();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to revoke key";
      setError(msg);
      toast.error(msg, "[REVOKE ERROR]");
    }
  };

  const memberSummary = detail?.member;
  const modelDetails = detail?.model_details || [];
  const dailyTrends = detail?.daily_trends || [];

  return (
    <>
      <Modal
        title={
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div
              style={{
                width: 32,
                height: 32,
                borderRadius: "50%",
                background: user.role === "admin" ? "rgba(168, 85, 247, 0.2)" : "rgba(34, 211, 238, 0.2)",
                color: user.role === "admin" ? "#c084fc" : "#22d3ee",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontWeight: 700,
                fontSize: 14,
              }}
            >
              {(user.name || user.email).charAt(0).toUpperCase()}
            </div>
            <div>
              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <span style={{ fontSize: 16, fontWeight: 700 }}>{user.name || "Unnamed User"}</span>
                {user.is_root ? (
                  <span className="terminal-badge terminal-badge--root glow-amber">
                    [ROOT ADMIN]
                  </span>
                ) : user.role === "admin" ? (
                  <span className="terminal-badge terminal-badge--admin">
                    [ADMIN]
                  </span>
                ) : (
                  <span className="terminal-badge terminal-badge--user">
                    [USER]
                  </span>
                )}
                {tenant && (
                  <span
                    className={`terminal-badge ${tenant.type === "internal" ? "terminal-badge--internal" : "terminal-badge--external"}`}
                  >
                    [{tenant.type.toUpperCase()}] {tenant.name} ({user.tenant_role || "member"})
                  </span>
                )}
              </div>
              <div className="mono muted" style={{ fontSize: 12, marginTop: 2 }}>
                {user.email}
              </div>
            </div>
          </div>
        }
        onClose={onClose}
        maxWidth="1000px"
        footer={
          <div style={{ display: "flex", justifyContent: "space-between", width: "100%", alignItems: "center" }}>
            <span className="mono muted" style={{ fontSize: 11 }}>
              User ID: {user.id}
            </span>
            <button className="btn" onClick={onClose}>
              {t("common.close", "Close")}
            </button>
          </div>
        }
      >
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {error && <div className="error-banner">{error}</div>}

          {/* Sub-tab Navigation */}
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              borderBottom: "1px solid var(--border)",
              paddingBottom: 12,
              flexWrap: "wrap",
              gap: 8,
            }}
          >
            <div style={{ display: "flex", gap: 6 }}>
              <button
                type="button"
                className="btn btn--sm"
                style={{
                  background: activeTab === "dashboard" ? "var(--primary)" : "var(--surface)",
                  color: activeTab === "dashboard" ? "#06282d" : "var(--text-muted)",
                  border: "1px solid var(--border)",
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontWeight: activeTab === "dashboard" ? 700 : 400,
                  boxShadow: activeTab === "dashboard" ? "0 0 10px rgba(34, 211, 238, 0.3)" : "none",
                }}
                onClick={() => setActiveTab("dashboard")}
              >
                <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                  insights
                </span>
                {t("user_detail.tab_dashboard", "Dashboard & Models")}
              </button>
              <button
                type="button"
                className="btn btn--sm"
                style={{
                  background: activeTab === "keys" ? "var(--primary)" : "var(--surface)",
                  color: activeTab === "keys" ? "#06282d" : "var(--text-muted)",
                  border: "1px solid var(--border)",
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontWeight: activeTab === "keys" ? 700 : 400,
                  boxShadow: activeTab === "keys" ? "0 0 10px rgba(34, 211, 238, 0.3)" : "none",
                }}
                onClick={() => setActiveTab("keys")}
              >
                <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                  key
                </span>
                {t("user_detail.tab_keys", "API Keys")} {keys.length > 0 && `(${keys.length})`}
              </button>
              <button
                type="button"
                className="btn btn--sm"
                style={{
                  background: activeTab === "logs" ? "var(--primary)" : "var(--surface)",
                  color: activeTab === "logs" ? "#06282d" : "var(--text-muted)",
                  border: "1px solid var(--border)",
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontWeight: activeTab === "logs" ? 700 : 400,
                  boxShadow: activeTab === "logs" ? "0 0 10px rgba(34, 211, 238, 0.3)" : "none",
                }}
                onClick={() => setActiveTab("logs")}
              >
                <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                  receipt_long
                </span>
                {t("user_detail.tab_logs", "Usage Logs")}
              </button>
              <button
                type="button"
                className="btn btn--sm"
                style={{
                  background: activeTab === "profile" ? "var(--primary)" : "var(--surface)",
                  color: activeTab === "profile" ? "#06282d" : "var(--text-muted)",
                  border: "1px solid var(--border)",
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontWeight: activeTab === "profile" ? 700 : 400,
                  boxShadow: activeTab === "profile" ? "0 0 10px rgba(34, 211, 238, 0.3)" : "none",
                }}
                onClick={() => setActiveTab("profile")}
              >
                <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                  badge
                </span>
                {t("user_detail.tab_profile", "Profile & IAM")}
              </button>
            </div>

            {/* Time range switcher (only active on dashboard tab) */}
            {activeTab === "dashboard" && (
              <div style={{ display: "flex", gap: 4, background: "var(--surface-high)", padding: 3, borderRadius: 6, border: "1px solid var(--border)" }}>
                {(["24h", "7d", "30d", "90d"] as TimeRange[]).map((r) => (
                  <button
                    key={r}
                    type="button"
                    style={{
                      border: "none",
                      background: timeRange === r ? "var(--primary)" : "transparent",
                      color: timeRange === r ? "#fff" : "var(--text-muted)",
                      padding: "3px 10px",
                      borderRadius: 4,
                      fontSize: 12,
                      fontWeight: timeRange === r ? 600 : 400,
                      cursor: "pointer",
                    }}
                    onClick={() => setTimeRange(r)}
                  >
                    {r.toUpperCase()}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* TAB 1: DASHBOARD & MODELS */}
          {activeTab === "dashboard" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              {loadingDetail ? (
                <div style={{ padding: 40, textAlign: "center", color: "var(--text-muted)" }}>
                  {t("common.loading", "Loading member analytics & model telemetry…")}
                </div>
              ) : (
                <>
                  {/* Top KPI Cards */}
                  <div
                    style={{
                      display: "grid",
                      gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))",
                      gap: 12,
                    }}
                  >
                    {/* Card 1: Total Spend */}
                    <div
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        padding: 14,
                      }}
                    >
                      <div className="muted" style={{ fontSize: 11, textTransform: "uppercase", fontWeight: 600 }}>
                        {t("user_detail.total_spend", "Total Spend")} ({timeRange.toUpperCase()})
                      </div>
                      <div style={{ fontSize: 22, fontWeight: 700, color: "var(--emerald)", marginTop: 4 }}>
                        {fmtMoney(memberSummary?.cost_usd || 0)}
                      </div>
                      <div style={{ marginTop: 8 }}>
                        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }} className="muted">
                          <span>{t("user_detail.budget_limit", "Budget Limit")}</span>
                          <span>
                            {memberSummary?.budget_limit && memberSummary.budget_limit > 0
                              ? fmtMoney(memberSummary.budget_limit, 2)
                              : t("user_detail.no_limit", "No limit")}
                          </span>
                        </div>
                        {memberSummary?.budget_limit && memberSummary.budget_limit > 0 ? (
                          <div
                            style={{
                              width: "100%",
                              height: 5,
                              background: "rgba(255,255,255,0.1)",
                              borderRadius: 3,
                              marginTop: 4,
                              overflow: "hidden",
                            }}
                          >
                            <div
                              style={{
                                width: `${Math.min(100, memberSummary.budget_used_percent)}%`,
                                height: "100%",
                                background:
                                  memberSummary.budget_used_percent > 90
                                    ? "var(--danger)"
                                    : memberSummary.budget_used_percent > 70
                                      ? "var(--amber)"
                                      : "var(--primary)",
                              }}
                            />
                          </div>
                        ) : null}
                      </div>
                    </div>

                    {/* Card 2: Total Requests */}
                    <div
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        padding: 14,
                      }}
                    >
                      <div className="muted" style={{ fontSize: 11, textTransform: "uppercase", fontWeight: 600 }}>
                        {t("user_detail.total_requests", "Total Requests")}
                      </div>
                      <div style={{ fontSize: 22, fontWeight: 700, marginTop: 4 }}>
                        {fmtNum(memberSummary?.requests || 0)}
                      </div>
                      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
                        {t("user_detail.prompt_tokens", "Prompt Tokens")}: {fmtNum(memberSummary?.prompt_tokens || 0)}
                      </div>
                    </div>

                    {/* Card 3: Completion Tokens */}
                    <div
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        padding: 14,
                      }}
                    >
                      <div className="muted" style={{ fontSize: 11, textTransform: "uppercase", fontWeight: 600 }}>
                        {t("user_detail.completion_tokens", "Output (Completion) Tokens")}
                      </div>
                      <div style={{ fontSize: 22, fontWeight: 700, color: "var(--primary)", marginTop: 4 }}>
                        {fmtNum(memberSummary?.completion_tokens || 0)}
                      </div>
                      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
                        Total: {fmtNum((memberSummary?.prompt_tokens || 0) + (memberSummary?.completion_tokens || 0))} tokens
                      </div>
                    </div>

                    {/* Card 4: Top Model */}
                    <div
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        padding: 14,
                      }}
                    >
                      <div className="muted" style={{ fontSize: 11, textTransform: "uppercase", fontWeight: 600 }}>
                        {t("user_detail.top_model", "Most Frequent Model")}
                      </div>
                      <div className="mono" style={{ fontSize: 16, fontWeight: 700, marginTop: 6, color: "#f59e0b" }}>
                        {modelDetails.length > 0 ? modelDetails[0].model : "—"}
                      </div>
                      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
                        {modelDetails.length > 0
                          ? `${fmtNum(modelDetails[0].requests)} reqs (${fmtMoney(modelDetails[0].cost_usd)})`
                          : t("user_detail.no_activity", "No activity in period")}
                      </div>
                    </div>
                  </div>

                  {/* Hay dùng những model nào: Detailed Model Breakdown */}
                  <div
                    style={{
                      background: "var(--surface)",
                      border: "1px solid var(--border)",
                      borderRadius: 8,
                      padding: 16,
                    }}
                  >
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                      <div>
                        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                          {t("user_detail.model_breakdown", "Model Breakdown & Spending")}
                        </h3>
                        <p className="muted" style={{ margin: "2px 0 0 0", fontSize: 12 }}>
                          {t("user_detail.model_breakdown_sub", "Models frequently used sorted by cost")}
                        </p>
                      </div>
                      <span className="badge badge--info">{modelDetails.length} {t("user_detail.models_used_badge", "model(s) used")}</span>
                    </div>

                    {modelDetails.length === 0 ? (
                      <div className="empty" style={{ padding: 24 }}>
                        {t("user_detail.no_models_period", "No models called by this user in the selected time range")} ({timeRange.toUpperCase()}).
                      </div>
                    ) : (
                      <div className="table-wrap">
                        <table className="table">
                          <thead>
                            <tr>
                              <th>{t("user_detail.col_model_name", "Model Name")}</th>
                              <th className="num">{t("dash.total_requests", "Requests")}</th>
                              <th style={{ width: 140 }}>{t("user_detail.col_activity_share", "Activity Share")}</th>
                              <th className="num">{t("user_detail.col_prompt_tk", "Prompt Tokens")}</th>
                              <th className="num">{t("user_detail.col_output_tk", "Output Tokens")}</th>
                              <th className="num">{t("user_detail.col_cost", "Cost (USD)")}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {modelDetails.map((m) => {
                              const totalReqs = memberSummary?.requests || 1;
                              const sharePct = ((m.requests / totalReqs) * 100).toFixed(1);
                              return (
                                <tr key={m.model}>
                                  <td className="mono" style={{ fontSize: 12, fontWeight: 600 }}>
                                    {m.model}
                                  </td>
                                  <td className="num">{fmtNum(m.requests)}</td>
                                  <td>
                                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                                      <div
                                        style={{
                                          flex: 1,
                                          height: 6,
                                          background: "rgba(255,255,255,0.08)",
                                          borderRadius: 3,
                                          overflow: "hidden",
                                        }}
                                      >
                                        <div
                                          style={{
                                            width: `${sharePct}%`,
                                            height: "100%",
                                            background: "var(--primary)",
                                          }}
                                        />
                                      </div>
                                      <span className="mono muted" style={{ fontSize: 11, width: 38 }}>
                                        {sharePct}%
                                      </span>
                                    </div>
                                  </td>
                                  <td className="num muted">{fmtNum(m.prompt_tokens)}</td>
                                  <td className="num muted">{fmtNum(m.completion_tokens)}</td>
                                  <td className="num" style={{ fontWeight: 600, color: "var(--emerald)" }}>
                                    {fmtMoney(m.cost_usd)}
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  {/* Daily Trend Activity */}
                  {dailyTrends.length > 0 && (
                    <div
                      style={{
                        background: "var(--surface)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        padding: 16,
                      }}
                    >
                      <h3 style={{ margin: "0 0 8px 0", fontSize: 14, fontWeight: 600 }}>
                        {t("user_detail.timeline_title", "Daily Request & Spend Timeline")}
                      </h3>
                      <div className="table-wrap">
                        <table className="table">
                          <thead>
                            <tr>
                              <th>{t("user_detail.col_date", "Date")}</th>
                              <th className="num">{t("dash.total_requests", "Requests")}</th>
                              <th className="num">{t("dash.total_cost", "Cost")}</th>
                              <th className="num">{t("user_detail.col_latency", "Avg Latency")}</th>
                              <th className="num">{t("user_detail.col_errors", "Errors")}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {dailyTrends.map((d) => (
                              <tr key={d.time_bucket}>
                                <td className="mono" style={{ fontSize: 12 }}>
                                  {new Date(d.time_bucket).toLocaleDateString("en-US", {
                                    weekday: "short",
                                    month: "short",
                                    day: "numeric",
                                  })}
                                </td>
                                <td className="num">{fmtNum(d.requests)}</td>
                                <td className="num" style={{ color: "var(--emerald)" }}>
                                  {fmtMoney(d.cost_usd)}
                                </td>
                                <td className="num muted">{d.avg_latency_ms} ms</td>
                                <td className="num">
                                  {d.errors > 0 ? (
                                    <span style={{ color: "var(--danger)", fontWeight: 600 }}>
                                      {d.errors}
                                    </span>
                                  ) : (
                                    <span className="muted">0</span>
                                  )}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* TAB 2: VIRTUAL KEYS */}
          {activeTab === "keys" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <div>
                  <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                    {t("user_detail.keys_title", "User Virtual Keys")}
                  </h3>
                  <p className="muted" style={{ margin: "2px 0 0 0", fontSize: 12 }}>
                    {t("user_detail.keys_sub", "Active and revoked gateway API keys belonging to this account")}
                  </p>
                </div>
                <button className="btn btn--primary btn--sm" onClick={() => setNewKeyModalOpen(true)}>
                  {t("user_detail.btn_generate_key", "+ Generate key for user")}
                </button>
              </div>

              {createdKeyResult && (
                <div
                  style={{
                    background: "rgba(16, 185, 129, 0.1)",
                    border: "1px solid rgba(16, 185, 129, 0.4)",
                    borderRadius: 8,
                    padding: 14,
                  }}
                >
                  <div style={{ color: "var(--emerald)", fontWeight: 600, fontSize: 13, marginBottom: 4 }}>
                    {t("user_detail.new_key_created", "New Key Created:")} {createdKeyResult.key_info.name}
                  </div>
                  <div className="muted" style={{ fontSize: 12, marginBottom: 8 }}>
                    {t("user_detail.copy_key_warning", "Copy this API key now. For security reasons, you will not be able to see it again.")}
                  </div>
                  <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                    <input
                      className="input mono"
                      readOnly
                      value={createdKeyResult.api_key}
                      style={{ flex: 1, fontSize: 12, background: "var(--surface-raised)" }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => copyToClipboard(createdKeyResult.api_key, "createdKey")}
                    >
                      {copiedText === "createdKey" ? t("common.copied", "Copied!") : t("user_detail.copy_key_btn", "Copy Key")}
                    </button>
                    <button className="btn btn--ghost btn--sm" onClick={() => setCreatedKeyResult(null)}>
                      {t("user_detail.dismiss", "Dismiss")}
                    </button>
                  </div>
                </div>
              )}

              {loadingKeys ? (
                <div style={{ padding: 40, textAlign: "center", color: "var(--text-muted)" }}>
                  {t("common.loading", "Loading…")}
                </div>
              ) : keys.length === 0 ? (
                <div className="empty" style={{ padding: 32 }}>
                  <p>{t("user_detail.no_keys", "This user has no API keys assigned yet.")}</p>
                  <button className="btn btn--primary btn--sm" onClick={() => setNewKeyModalOpen(true)}>
                    {t("user_detail.btn_first_key", "Generate First Key")}
                  </button>
                </div>
              ) : (
                <div className="table-wrap">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>{t("keys.col_name", "Key Name")}</th>
                        <th>{t("user_detail.col_prefix", "Prefix / Token")}</th>
                        <th>{t("common.status", "Status")}</th>
                        <th className="num">{t("user_detail.col_rpm", "Rate Limit")}</th>
                        <th className="num">{t("user_detail.col_budget", "Budget Limit")}</th>
                        <th className="num">{t("user_detail.col_cache", "Cache TTL")}</th>
                        <th>{t("keys.col_created", "Created")}</th>
                        <th style={{ textAlign: "right" }}>{t("common.actions", "Actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {keys.map((k) => (
                        <tr key={k.id}>
                          <td style={{ fontWeight: 600 }}>{k.name}</td>
                          <td>
                            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                              <span className="mono" style={{ fontSize: 12 }}>
                                {k.key_prefix}…****
                              </span>
                              <button
                                className="btn btn--ghost btn--sm"
                                style={{ padding: "0 4px", minHeight: 22 }}
                                title={t("common.copy", "Copy")}
                                onClick={() => copyToClipboard(k.key_prefix, k.id)}
                              >
                                {copiedText === k.id ? "✓" : t("common.copy", "copy")}
                              </button>
                            </div>
                          </td>
                          <td>
                            {k.is_active ? (
                              <span className="badge badge--ok">{t("keys.badge_active", "Active")}</span>
                            ) : (
                              <span className="badge badge--danger">{t("keys.badge_revoked", "Revoked")}</span>
                            )}
                          </td>
                          <td className="num muted">
                            {k.rate_limit_rpm > 0 ? `${k.rate_limit_rpm} RPM` : t("user_detail.no_limit", "Unlimited")}
                          </td>
                          <td className="num">
                            {k.budget_limit > 0 ? fmtMoney(k.budget_limit, 2) : t("user_detail.no_limit", "No limit")}
                          </td>
                          <td className="num muted">{k.cache_ttl_seconds}s</td>
                          <td className="muted">{fmtDate(k.created_at)}</td>
                          <td>
                            <div className="table__actions">
                              {k.is_active && (
                                <button
                                  className="btn btn--sm btn--danger"
                                  onClick={() => handleRevokeKey(k.id)}
                                >
                                  {t("user_detail.revoke_btn", "Revoke")}
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}

          {/* TAB 3: USAGE LOGS */}
          {activeTab === "logs" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <div>
                  <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                    {t("user_detail.logs_title", "User Request Logs")}
                  </h3>
                  <p className="muted" style={{ margin: "2px 0 0 0", fontSize: 12 }}>
                    {t("user_detail.logs_sub", "Showing recent requests executed")} {user.name || user.email}
                  </p>
                </div>
                <button className="btn btn--sm" onClick={loadLogs} disabled={loadingLogs}>
                  {loadingLogs ? t("common.refreshing", "Refreshing…") : `↻ ${t("common.refresh", "Refresh")}`}
                </button>
              </div>

              {loadingLogs ? (
                <div style={{ padding: 40, textAlign: "center", color: "var(--text-muted)" }}>
                  {t("common.loading", "Loading…")}
                </div>
              ) : logs.length === 0 ? (
                <div className="empty" style={{ padding: 32 }}>
                  {t("user_detail.no_logs", "No usage logs recorded for this user yet.")}
                </div>
              ) : (
                <div className="table-wrap">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>{t("common.time", "Time")}</th>
                        <th>{t("common.model", "Model")}</th>
                        <th>{t("common.status", "Status")}</th>
                        <th className="num">{t("logs.col_latency", "Latency")}</th>
                        <th className="num">{t("logs.col_tokens", "Tokens (P / C / Cache)")}</th>
                        <th className="num">{t("logs.col_cost", "Cost")}</th>
                        <th style={{ textAlign: "right" }}>{t("logs.inspect", "Inspect")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {logs.map((log) => {
                        const status = getStatusInfo(log.status_code);
                        return (
                          <tr key={log.id}>
                            <td className="mono muted" style={{ fontSize: 12 }}>
                              {shortTs(log.created_at)}
                            </td>
                            <td>
                              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                                <span className="badge badge--info">{log.provider}</span>
                                <span className="mono" style={{ fontSize: 12 }}>
                                  {log.model}
                                </span>
                              </div>
                            </td>
                            <td>
                              <span
                                className="mono"
                                style={{
                                  fontSize: 11,
                                  padding: "2px 6px",
                                  borderRadius: 4,
                                  background: status.bg,
                                  color: status.color,
                                  border: `1px solid ${status.border}`,
                                  fontWeight: 600,
                                }}
                              >
                                {log.status_code} {status.text}
                              </span>
                            </td>
                            <td className="num mono" style={{ fontSize: 12 }}>
                              {log.total_latency_ms} ms
                            </td>
                            <td className="num mono" style={{ fontSize: 11 }}>
                              <span>{fmtNum(log.prompt_tokens)}</span>
                              <span className="muted"> / </span>
                              <span>{fmtNum(log.completion_tokens)}</span>
                              {log.cached_input_tokens > 0 && (
                                <span style={{ color: "var(--emerald)", marginLeft: 4 }}>
                                  ({fmtNum(log.cached_input_tokens)}⚡)
                                </span>
                              )}
                            </td>
                            <td className="num" style={{ fontWeight: 600, color: "var(--emerald)" }}>
                              {fmtMoney(log.cost)}
                            </td>
                            <td>
                              <div className="table__actions">
                                <button
                                  className="btn btn--sm"
                                  style={{
                                    display: "inline-flex",
                                    alignItems: "center",
                                    gap: 4,
                                    fontSize: 11,
                                  }}
                                  onClick={() => setInspectLog(log)}
                                >
                                  <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                                    visibility
                                  </span>
                                  {t("logs.inspect", "Inspect")}
                                </button>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}

          {/* TAB 4: PROFILE & IAM */}
          {activeTab === "profile" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              <div
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: 8,
                  padding: 16,
                }}
              >
                <h3 style={{ margin: "0 0 12px 0", fontSize: 15, fontWeight: 600 }}>
                  {t("user_detail.account_props", "Account & IAM Properties")}
                </h3>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("users.col_user", "User ID")}
                    </label>
                    <div style={{ display: "flex", alignItems: "center", gap: 6, marginTop: 2 }}>
                      <span className="mono" style={{ fontSize: 12 }}>{user.id}</span>
                      <button
                        className="btn btn--ghost btn--sm"
                        style={{ padding: "0 4px", minHeight: 20 }}
                        onClick={() => copyToClipboard(user.id, "uid")}
                      >
                        {copiedText === "uid" ? t("common.copied", "Copied") : t("common.copy", "Copy")}
                      </button>
                    </div>
                  </div>

                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("common.email", "Email Address")}
                    </label>
                    <div className="mono" style={{ fontSize: 13, marginTop: 2 }}>
                      {user.email}
                    </div>
                  </div>

                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("users.col_name", "Display Name")}
                    </label>
                    <div style={{ fontSize: 13, marginTop: 2 }}>
                      {user.name || "—"}
                    </div>
                  </div>

                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("users.col_role", "System Role")}
                    </label>
                    <div style={{ marginTop: 2, display: "flex", gap: 6, alignItems: "center" }}>
                      <span className={`badge ${user.role === "admin" ? "badge--violet" : ""}`}>
                        {user.role}
                      </span>
                      {user.is_root && (
                        <span className="badge badge--warning">ROOT</span>
                      )}
                    </div>
                  </div>

                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("users.col_tenant", "Tenant / Organization")}
                    </label>
                    <div style={{ marginTop: 2 }}>
                      {tenant ? (
                        <span style={{ fontWeight: 600 }}>
                          {tenant.name} ({tenant.type} / {user.tenant_role || "member"})
                        </span>
                      ) : (
                        <span className="muted">{t("user_detail.global_no_tenant", "Global (No specific tenant)")}</span>
                      )}
                    </div>
                  </div>

                  <div>
                    <label className="muted" style={{ fontSize: 11, textTransform: "uppercase" }}>
                      {t("keys.col_created", "Account Created")}
                    </label>
                    <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                      {fmtDate(user.created_at)}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </Modal>

      {/* Modal for creating a new key for this user */}
      {newKeyModalOpen && (
        <Modal
          title={`${t("user_detail.create_modal_title", "Create API Key for")} ${user.name || user.email}`}
          onClose={() => setNewKeyModalOpen(false)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setNewKeyModalOpen(false)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={handleCreateKey}
                disabled={savingKey || !newKeyName.trim()}
              >
                {savingKey ? t("user_detail.btn_generating", "Generating…") : t("user_detail.btn_gen_key", "Generate Key")}
              </button>
            </>
          }
        >
          <form onSubmit={handleCreateKey}>
            <div className="field">
              <label className="field__label">{t("user_detail.key_name_label", "Key Name / Description")}</label>
              <input
                className="input"
                required
                placeholder={t("user_detail.key_name_placeholder", "e.g. CLI Production Key")}
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
              />
            </div>
            <div className="row">
              <div className="field" style={{ flex: 1 }}>
                <label className="field__label">{t("user_detail.budget_label", "Monthly Budget Limit ($ USD)")}</label>
                <input
                  className="input"
                  type="number"
                  min="0"
                  step="0.01"
                  placeholder={t("user_detail.unlimited_hint", "0 = Unlimited")}
                  value={newKeyBudget}
                  onChange={(e) => setNewKeyBudget(e.target.value)}
                />
              </div>
              <div className="field" style={{ flex: 1 }}>
                <label className="field__label">{t("user_detail.rate_label", "Rate Limit (RPM)")}</label>
                <input
                  className="input"
                  type="number"
                  min="0"
                  placeholder={t("user_detail.unlimited_hint", "0 = Unlimited")}
                  value={newKeyRate}
                  onChange={(e) => setNewKeyRate(e.target.value)}
                />
              </div>
            </div>
            <div className="field">
              <label className="field__label">{t("user_detail.cache_ttl_label", "Cache TTL (seconds)")}</label>
              <input
                className="input"
                type="number"
                min="0"
                placeholder={t("user_detail.default_cache_hint", "0 = Use gateway default")}
                value={newKeyCacheTtl}
                onChange={(e) => setNewKeyCacheTtl(e.target.value)}
              />
            </div>
          </form>
        </Modal>
      )}

      {/* Deep Log Inspector Drawer */}
      {inspectLog && (
        <LogDetailDrawer
          logId={inspectLog.id}
          initialLog={inspectLog}
          onClose={() => setInspectLog(null)}
        />
      )}
    </>
  );
}
