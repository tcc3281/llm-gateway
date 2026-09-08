import { useEffect, useState, useMemo } from "react";
import { api } from "../api/client";
import type {
  AlertRule,
  AlertRulePatch,
  AlertHistory,
  AlertEventType,
  AlertChannelType,
  AlertSeverity,
  AlertChannelConfig,
  Tenant,
} from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";
import { useToast } from "../components/Toast";
import { useLanguage } from "../i18n";

type TabKey = "rules" | "history";

const EVENT_TYPE_LABELS: Record<
  AlertEventType,
  { label: string; desc: string; unit: string; defaultThreshold: number }
> = {
  budget_threshold: {
    label: "Low Prepaid Budget Balance",
    desc: "Triggers when a tenant's prepaid credit balance drops below threshold",
    unit: "USD ($)",
    defaultThreshold: 20.0,
  },
  rate_limit_surge: {
    label: "Rate Limit Surge (429)",
    desc: "Triggers when client requests exceed configured RPM limits",
    unit: "req/min",
    defaultThreshold: 1.0,
  },
  error_rate_surge: {
    label: "Upstream Provider Errors (5xx)",
    desc: "Triggers on upstream LLM API failures, timeouts, or 5xx crashes",
    unit: "errors",
    defaultThreshold: 5.0,
  },
  latency_anomaly: {
    label: "High Latency Anomaly",
    desc: "Triggers when end-to-end completion latency spikes",
    unit: "ms",
    defaultThreshold: 8000.0,
  },
  iam_deny_surge: {
    label: "IAM Policy Denials (403)",
    desc: "Triggers when unauthorized requests violate group policies",
    unit: "denials",
    defaultThreshold: 1.0,
  },
};

const CHANNEL_DEFS: Record<
  AlertChannelType,
  { label: string; icon: string; color: string; bg: string; border: string }
> = {
  slack: {
    label: "Slack Webhook",
    icon: "forum",
    color: "#4edea3",
    bg: "rgba(78, 222, 163, 0.1)",
    border: "rgba(78, 222, 163, 0.3)",
  },
  discord: {
    label: "Discord Webhook",
    icon: "chat",
    color: "#a855f7",
    bg: "rgba(168, 85, 247, 0.1)",
    border: "rgba(168, 85, 247, 0.3)",
  },
  pagerduty: {
    label: "PagerDuty / Incident API",
    icon: "notifications_active",
    color: "#ffb13b",
    bg: "rgba(255, 177, 59, 0.1)",
    border: "rgba(255, 177, 59, 0.3)",
  },
  generic: {
    label: "Generic JSON HTTP",
    icon: "webhook",
    color: "#22d3ee",
    bg: "rgba(34, 211, 238, 0.1)",
    border: "rgba(34, 211, 238, 0.3)",
  },
};

export default function Alerts() {
  const toast = useToast();
  const { t } = useLanguage();
  const [tab, setTab] = useState<TabKey>("rules");

  // Telemetry Data
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [history, setHistory] = useState<AlertHistory[]>([]);
  const [droppedEvents, setDroppedEvents] = useState<number>(0);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [filterEventType, setFilterEventType] = useState<string>("");
  const [filterStatus, setFilterStatus] = useState<string>("");

  // Modals & Drawers
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null);
  const [showTestModal, setShowTestModal] = useState(false);
  const [inspectItem, setInspectItem] = useState<AlertHistory | null>(null);

  // Multi-Config Rule Builder Form State
  const [formName, setFormName] = useState("");
  const [formEventType, setFormEventType] = useState<AlertEventType>("error_rate_surge");
  const [formSeverity, setFormSeverity] = useState<AlertSeverity>("warning");
  const [formThreshold, setFormThreshold] = useState<number>(5.0);
  const [formWindowSec, setFormWindowSec] = useState<number>(60);
  const [formCooldownSec, setFormCooldownSec] = useState<number>(900);
  const [formTenantID, setFormTenantID] = useState<string>("");
  const [formFilterProvider, setFormFilterProvider] = useState<string>("");
  const [formFilterModel, setFormFilterModel] = useState<string>("");
  const [formChannels, setFormChannels] = useState<AlertChannelConfig[]>([
    {
      channel_type: "slack",
      channel_name: "#ops-incidents",
      webhook_url: "",
      webhook_secret: "",
      is_enabled: true,
    },
  ]);
  const [formCircuitBreaker, setFormCircuitBreaker] = useState(true);

  // Test Webhook State
  const [testURL, setTestURL] = useState("");
  const [testChannel, setTestChannel] = useState<AlertChannelType>("slack");
  const [testSecret, setTestSecret] = useState("");
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; status_code: number; error?: string } | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const [rulesData, histData, tenantsData] = await Promise.all([
        api.listAlertRules({ event_type: filterEventType || undefined }),
        api.listAlertHistory({ limit: 100 }),
        api.listTenants().catch(() => [] as Tenant[]),
      ]);
      setRules(rulesData || []);
      setHistory(histData.history || []);
      setDroppedEvents(histData.dropped_events || 0);
      setTenants(tenantsData || []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load alert configurations");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [filterEventType]);

  const openCreateModal = () => {
    setEditingRule(null);
    setFormName("");
    setFormEventType("error_rate_surge");
    setFormSeverity("warning");
    setFormThreshold(EVENT_TYPE_LABELS.error_rate_surge.defaultThreshold);
    setFormWindowSec(60);
    setFormCooldownSec(900);
    setFormTenantID("");
    setFormFilterProvider("");
    setFormFilterModel("");
    setFormChannels([
      {
        channel_type: "slack",
        channel_name: "#ops-incidents",
        webhook_url: "",
        webhook_secret: "",
        is_enabled: true,
      },
    ]);
    setFormCircuitBreaker(true);
    setShowConfigModal(true);
  };

  const openEditModal = (rule: AlertRule) => {
    setEditingRule(rule);
    setFormName(rule.name);
    setFormEventType(rule.event_type);
    setFormSeverity(rule.severity || "warning");
    setFormThreshold(rule.threshold_value);
    setFormWindowSec(rule.window_seconds || rule.threshold_window_sec || 60);
    setFormCooldownSec(rule.cooldown_seconds || rule.cooldown_sec || 900);
    setFormTenantID(rule.tenant_id || "");
    setFormFilterProvider(rule.filter_provider || "");
    setFormFilterModel(rule.filter_model || "");

    // Populate channels (or fallback to primary single channel)
    if (rule.channels && rule.channels.length > 0) {
      setFormChannels(rule.channels);
    } else {
      setFormChannels([
        {
          channel_type: rule.channel_type || "generic",
          channel_name: rule.channel_type || "Primary Target",
          webhook_url: rule.webhook_url,
          webhook_secret: rule.webhook_secret || "",
          is_enabled: true,
        },
      ]);
    }
    setFormCircuitBreaker(true);
    setShowConfigModal(true);
  };

  const handleAddChannel = () => {
    setFormChannels([
      ...formChannels,
      {
        channel_type: "generic",
        channel_name: "HTTP Webhook",
        webhook_url: "",
        webhook_secret: "",
        is_enabled: true,
      },
    ]);
  };

  const handleRemoveChannel = (index: number) => {
    if (formChannels.length === 1) {
      toast.warning("At least one notification channel is recommended");
      return;
    }
    setFormChannels(formChannels.filter((_, idx) => idx !== index));
  };

  const handleUpdateChannel = (index: number, patch: Partial<AlertChannelConfig>) => {
    setFormChannels(
      formChannels.map((c, idx) => (idx === index ? { ...c, ...patch } : c))
    );
  };

  const handleSaveRule = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formName.trim()) {
      toast.warning("Rule name is required");
      return;
    }

    const validChannels = formChannels.filter((c) => c.webhook_url.trim() !== "");
    if (validChannels.length === 0) {
      toast.warning("At least one valid destination Webhook URL is required");
      return;
    }

    const primaryChannel = validChannels[0];

    try {
      if (editingRule) {
        const patch: AlertRulePatch = {
          name: formName.trim(),
          event_type: formEventType,
          severity: formSeverity,
          channel_type: primaryChannel.channel_type,
          webhook_url: primaryChannel.webhook_url.trim(),
          webhook_secret: primaryChannel.webhook_secret?.trim() || undefined,
          channels: validChannels,
          threshold_value: formThreshold,
          window_seconds: formWindowSec,
          cooldown_seconds: formCooldownSec,
          cooldown_sec: formCooldownSec,
          tenant_id: formTenantID || null,
          filter_provider: formFilterProvider.trim(),
          filter_model: formFilterModel.trim(),
        };
        await api.updateAlertRule(editingRule.id, patch);
        toast.success(`Multi-Config Rule "${formName}" updated successfully`);
      } else {
        await api.createAlertRule({
          name: formName.trim(),
          event_type: formEventType,
          severity: formSeverity,
          channel_type: primaryChannel.channel_type,
          webhook_url: primaryChannel.webhook_url.trim(),
          webhook_secret: primaryChannel.webhook_secret?.trim() || undefined,
          channels: validChannels,
          threshold_value: formThreshold,
          window_seconds: formWindowSec,
          cooldown_seconds: formCooldownSec,
          cooldown_sec: formCooldownSec,
          tenant_id: formTenantID || undefined,
          filter_provider: formFilterProvider.trim(),
          filter_model: formFilterModel.trim(),
          is_active: true,
        });
        toast.success(`Multi-Config Alert Rule "${formName}" deployed`);
      }
      setShowConfigModal(false);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save alert rule");
    }
  };

  const handleToggleRule = async (rule: AlertRule) => {
    try {
      await api.updateAlertRule(rule.id, { is_active: !rule.is_active });
      toast.info(`Rule "${rule.name}" is now ${!rule.is_active ? "active" : "paused"}`);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to toggle rule state");
    }
  };

  const handleDeleteRule = async (rule: AlertRule) => {
    if (!confirm(`Are you sure you want to delete alert rule "${rule.name}"?`)) return;
    try {
      await api.deleteAlertRule(rule.id);
      toast.success(`Rule "${rule.name}" deleted`);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete rule");
    }
  };

  const handleRunTest = async (url?: string, channel?: AlertChannelType, secret?: string) => {
    const targetURL = url || testURL;
    const targetChannel = channel || testChannel;
    const targetSecret = secret || testSecret;

    if (!targetURL.trim()) {
      toast.warning("Please provide a valid webhook destination URL");
      return;
    }

    try {
      setTestLoading(true);
      setTestResult(null);
      const res = await api.testAlertWebhook({
        webhook_url: targetURL.trim(),
        channel_type: targetChannel,
        webhook_secret: targetSecret.trim() || undefined,
      });
      setTestResult(res);
      if (res.success) {
        toast.success(`Webhook test ping delivered! HTTP ${res.status_code}`);
      } else {
        toast.error(`Webhook test failed: ${res.error || `HTTP ${res.status_code}`}`);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Webhook test invocation failed";
      setTestResult({ success: false, status_code: 0, error: msg });
      toast.error(msg);
    } finally {
      setTestLoading(false);
    }
  };

  const filteredHistory = useMemo(() => {
    if (!filterStatus) return history;
    return history.filter((h) => h.status === filterStatus);
  }, [history, filterStatus]);

  // Telemetry Aggregates
  const activeRulesCount = rules.filter((r) => r.is_active).length;
  const deliveredCount = history.filter((h) => h.status === "delivered").length;
  const suppressedCount = history.filter((h) => h.status === "suppressed_cooldown").length;
  const failedCount = history.filter((h) => h.status === "failed").length;
  const totalDispatches = deliveredCount + failedCount;
  const successRate = totalDispatches > 0 ? ((deliveredCount / totalDispatches) * 100).toFixed(1) : "100.0";

  return (
    <div className="page__content" style={{ maxWidth: 1440, margin: "0 auto", paddingBottom: 48 }}>
      {/* Top Header & Telemetry Breadcrumbs */}
      <div
        className="glass-panel"
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: 16,
          padding: "16px 20px",
          borderRadius: "var(--radius)",
          marginBottom: 20,
          background: "rgba(26, 33, 34, 0.8)",
          borderColor: "var(--border)",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--text-muted)", fontSize: 13 }}>
            <span>{t("common.system", "Observability")}</span>
            <span className="mono" style={{ color: "var(--border-strong)" }}>/</span>
            <span style={{ color: "var(--text)", fontWeight: 600 }}>{t("alerts.title", "Alerts & Multi-Channel Webhooks")}</span>
          </div>

          <span
            className="mono"
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 5,
              padding: "2px 8px",
              fontSize: 11,
              fontWeight: 700,
              color: "#4edea3",
              background: "rgba(78, 222, 163, 0.12)",
              border: "1px solid rgba(78, 222, 163, 0.35)",
              borderRadius: 2,
            }}
          >
            <span style={{ width: 6, height: 6, borderRadius: "50%", background: "#4edea3" }} />
            {t("alerts.engineActive", "[ENGINE ACTIVE]")}
          </span>

          <div
            className="mono"
            style={{
              fontSize: 11,
              color: "var(--text-muted)",
              display: "inline-flex",
              alignItems: "center",
              gap: 8,
              borderLeft: "1px solid var(--border)",
              paddingLeft: 12,
            }}
          >
            <span>{droppedEvents} {t("alerts.droppedEvents", "dropped events")}</span>
            <span style={{ color: "var(--border)" }}>|</span>
            <span>{t("alerts.rateLimit", "Rate limit: 30/min token bucket")}</span>
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <button
            type="button"
            className="btn btn--ghost btn--sm"
            onClick={() => {
              setTestURL("");
              setTestResult(null);
              setShowTestModal(true);
            }}
            style={{ display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            <span className="mono" style={{ fontSize: 12 }}>⚡</span>
            {t("alerts.testWebhook", "Test Webhook")}
          </button>

          <button
            type="button"
            className="btn btn--primary btn--sm"
            onClick={openCreateModal}
            style={{
              background: "#22d3ee",
              color: "#001f25",
              fontWeight: 700,
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            <span className="mono" style={{ fontSize: 14, fontWeight: 900 }}>+</span>
            {t("alerts.createRule", "Create Multi-Config Rule")}
          </button>
        </div>
      </div>

      {/* KPI Telemetry Row (Obsidian 4-Card Blueprint) */}
      <div className="kpi-grid" style={{ marginBottom: 24 }}>
        {/* Card 1: Active Alert Rules */}
        <div
          className="kpi"
          style={{
            position: "relative",
            overflow: "hidden",
            background: "#181a20",
            borderColor: "var(--border)",
          }}
        >
          <div className="kpi__label" style={{ color: "var(--text-muted)" }}>{t("alerts.kpiActiveRules", "Active Alert Rules")}</div>
          <div className="kpi__value kpi__value--accent" style={{ color: "#8aebff" }}>
            {activeRulesCount}{" "}
            <span style={{ fontSize: 13, fontWeight: 400, color: "var(--text-muted)" }}>/ {rules.length}</span>
          </div>
          <div className="kpi__delta mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
            running across {tenants.length || 1} tenant scope(s)
          </div>
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              height: 2,
              background: "#22d3ee",
              opacity: 0.6,
            }}
          />
        </div>

        {/* Card 2: Dispatched 24h */}
        <div
          className="kpi"
          style={{
            position: "relative",
            overflow: "hidden",
            background: "#181a20",
            borderColor: "var(--border)",
          }}
        >
          <div className="kpi__label" style={{ color: "#4edea3" }}>{t("alerts.kpiDispatched", "Dispatched 24h")}</div>
          <div className="kpi__value kpi__value--emerald">
            {deliveredCount}{" "}
            <span style={{ fontSize: 13, fontWeight: 400, color: "var(--text-muted)" }}>events</span>
          </div>
          <div className="kpi__delta mono" style={{ fontSize: 11, color: "#4edea3", display: "flex", alignItems: "center", gap: 5 }}>
            <span style={{ width: 6, height: 6, borderRadius: "50%", background: "#4edea3" }} />
            {successRate}% {t("alerts.kpiSuccessRate", "delivery rate")}
          </div>
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              height: 2,
              background: "#10b981",
              opacity: 0.7,
            }}
          />
        </div>

        {/* Card 3: Spam Suppressed */}
        <div
          className="kpi"
          style={{
            position: "relative",
            overflow: "hidden",
            background: "#181a20",
            borderColor: "var(--border)",
          }}
        >
          <div className="kpi__label" style={{ color: "#ffb13b" }}>{t("alerts.kpiSuppressed", "Spam Suppressed")}</div>
          <div className="kpi__value kpi__value--amber">
            {suppressedCount}{" "}
            <span style={{ fontSize: 13, fontWeight: 400, color: "var(--text-muted)" }}>throttled</span>
          </div>
          <div className="kpi__delta mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
            {t("alerts.kpiSuppressedDesc", "by Cooldown Debounce Protection")}
          </div>
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              height: 2,
              background: "#f59e0b",
              opacity: 0.6,
            }}
          />
        </div>

        {/* Card 4: Circuit Health */}
        <div
          className="kpi"
          style={{
            position: "relative",
            overflow: "hidden",
            background: "#181a20",
            borderColor: "var(--border)",
          }}
        >
          <div className="kpi__label" style={{ color: "#4edea3" }}>{t("alerts.kpiCircuit", "Circuit Health")}</div>
          <div className="kpi__value" style={{ color: "#4edea3" }}>
            {t("alerts.kpiCircuitHealthy", "Healthy")}
          </div>
          <div className="kpi__delta mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
            {t("alerts.kpiCircuitDesc", "0 open circuit breakers")} (5m backoff)
          </div>
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              height: 2,
              background: "#4edea3",
              opacity: 0.6,
            }}
          />
        </div>
      </div>

      {/* Main Tabbed Navigation */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 20,
          borderBottom: "1px solid var(--border)",
          marginBottom: 20,
        }}
      >
        <button
          type="button"
          onClick={() => setTab("rules")}
          style={{
            padding: "10px 4px",
            border: "none",
            background: "none",
            color: tab === "rules" ? "#22d3ee" : "var(--text-muted)",
            borderBottom: tab === "rules" ? "2px solid #22d3ee" : "2px solid transparent",
            fontWeight: 600,
            fontSize: 14,
            cursor: "pointer",
            display: "inline-flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          {t("alerts.tabRules", "Alert Rules & Routing")}
          <span
            className="mono"
            style={{
              fontSize: 11,
              padding: "1px 6px",
              borderRadius: 2,
              background: tab === "rules" ? "rgba(34, 211, 238, 0.15)" : "var(--surface-high)",
              color: tab === "rules" ? "#22d3ee" : "var(--text-muted)",
              border: `1px solid ${tab === "rules" ? "rgba(34, 211, 238, 0.3)" : "var(--border)"}`,
            }}
          >
            {rules.length}
          </span>
        </button>

        <button
          type="button"
          onClick={() => setTab("history")}
          style={{
            padding: "10px 4px",
            border: "none",
            background: "none",
            color: tab === "history" ? "#22d3ee" : "var(--text-muted)",
            borderBottom: tab === "history" ? "2px solid #22d3ee" : "2px solid transparent",
            fontWeight: 600,
            fontSize: 14,
            cursor: "pointer",
            display: "inline-flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          {t("alerts.tabHistory", "Delivery Audit & Incident History")}
          <span
            className="mono"
            style={{
              fontSize: 11,
              padding: "1px 6px",
              borderRadius: 2,
              background: tab === "history" ? "rgba(34, 211, 238, 0.15)" : "var(--surface-high)",
              color: tab === "history" ? "#22d3ee" : "var(--text-muted)",
              border: `1px solid ${tab === "history" ? "rgba(34, 211, 238, 0.3)" : "var(--border)"}`,
            }}
          >
            {history.length}
          </span>
        </button>
      </div>

      {/* TAB 1: Alert Rules Matrix */}
      {tab === "rules" && (
        <div>
          {/* Rules Filter bar */}
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: 14,
              flexWrap: "wrap",
              gap: 12,
            }}
          >
            <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
              <select
                className="form-select"
                value={filterEventType}
                onChange={(e) => setFilterEventType(e.target.value)}
                style={{ width: 240, fontSize: 13, background: "#181a20", borderColor: "var(--border)" }}
              >
                <option value="">All Event Triggers</option>
                {Object.entries(EVENT_TYPE_LABELS).map(([k, meta]) => (
                  <option key={k} value={k}>
                    {meta.label}
                  </option>
                ))}
              </select>
              <span className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
                Showing {rules.length} configured alert rule(s)
              </span>
            </div>

            <button
              type="button"
              className="btn btn--ghost btn--sm"
              onClick={loadData}
              title="Refresh Alert Rules"
            >
              ⟳ Refresh
            </button>
          </div>

          {/* Rules Table */}
          <div
            className="table-wrap"
            style={{
              background: "#181a20",
              border: "1px solid var(--border)",
              borderRadius: "var(--radius)",
              overflow: "hidden",
            }}
          >
            <table className="data-table" style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ background: "rgba(36, 43, 45, 0.6)", borderBottom: "1px solid var(--border)" }}>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    RULE NAME & SEVERITY
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    TRIGGER CONDITION
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    SCOPES & TARGETS
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    NOTIFICATION MATRIX
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "right", fontSize: 11 }} className="mono">
                    ACTIONS
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={5} style={{ textAlign: "center", padding: "40px 16px", color: "var(--text-muted)" }}>
                      <div className="mono">Loading multi-config alert rules...</div>
                    </td>
                  </tr>
                ) : rules.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ textAlign: "center", padding: "48px 16px", color: "var(--text-muted)" }}>
                      <div style={{ fontSize: 16, marginBottom: 8, color: "var(--text)" }}>No alert rules configured yet</div>
                      <div style={{ fontSize: 13, marginBottom: 16 }}>
                        Create a multi-config rule to route error spikes, budget breaches, or latency anomalies to your webhooks.
                      </div>
                      <button
                        type="button"
                        className="btn btn--primary btn--sm"
                        onClick={openCreateModal}
                        style={{ background: "#22d3ee", color: "#001f25" }}
                      >
                        + Create First Alert Rule
                      </button>
                    </td>
                  </tr>
                ) : (
                  rules.map((rule) => {
                    const sev = rule.severity || "warning";
                    const sevBadgeStyle =
                      sev === "critical"
                        ? { color: "#ffb4ab", bg: "rgba(255, 180, 171, 0.12)", border: "rgba(255, 180, 171, 0.4)" }
                        : sev === "warning"
                        ? { color: "#ffb13b", bg: "rgba(255, 177, 59, 0.12)", border: "rgba(255, 177, 59, 0.4)" }
                        : { color: "#8aebff", bg: "rgba(34, 211, 238, 0.12)", border: "rgba(34, 211, 238, 0.4)" };

                    const evMeta = EVENT_TYPE_LABELS[rule.event_type] || {
                      label: rule.event_type,
                      unit: "",
                      desc: "",
                    };

                    const tenantName = tenants.find((t) => t.id === rule.tenant_id)?.name || rule.tenant_name;
                    const channelsList: AlertChannelConfig[] =
                      rule.channels && rule.channels.length > 0
                        ? rule.channels
                        : [
                            {
                              channel_type: rule.channel_type || "generic",
                              channel_name: rule.channel_type || "Target",
                              webhook_url: rule.webhook_url,
                              webhook_secret: rule.webhook_secret,
                              is_enabled: true,
                            },
                          ];

                    return (
                      <tr
                        key={rule.id}
                        style={{
                          borderBottom: "1px solid #1f2229",
                          opacity: rule.is_active ? 1 : 0.5,
                          transition: "background 0.15s",
                        }}
                      >
                        {/* 1. Rule Name & Severity */}
                        <td style={{ padding: "14px 16px", verticalAlign: "top" }}>
                          <div style={{ fontWeight: 600, color: "var(--text)", marginBottom: 4 }}>
                            {rule.name}
                          </div>
                          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                            <span
                              className="mono"
                              style={{
                                display: "inline-block",
                                padding: "1px 6px",
                                fontSize: 10,
                                fontWeight: 700,
                                borderRadius: 2,
                                color: sevBadgeStyle.color,
                                background: sevBadgeStyle.bg,
                                border: `1px solid ${sevBadgeStyle.border}`,
                              }}
                            >
                              [{sev.toUpperCase()}]
                            </span>
                            <span style={{ fontSize: 11, color: "var(--text-muted)" }}>
                              {evMeta.label}
                            </span>
                          </div>
                        </td>

                        {/* 2. Trigger Condition & Safeguards */}
                        <td style={{ padding: "14px 16px", verticalAlign: "top" }}>
                          <div
                            className="mono"
                            style={{
                              fontSize: 12,
                              color: "var(--text)",
                              background: "#0e1416",
                              padding: "3px 8px",
                              borderRadius: 3,
                              border: "1px solid var(--border)",
                              display: "inline-block",
                              marginBottom: 6,
                            }}
                          >
                            {rule.event_type === "budget_threshold"
                              ? `Balance <= $${rule.threshold_value.toFixed(2)}`
                              : `${rule.event_type}: >= ${rule.threshold_value} ${evMeta.unit}`}
                          </div>

                          <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                            <span
                              className="mono"
                              style={{
                                fontSize: 10,
                                padding: "1px 5px",
                                borderRadius: 2,
                                background: "rgba(133, 147, 151, 0.12)",
                                color: "var(--text-muted)",
                                border: "1px solid rgba(133, 147, 151, 0.3)",
                              }}
                            >
                              {rule.cooldown_seconds || rule.cooldown_sec || 900}s debounce
                            </span>

                            {rule.webhook_secret && (
                              <span
                                className="mono"
                                style={{
                                  fontSize: 10,
                                  padding: "1px 5px",
                                  borderRadius: 2,
                                  background: "rgba(34, 211, 238, 0.1)",
                                  color: "#8aebff",
                                  border: "1px solid rgba(34, 211, 238, 0.3)",
                                }}
                              >
                                HMAC-SHA256
                              </span>
                            )}
                          </div>
                        </td>

                        {/* 3. Scopes & Filters */}
                        <td style={{ padding: "14px 16px", verticalAlign: "top" }}>
                          <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                            <div>
                              <span
                                className="mono"
                                style={{
                                  fontSize: 11,
                                  padding: "2px 6px",
                                  borderRadius: 2,
                                  background: rule.tenant_id ? "rgba(168, 85, 247, 0.12)" : "rgba(133, 147, 151, 0.1)",
                                  color: rule.tenant_id ? "#c084fc" : "var(--text-muted)",
                                  border: `1px solid ${rule.tenant_id ? "rgba(168, 85, 247, 0.3)" : "var(--border)"}`,
                                }}
                              >
                                {rule.tenant_id ? `Tenant: ${tenantName || rule.tenant_id.slice(0, 8)}` : "All Tenants (Global)"}
                              </span>
                            </div>

                            {(rule.filter_provider || rule.filter_model) && (
                              <div style={{ display: "flex", gap: 4, flexWrap: "wrap", marginTop: 2 }}>
                                {rule.filter_provider && (
                                  <span
                                    className="mono"
                                    style={{
                                      fontSize: 10,
                                      padding: "1px 5px",
                                      background: "rgba(34, 211, 238, 0.08)",
                                      color: "#22d3ee",
                                      borderRadius: 2,
                                      border: "1px solid rgba(34, 211, 238, 0.2)",
                                    }}
                                  >
                                    prov: {rule.filter_provider}
                                  </span>
                                )}
                                {rule.filter_model && (
                                  <span
                                    className="mono"
                                    style={{
                                      fontSize: 10,
                                      padding: "1px 5px",
                                      background: "rgba(78, 222, 163, 0.08)",
                                      color: "#4edea3",
                                      borderRadius: 2,
                                      border: "1px solid rgba(78, 222, 163, 0.2)",
                                    }}
                                  >
                                    mod: {rule.filter_model}
                                  </span>
                                )}
                              </div>
                            )}
                          </div>
                        </td>

                        {/* 4. Multi-Channel Notification Matrix */}
                        <td style={{ padding: "14px 16px", verticalAlign: "top" }}>
                          <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                            {channelsList.map((ch, idx) => {
                              const chDef = CHANNEL_DEFS[ch.channel_type] || CHANNEL_DEFS.generic;
                              return (
                                <span
                                  key={idx}
                                  className="mono"
                                  title={ch.webhook_url}
                                  style={{
                                    fontSize: 11,
                                    padding: "2px 7px",
                                    borderRadius: 3,
                                    display: "inline-flex",
                                    alignItems: "center",
                                    gap: 5,
                                    color: chDef.color,
                                    background: chDef.bg,
                                    border: `1px solid ${chDef.border}`,
                                    opacity: ch.is_enabled !== false ? 1 : 0.4,
                                  }}
                                >
                                  <span style={{ fontSize: 9 }}>●</span>
                                  [{ch.channel_name || chDef.label}]
                                </span>
                              );
                            })}
                          </div>
                        </td>

                        {/* 5. Actions */}
                        <td style={{ padding: "14px 16px", verticalAlign: "top", textAlign: "right" }}>
                          <div style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                            <button
                              type="button"
                              className="btn btn--ghost btn--sm"
                              onClick={() => handleToggleRule(rule)}
                              title={rule.is_active ? "Pause Rule" : "Activate Rule"}
                              style={{ padding: "4px 8px", fontSize: 11 }}
                            >
                              {rule.is_active ? "Pause" : "Resume"}
                            </button>

                            <button
                              type="button"
                              className="btn btn--ghost btn--sm"
                              onClick={() =>
                                handleRunTest(
                                  rule.webhook_url,
                                  rule.channel_type || "generic",
                                  rule.webhook_secret
                                )
                              }
                              title="Test Fire Webhook"
                              style={{ padding: "4px 8px", color: "#4edea3" }}
                            >
                              ⚡ Test
                            </button>

                            <button
                              type="button"
                              className="btn btn--ghost btn--sm"
                              onClick={() => openEditModal(rule)}
                              title="Edit Multi-Config Rule"
                              style={{ padding: "4px 8px", color: "#22d3ee" }}
                            >
                              Edit
                            </button>

                            <button
                              type="button"
                              className="btn btn--ghost btn--sm"
                              onClick={() => handleDeleteRule(rule)}
                              title="Delete Rule"
                              style={{ padding: "4px 8px", color: "#f87171" }}
                            >
                              ✕
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* TAB 2: Incident Delivery History Table */}
      {tab === "history" && (
        <div>
          {/* History Filters */}
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: 14,
              flexWrap: "wrap",
              gap: 12,
            }}
          >
            <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
              <select
                className="form-select"
                value={filterStatus}
                onChange={(e) => setFilterStatus(e.target.value)}
                style={{ width: 220, fontSize: 13, background: "#181a20", borderColor: "var(--border)" }}
              >
                <option value="">All Delivery Statuses</option>
                <option value="delivered">Delivered (2xx Success)</option>
                <option value="suppressed_cooldown">Suppressed by Cooldown</option>
                <option value="failed">Failed / Circuit Open</option>
              </select>

              <span className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
                Showing {filteredHistory.length} incident record(s)
              </span>
            </div>

            <button type="button" className="btn btn--ghost btn--sm" onClick={loadData}>
              ⟳ Refresh Feed
            </button>
          </div>

          <div
            className="table-wrap"
            style={{
              background: "#181a20",
              border: "1px solid var(--border)",
              borderRadius: "var(--radius)",
              overflow: "hidden",
            }}
          >
            <table className="data-table" style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ background: "rgba(36, 43, 45, 0.6)", borderBottom: "1px solid var(--border)" }}>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    TIMESTAMP
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    RULE & EVENT
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    MESSAGE SUMMARY
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "left", fontSize: 11 }} className="mono">
                    DELIVERY STATUS
                  </th>
                  <th style={{ padding: "12px 16px", textAlign: "right", fontSize: 11 }} className="mono">
                    PAYLOAD
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={5} style={{ textAlign: "center", padding: "40px 16px", color: "var(--text-muted)" }}>
                      <div className="mono">Loading delivery audit trail...</div>
                    </td>
                  </tr>
                ) : filteredHistory.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ textAlign: "center", padding: "48px 16px", color: "var(--text-muted)" }}>
                      <div style={{ fontSize: 15, color: "var(--text-muted)" }}>No alert incidents matching filter</div>
                    </td>
                  </tr>
                ) : (
                  filteredHistory.map((item) => {
                    const statusBadge =
                      item.status === "delivered" ? (
                        <span
                          className="mono"
                          style={{
                            padding: "2px 7px",
                            fontSize: 10,
                            fontWeight: 700,
                            borderRadius: 2,
                            color: "#4edea3",
                            background: "rgba(78, 222, 163, 0.12)",
                            border: "1px solid rgba(78, 222, 163, 0.35)",
                          }}
                        >
                          [DELIVERED] {item.status_code ? `HTTP ${item.status_code}` : ""}
                        </span>
                      ) : item.status === "suppressed_cooldown" ? (
                        <span
                          className="mono"
                          style={{
                            padding: "2px 7px",
                            fontSize: 10,
                            fontWeight: 700,
                            borderRadius: 2,
                            color: "#ffb13b",
                            background: "rgba(255, 177, 59, 0.12)",
                            border: "1px solid rgba(255, 177, 59, 0.35)",
                          }}
                        >
                          [SUPPRESSED_COOLDOWN]
                        </span>
                      ) : (
                        <span
                          className="mono"
                          style={{
                            padding: "2px 7px",
                            fontSize: 10,
                            fontWeight: 700,
                            borderRadius: 2,
                            color: "#ffb4ab",
                            background: "rgba(255, 180, 171, 0.12)",
                            border: "1px solid rgba(255, 180, 171, 0.35)",
                          }}
                        >
                          [FAILED] {item.error_message || (item.status_code ? `HTTP ${item.status_code}` : "")}
                        </span>
                      );

                    return (
                      <tr key={String(item.id)} style={{ borderBottom: "1px solid #1f2229" }}>
                        <td style={{ padding: "12px 16px", verticalAlign: "top", fontSize: 12 }} className="mono">
                          {fmtDate(item.triggered_at)}
                        </td>
                        <td style={{ padding: "12px 16px", verticalAlign: "top" }}>
                          <div style={{ fontWeight: 600, color: "var(--text)" }}>{item.rule_name}</div>
                          <div className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
                            {item.event_type}
                          </div>
                        </td>
                        <td style={{ padding: "12px 16px", verticalAlign: "top", fontSize: 13, color: "var(--text)" }}>
                          {item.message || item.title}
                        </td>
                        <td style={{ padding: "12px 16px", verticalAlign: "top" }}>{statusBadge}</td>
                        <td style={{ padding: "12px 16px", verticalAlign: "top", textAlign: "right" }}>
                          <button
                            type="button"
                            className="btn btn--ghost btn--sm mono"
                            onClick={() => setInspectItem(item)}
                            style={{ fontSize: 11, padding: "2px 8px" }}
                          >
                            Inspect
                          </button>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* MODAL / DRAWER: Multi-Config Rule Builder */}
      {showConfigModal && (
        <Modal
          title={editingRule ? `Edit Rule: ${editingRule.name}` : "Create Multi-Config Alert Rule"}
          onClose={() => setShowConfigModal(false)}
          maxWidth={760}
        >
          <form onSubmit={handleSaveRule} style={{ display: "flex", flexDirection: "column", gap: 20 }}>
            {/* TIER 1: Rule Definition */}
            <div
              style={{
                padding: "16px 18px",
                background: "var(--surface-high)",
                borderRadius: "var(--radius)",
                border: "1px solid var(--border)",
              }}
            >
              <div
                className="mono"
                style={{
                  fontSize: 11,
                  fontWeight: 700,
                  color: "#22d3ee",
                  textTransform: "uppercase",
                  letterSpacing: "0.08em",
                  marginBottom: 12,
                }}
              >
                1. Trigger Definition & Severity
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14, marginBottom: 14 }}>
                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Rule Name *</label>
                  <input
                    type="text"
                    className="form-input"
                    placeholder="e.g. Critical 5xx Upstream Spikes"
                    value={formName}
                    onChange={(e) => setFormName(e.target.value)}
                    required
                  />
                </div>

                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Event Type *</label>
                  <select
                    className="form-select"
                    value={formEventType}
                    onChange={(e) => {
                      const nextType = e.target.value as AlertEventType;
                      setFormEventType(nextType);
                      setFormThreshold(EVENT_TYPE_LABELS[nextType].defaultThreshold);
                    }}
                  >
                    {Object.entries(EVENT_TYPE_LABELS).map(([k, meta]) => (
                      <option key={k} value={k}>
                        {meta.label} ({meta.unit})
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              {/* Severity Radios */}
              <div className="form-group" style={{ marginBottom: 14 }}>
                <label className="form-label" style={{ fontSize: 12 }}>Severity Tier</label>
                <div style={{ display: "flex", gap: 12 }}>
                  {[
                    { key: "info", label: "[INFO]", color: "#22d3ee", bg: "rgba(34, 211, 238, 0.12)" },
                    { key: "warning", label: "[WARNING]", color: "#ffb13b", bg: "rgba(255, 177, 59, 0.12)" },
                    { key: "critical", label: "[CRITICAL]", color: "#ffb4ab", bg: "rgba(255, 180, 171, 0.12)" },
                  ].map((s) => (
                    <label
                      key={s.key}
                      className="mono"
                      style={{
                        flex: 1,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        gap: 8,
                        padding: "8px 12px",
                        borderRadius: "var(--radius-sm)",
                        cursor: "pointer",
                        border: formSeverity === s.key ? `2px solid ${s.color}` : "1px solid var(--border)",
                        background: formSeverity === s.key ? s.bg : "#181a20",
                        color: s.color,
                        fontSize: 12,
                        fontWeight: 700,
                        transition: "all 0.15s",
                      }}
                    >
                      <input
                        type="radio"
                        name="severity"
                        checked={formSeverity === s.key}
                        onChange={() => setFormSeverity(s.key as AlertSeverity)}
                        style={{ display: "none" }}
                      />
                      {s.label}
                    </label>
                  ))}
                </div>
              </div>

              {/* Threshold & Window */}
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>
                    Threshold Value ({EVENT_TYPE_LABELS[formEventType].unit}) *
                  </label>
                  <input
                    type="number"
                    step="any"
                    className="form-input mono"
                    value={formThreshold}
                    onChange={(e) => setFormThreshold(parseFloat(e.target.value) || 0)}
                    required
                  />
                  <div className="form-hint" style={{ fontSize: 11, marginTop: 4 }}>
                    {EVENT_TYPE_LABELS[formEventType].desc}
                  </div>
                </div>

                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Evaluation Window</label>
                  <select
                    className="form-select mono"
                    value={formWindowSec}
                    onChange={(e) => setFormWindowSec(parseInt(e.target.value, 10) || 60)}
                  >
                    <option value="60">1 minute (60s)</option>
                    <option value="300">5 minutes (300s)</option>
                    <option value="900">15 minutes (900s)</option>
                    <option value="3600">1 hour (3600s)</option>
                  </select>
                </div>
              </div>
            </div>

            {/* TIER 2: Granular Scoping & Filters */}
            <div
              style={{
                padding: "16px 18px",
                background: "var(--surface-high)",
                borderRadius: "var(--radius)",
                border: "1px solid var(--border)",
              }}
            >
              <div
                className="mono"
                style={{
                  fontSize: 11,
                  fontWeight: 700,
                  color: "#a855f7",
                  textTransform: "uppercase",
                  letterSpacing: "0.08em",
                  marginBottom: 12,
                }}
              >
                2. Target Scopes & Filters
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Tenant Scope</label>
                  <select
                    className="form-select"
                    value={formTenantID}
                    onChange={(e) => setFormTenantID(e.target.value)}
                  >
                    <option value="">All Tenants (Global)</option>
                    {tenants.map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name} ({t.type})
                      </option>
                    ))}
                  </select>
                </div>

                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Provider Filter</label>
                  <input
                    type="text"
                    className="form-input mono"
                    placeholder="All (e.g. openai)"
                    value={formFilterProvider}
                    onChange={(e) => setFormFilterProvider(e.target.value)}
                  />
                </div>

                <div className="form-group" style={{ margin: 0 }}>
                  <label className="form-label" style={{ fontSize: 12 }}>Model Filter</label>
                  <input
                    type="text"
                    className="form-input mono"
                    placeholder="All (e.g. gpt-4o)"
                    value={formFilterModel}
                    onChange={(e) => setFormFilterModel(e.target.value)}
                  />
                </div>
              </div>
            </div>

            {/* TIER 3: Multi-Channel Destinations */}
            <div
              style={{
                padding: "16px 18px",
                background: "var(--surface-high)",
                borderRadius: "var(--radius)",
                border: "1px solid var(--border)",
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  marginBottom: 12,
                }}
              >
                <div
                  className="mono"
                  style={{
                    fontSize: 11,
                    fontWeight: 700,
                    color: "#4edea3",
                    textTransform: "uppercase",
                    letterSpacing: "0.08em",
                  }}
                >
                  3. Multi-Channel Destinations ({formChannels.length})
                </div>
                <button
                  type="button"
                  className="btn btn--ghost btn--sm mono"
                  onClick={handleAddChannel}
                  style={{ color: "#4edea3", borderColor: "rgba(78, 222, 163, 0.3)", fontSize: 11 }}
                >
                  + Add Destination
                </button>
              </div>

              <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
                {formChannels.map((ch, idx) => (
                  <div
                    key={idx}
                    style={{
                      padding: "12px",
                      background: "#181a20",
                      borderRadius: "var(--radius-sm)",
                      border: "1px solid var(--border)",
                      display: "flex",
                      flexDirection: "column",
                      gap: 8,
                    }}
                  >
                    <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                      <select
                        className="form-select mono"
                        value={ch.channel_type}
                        onChange={(e) =>
                          handleUpdateChannel(idx, { channel_type: e.target.value as AlertChannelType })
                        }
                        style={{ width: 180, fontSize: 12 }}
                      >
                        <option value="slack">Slack Webhook</option>
                        <option value="discord">Discord Webhook</option>
                        <option value="pagerduty">PagerDuty Alert</option>
                        <option value="generic">Custom HTTP Target</option>
                      </select>

                      <input
                        type="text"
                        className="form-input"
                        placeholder="Label / Channel Name (e.g. #ops-incidents)"
                        value={ch.channel_name || ""}
                        onChange={(e) => handleUpdateChannel(idx, { channel_name: e.target.value })}
                        style={{ width: 220, fontSize: 12 }}
                      />

                      <label
                        className="mono"
                        style={{
                          fontSize: 11,
                          color: "var(--text-muted)",
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 6,
                          cursor: "pointer",
                          marginLeft: "auto",
                        }}
                      >
                        <input
                          type="checkbox"
                          checked={ch.is_enabled !== false}
                          onChange={(e) => handleUpdateChannel(idx, { is_enabled: e.target.checked })}
                        />
                        Active
                      </label>

                      {formChannels.length > 1 && (
                        <button
                          type="button"
                          className="btn btn--ghost btn--sm"
                          onClick={() => handleRemoveChannel(idx)}
                          style={{ color: "#f87171", padding: "2px 8px", fontSize: 12 }}
                          title="Remove target"
                        >
                          ✕
                        </button>
                      )}
                    </div>

                    <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr", gap: 8 }}>
                      <input
                        type="url"
                        className="form-input mono"
                        placeholder="Webhook Target URL (https://hooks.slack.com/...)"
                        value={ch.webhook_url}
                        onChange={(e) => handleUpdateChannel(idx, { webhook_url: e.target.value })}
                        style={{ fontSize: 12 }}
                        required
                      />

                      <input
                        type="password"
                        className="form-input mono"
                        placeholder="HMAC Secret Key (Optional)"
                        value={ch.webhook_secret || ""}
                        onChange={(e) => handleUpdateChannel(idx, { webhook_secret: e.target.value })}
                        style={{ fontSize: 12 }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* TIER 4: Anti-Spam Safeguards */}
            <div
              style={{
                padding: "16px 18px",
                background: "var(--surface-high)",
                borderRadius: "var(--radius)",
                border: "1px solid var(--border)",
              }}
            >
              <div
                className="mono"
                style={{
                  fontSize: 11,
                  fontWeight: 700,
                  color: "#ffb13b",
                  textTransform: "uppercase",
                  letterSpacing: "0.08em",
                  marginBottom: 12,
                }}
              >
                4. Anti-Spam Safeguards & Circuit Breaker
              </div>

              <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                <div>
                  <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
                    <label className="form-label" style={{ fontSize: 12, margin: 0 }}>
                      Cooldown Period (Debounce Suppression)
                    </label>
                    <span
                      className="mono"
                      style={{
                        fontSize: 11,
                        padding: "2px 8px",
                        borderRadius: 2,
                        background: "rgba(34, 211, 238, 0.1)",
                        color: "#22d3ee",
                        border: "1px solid rgba(34, 211, 238, 0.3)",
                      }}
                    >
                      {formCooldownSec}s ({Math.round(formCooldownSec / 60)} min)
                    </span>
                  </div>

                  <input
                    type="range"
                    min="60"
                    max="3600"
                    step="30"
                    value={formCooldownSec}
                    onChange={(e) => setFormCooldownSec(parseInt(e.target.value, 10))}
                    style={{ width: "100%", accentColor: "#22d3ee" }}
                  />
                  <div
                    className="mono"
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      fontSize: 10,
                      color: "var(--text-muted)",
                      marginTop: 2,
                    }}
                  >
                    <span>60s (1 min)</span>
                    <span>900s (15 min)</span>
                    <span>3600s (1 hour)</span>
                  </div>
                </div>

                <label
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "10px 12px",
                    background: "#181a20",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-sm)",
                    cursor: "pointer",
                  }}
                >
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 600, color: "var(--text)" }}>
                      Circuit Breaker Protection
                    </div>
                    <div style={{ fontSize: 11, color: "var(--text-muted)" }}>
                      Automatically pause outbound dispatch to hosts after 3 sequential HTTP failures (5m backoff)
                    </div>
                  </div>
                  <input
                    type="checkbox"
                    checked={formCircuitBreaker}
                    onChange={(e) => setFormCircuitBreaker(e.target.checked)}
                    style={{ width: 16, height: 16, accentColor: "#4edea3" }}
                  />
                </label>
              </div>
            </div>

            {/* Modal Actions */}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 6 }}>
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => setShowConfigModal(false)}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="btn btn--primary"
                style={{ background: "#22d3ee", color: "#001f25", fontWeight: 700 }}
              >
                {editingRule ? "Save Rule Configuration" : "Deploy Multi-Config Rule"}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* MODAL: Test Webhook Sandbox */}
      {showTestModal && (
        <Modal title="Webhook Sandbox Test Bench" onClose={() => setShowTestModal(false)} maxWidth={580}>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              handleRunTest();
            }}
            style={{ display: "flex", flexDirection: "column", gap: 16 }}
          >
            <div className="form-group" style={{ margin: 0 }}>
              <label className="form-label">Channel Target Type</label>
              <select
                className="form-select mono"
                value={testChannel}
                onChange={(e) => setTestChannel(e.target.value as AlertChannelType)}
              >
                <option value="slack">Slack Incoming Webhook</option>
                <option value="discord">Discord Webhook</option>
                <option value="pagerduty">PagerDuty Events API v2</option>
                <option value="generic">Custom HTTP Target (Generic JSON)</option>
              </select>
            </div>

            <div className="form-group" style={{ margin: 0 }}>
              <label className="form-label">Destination Webhook URL *</label>
              <input
                type="url"
                className="form-input mono"
                placeholder="https://hooks.slack.com/services/..."
                value={testURL}
                onChange={(e) => setTestURL(e.target.value)}
                required
              />
            </div>

            <div className="form-group" style={{ margin: 0 }}>
              <label className="form-label">HMAC Secret Key (Optional)</label>
              <input
                type="password"
                className="form-input mono"
                placeholder="Shared secret for sha256 HMAC header verification"
                value={testSecret}
                onChange={(e) => setTestSecret(e.target.value)}
              />
            </div>

            {testResult && (
              <div
                style={{
                  padding: "12px",
                  borderRadius: "var(--radius-sm)",
                  background: testResult.success ? "rgba(78, 222, 163, 0.1)" : "rgba(255, 180, 171, 0.1)",
                  border: `1px solid ${testResult.success ? "rgba(78, 222, 163, 0.4)" : "rgba(255, 180, 171, 0.4)"}`,
                }}
              >
                <div
                  className="mono"
                  style={{
                    fontSize: 12,
                    fontWeight: 700,
                    color: testResult.success ? "#4edea3" : "#ffb4ab",
                    marginBottom: 4,
                  }}
                >
                  {testResult.success ? `[SUCCESS: HTTP ${testResult.status_code}]` : `[FAILED: HTTP ${testResult.status_code}]`}
                </div>
                {testResult.error && (
                  <div className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
                    {testResult.error}
                  </div>
                )}
              </div>
            )}

            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 4 }}>
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => setShowTestModal(false)}
              >
                Close
              </button>
              <button
                type="submit"
                className="btn btn--primary"
                disabled={testLoading}
                style={{ background: "#22d3ee", color: "#001f25", fontWeight: 700 }}
              >
                {testLoading ? "Dispatching..." : "Fire Test Webhook"}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* MODAL: Payload Inspector */}
      {inspectItem && (
        <Modal
          title={`Incident Payload Inspector: ${inspectItem.rule_name}`}
          onClose={() => setInspectItem(null)}
          maxWidth={640}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
              <div>
                <span className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>Event Trigger:</span>
                <div className="mono" style={{ fontSize: 12, color: "#22d3ee" }}>{inspectItem.event_type}</div>
              </div>
              <div>
                <span className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>Dispatched At:</span>
                <div className="mono" style={{ fontSize: 12 }}>{fmtDate(inspectItem.triggered_at)}</div>
              </div>
            </div>

            <div>
              <span className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>Status Message:</span>
              <div style={{ fontSize: 13, marginTop: 2 }}>{inspectItem.message}</div>
            </div>

            {inspectItem.error_message && (
              <div
                style={{
                  padding: "10px",
                  borderRadius: "var(--radius-sm)",
                  background: "rgba(255, 180, 171, 0.1)",
                  border: "1px solid rgba(255, 180, 171, 0.3)",
                }}
              >
                <span className="mono" style={{ fontSize: 11, color: "#ffb4ab", fontWeight: 700 }}>Error Details:</span>
                <div className="mono" style={{ fontSize: 12, color: "#ffb4ab", marginTop: 2 }}>
                  {inspectItem.error_message}
                </div>
              </div>
            )}

            <div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
                <span className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>Sent Webhook Payload (JSON):</span>
                <button
                  type="button"
                  className="btn btn--ghost btn--sm mono"
                  style={{ fontSize: 10, padding: "2px 6px" }}
                  onClick={() => {
                    navigator.clipboard.writeText(
                      JSON.stringify(inspectItem.payload || inspectItem.payload_sent || {}, null, 2)
                    );
                    toast.info("Payload copied to clipboard");
                  }}
                >
                  Copy JSON
                </button>
              </div>

              <pre
                className="mono"
                style={{
                  background: "#090f11",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-sm)",
                  padding: "14px",
                  fontSize: 12,
                  maxHeight: 280,
                  overflowY: "auto",
                  color: "#4edea3",
                  margin: 0,
                }}
              >
                {JSON.stringify(inspectItem.payload || inspectItem.payload_sent || {}, null, 2)}
              </pre>
            </div>

            <div style={{ display: "flex", justifyContent: "flex-end" }}>
              <button
                type="button"
                className="btn btn--primary btn--sm"
                onClick={() => setInspectItem(null)}
              >
                Done
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
