import { ReactElement, ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api/client";
import type { APIKey, DashboardStats, UsageByModel, UsageByUser, User } from "../api/types";
import { fmtMoney, fmtNum, shortTs } from "../lib/format";
import LogDetailDrawer, { getStatusInfo } from "../components/LogDetailDrawer";

// Obsidian Telemetry palette — mirrors tokens in styles/theme.css so the
// dashboard reads as one system with the rest of the admin.
const C = {
  border: "#2d3139",
  muted: "#859397",
  faint: "#5c6a6e",
  cyan: "#22d3ee",
  cyanSoft: "#8aebff",
  emerald: "#10b981",
  emeraldSoft: "#4edea3",
  violet: "#a855f7",
  amber: "#f59e0b",
  amberSoft: "#ffb957",
  red: "#f87171",
};

const DONUT_COLORS = ["#a855f7", "#8b5cf6", "#7c3aed", "#9333ea", "#6d28d9", "#5b21b6", "#a78bfa", "#c084fc"];

const tooltipStyle = {
  background: "#181a20",
  border: "1px solid #2d3139",
  borderRadius: 6,
  fontSize: 12,
  fontFamily: "JetBrains Mono, monospace",
  color: "#dde4e5",
};

const tooltipItemStyle = {
  color: "#dde4e5",
};

const tooltipLabelStyle = {
  color: "#859397",
};

// Compact formatter for token counts / axis labels: 1_250_000 -> "1.3M".
function fmtCompact(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return "—";
  const a = Math.abs(n);
  if (a >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (a >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(Math.round(n));
}

export default function Dashboard() {
  const [interval, setInterval] = useState("hour");
  const [userFilter, setUserFilter] = useState("");
  const [keyFilter, setKeyFilter] = useState("");
  const [providerFilter, setProviderFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [modelInput, setModelInput] = useState("");
  const [selectedLogId, setSelectedLogId] = useState<number | null>(null);

  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [providerOptions, setProviderOptions] = useState<string[]>(["openai", "anthropic", "gemini"]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Debounce model filter text input
  useEffect(() => {
    const timer = setTimeout(() => {
      setModelFilter(modelInput);
    }, 350);
    return () => clearTimeout(timer);
  }, [modelInput]);

  // Load API keys when user filter changes
  useEffect(() => {
    api.listKeys(userFilter || undefined).then(setKeys).catch(() => {});
    setKeyFilter("");
  }, [userFilter]);

  // Load available providers
  useEffect(() => {
    api
      .listProviders()
      .then((ps) => {
        const known = new Set(["openai", "anthropic", "gemini"]);
        const opts = ["openai", "anthropic", "gemini"];
        for (const p of ps) {
          if (!known.has(p.provider)) opts.push(p.provider);
        }
        setProviderOptions(opts);
      })
      .catch(() => {});
  }, []);

  const load = useCallback(async () => {
    setError(null);
    setLoading(true);
    try {
      const d = await api.dashboard({
        interval,
        user_id: userFilter || undefined,
        key_id: keyFilter || undefined,
        provider: providerFilter || undefined,
        model: modelFilter || undefined,
        limit: 20,
      });
      setStats(d);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load dashboard");
    } finally {
      setLoading(false);
    }
  }, [interval, userFilter, keyFilter, providerFilter, modelFilter]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    api.listUsers().then(setUsers).catch(() => {});
  }, []);

  const clearFilters = () => {
    setUserFilter("");
    setKeyFilter("");
    setProviderFilter("");
    setModelFilter("");
    setModelInput("");
  };

  const isFiltered = Boolean(userFilter || keyFilter || providerFilter || modelFilter || modelInput);

  const t = stats?.totals;
  const series = stats?.time_series ?? [];
  const successRate = t && t.requests > 0 ? 1 - t.error_rate : 0;

  const reqSpark = useMemo(() => series.map((p) => p.requests), [series]);
  const costSpark = useMemo(() => series.map((p) => p.cost_usd), [series]);

  return (
    <div>
      <div className="page__header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 16 }}>
        <div>
          <h1 className="page__title">Overview</h1>
          <p className="page__sub">Live gateway telemetry and workload intelligence across the last 24h</p>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <button
            className="btn btn--primary"
            style={{ height: 32, display: "flex", alignItems: "center", gap: 6 }}
            disabled={loading}
            onClick={load}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>refresh</span>
            {loading ? "Refreshing..." : "Refresh"}
          </button>
        </div>
      </div>

      {/* ── Filter Toolbar ─────────────────────────────────────────────── */}
      <div className="filter-bar" style={{ marginBottom: 16, flexWrap: "wrap", alignItems: "center" }}>
        <select
          className="select"
          style={{ width: 140 }}
          value={interval}
          onChange={(e) => setInterval(e.target.value)}
        >
          <option value="hour">Hourly buckets</option>
          <option value="day">Daily buckets</option>
        </select>

        <select
          className="select"
          style={{ width: 170 }}
          value={userFilter}
          onChange={(e) => setUserFilter(e.target.value)}
        >
          <option value="">All users</option>
          {users.map((u) => (
            <option key={u.id} value={u.id}>
              {u.email}
            </option>
          ))}
        </select>

        <select
          className="select"
          style={{ width: 170 }}
          value={keyFilter}
          onChange={(e) => setKeyFilter(e.target.value)}
        >
          <option value="">All API keys</option>
          {keys.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name} ({k.key_prefix}…)
            </option>
          ))}
        </select>

        <select
          className="select"
          style={{ width: 140 }}
          value={providerFilter}
          onChange={(e) => setProviderFilter(e.target.value)}
        >
          <option value="">All providers</option>
          {providerOptions.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>

        <input
          className="input"
          style={{ width: 170 }}
          placeholder="Filter by model…"
          value={modelInput}
          onChange={(e) => setModelInput(e.target.value)}
        />

        {isFiltered && (
          <button
            className="btn btn--ghost btn--sm"
            style={{
              textDecoration: "underline",
              border: "none",
              background: "transparent",
              color: "var(--text-muted)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
              cursor: "pointer",
              padding: "0 4px",
              minHeight: "auto",
              boxShadow: "none",
            }}
            onClick={clearFilters}
          >
            Clear filters
          </button>
        )}
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        {/* ── Row 1 · Hero KPIs ─────────────────────────────────────────── */}
        <div className="kpi-grid" style={{ marginBottom: 0 }}>
          <HeroKpi
            label="Total Requests"
            value={fmtNum(t?.requests)}
            accent="cyan"
            spark={reqSpark}
            sub={t && t.streaming_requests ? `${t.streaming_requests} streaming` : undefined}
          />
          <HeroKpi label="Total Cost" value={fmtMoney(t?.cost_usd)} accent="emerald" spark={costSpark} />
          <HeroKpi
            label="Cache Savings"
            value={fmtMoney(t?.cache_savings_usd ?? 0)}
            accent="emerald"
            sub={t && t.cache_hit_rate ? `${t.cache_hit_rate.toFixed(1)}% prompt cache hit` : "0% cache hit"}
          />
          <SuccessRateCard rate={successRate} />
          <HeroKpi
            label="Total Tokens"
            value={fmtCompact((t?.prompt_tokens ?? 0) + (t?.completion_tokens ?? 0))}
            accent="cyan"
            sub={t ? `Prompt ${fmtCompact(t.prompt_tokens)} · Out ${fmtCompact(t.completion_tokens)}` : undefined}
          />
          <HeroKpi
            label="Avg Latency"
            value={t ? `${t.avg_latency_ms.toFixed(0)} ms` : "—"}
            accent="violet"
            sub={t && t.avg_ttft_ms ? `AVG TTFT: ${t.avg_ttft_ms.toFixed(0)} ms` : "AVG TTFT: —"}
          />
        </div>

        {/* ── Row 2 · Tokens + cost trend ───────────────────────────────── */}
        <div className="dashboard-grid dashboard-grid--7-5">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Token Analytics <span className="mono">[prompt · cached · completion · reasoning]</span>
          </div>
          <ChartBox height={240}>
            <BarChart data={series}>
              <CartesianGrid stroke="#2d3139" strokeDasharray="3 3" vertical={false} />
              <XAxis
                dataKey="time_bucket"
                tick={{ fill: "#859397", fontSize: 11 }}
                tickFormatter={(v: string) => shortTs(v)}
                axisLine={{ stroke: "#2d3139" }}
                tickLine={false}
              />
              <YAxis
                tick={{ fill: "#859397", fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v: number) => fmtCompact(v)}
              />
              <Tooltip contentStyle={tooltipStyle} itemStyle={tooltipItemStyle} labelStyle={tooltipLabelStyle} labelFormatter={(v) => shortTs(String(v))} />
              <Bar dataKey="prompt_tokens" name="Prompt" stackId="tokens" fill={C.cyan} radius={[0, 0, 0, 0]} />
              <Bar dataKey="cached_tokens" name="Cached" stackId="tokens" fill={C.emeraldSoft} radius={[0, 0, 0, 0]} />
              <Bar dataKey="completion_tokens" name="Completion" stackId="tokens" fill={C.violet} radius={[0, 0, 0, 0]} />
              <Bar dataKey="reasoning_tokens" name="Reasoning" stackId="tokens" fill={C.amberSoft} radius={[3, 3, 0, 0]} />
            </BarChart>
          </ChartBox>
          <LegendChips
            items={[
              { c: C.cyan, label: "Prompt" },
              { c: C.emeraldSoft, label: "Cached (50% off)" },
              { c: C.violet, label: "Completion" },
              { c: C.amberSoft, label: "Reasoning" },
            ]}
          />
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Cost Trend <span className="mono">[usd / bucket]</span>
          </div>
          <ChartBox height={240}>
            <AreaChart data={series}>
              <defs>
                <linearGradient id="costGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={C.emerald} stopOpacity={0.35} />
                  <stop offset="100%" stopColor={C.emerald} stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="#2d3139" strokeDasharray="3 3" vertical={false} />
              <XAxis
                dataKey="time_bucket"
                tick={{ fill: "#859397", fontSize: 11 }}
                tickFormatter={(v: string) => shortTs(v)}
                axisLine={{ stroke: "#2d3139" }}
                tickLine={false}
              />
              <YAxis
                tick={{ fill: "#859397", fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v: number) => `$${v}`}
              />
              <Tooltip contentStyle={tooltipStyle} itemStyle={tooltipItemStyle} labelStyle={tooltipLabelStyle} labelFormatter={(v) => shortTs(String(v))} />
              <Area type="monotone" dataKey="cost_usd" name="Cost" stroke={C.emeraldSoft} strokeWidth={2} fill="url(#costGrad)" dot={false} />
            </AreaChart>
          </ChartBox>
        </div>
      </div>

      {/* ── Row 3 · Model / consumers / performance ───────────────────── */}
      <div className="dashboard-grid dashboard-grid--3-col">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Cost by Model <span className="mono">[by_model]</span>
          </div>
          <ModelDonut rows={stats?.by_model ?? []} />
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Top Consumers <span className="mono">[by_user]</span>
          </div>
          <ConsumerBars rows={stats?.by_user ?? []} />
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Performance &amp; Latency <span className="mono">[metrics]</span>
          </div>
          <div className="stat-tiles-grid stat-tiles-grid--2" style={{ marginBottom: 10 }}>
            <StatTile label="AVG TTFT" value={t && t.avg_ttft_ms ? `${t.avg_ttft_ms.toFixed(0)} ms` : "—"} color={C.violet} />
            <StatTile label="AVG Latency" value={t ? `${t.avg_latency_ms.toFixed(0)} ms` : "—"} color={C.violet} />
          </div>
          <div className="stat-tiles-grid stat-tiles-grid--3" style={{ marginBottom: 10 }}>
            <StatTile label="P50 Latency" value={t && t.p50_latency_ms ? `${t.p50_latency_ms.toFixed(0)} ms` : "—"} color={C.cyan} />
            <StatTile label="P95 Latency" value={t && t.p95_latency_ms ? `${t.p95_latency_ms.toFixed(0)} ms` : "—"} color={C.amber} />
            <StatTile label="P99 Latency" value={t && t.p99_latency_ms ? `${t.p99_latency_ms.toFixed(0)} ms` : "—"} color={C.red} />
          </div>
          <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, color: "var(--text-muted)", borderTop: "1px solid #2d3139", paddingTop: 8 }}>
            <span>Streaming: <strong style={{ color: C.cyan }}>{t?.streaming_requests ?? 0}</strong> reqs</span>
            <span>Cache Hit Rate: <strong style={{ color: C.emeraldSoft }}>{t?.cache_hit_rate ? `${t.cache_hit_rate.toFixed(1)}%` : "0%"}</strong></span>
          </div>
        </div>
      </div>

      {/* ── Row 4 · Reliability ───────────────────────────────────────── */}
      <div className="dashboard-grid dashboard-grid--4-8">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Error Breakdown <span className="mono">[by status]</span>
          </div>
          <ErrorBreakdown breakdown={stats?.status_breakdown} totalRequests={t?.requests ?? 0} />
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Recent Requests <span className="mono">[recent]</span>
          </div>
          <RecentTable rows={stats?.recent ?? []} onInspect={(id) => setSelectedLogId(id)} />
        </div>
      </div>

      {/* ── Row 5 · Optimization & deep intelligence ──────────────────── */}
      <div className="dashboard-grid dashboard-grid--3-col">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Prompt Caching Intelligence <span className="mono">[upstream]</span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
              <div style={{ background: "#181a20", padding: "10px 12px", borderRadius: 6, border: "1px solid #2d3139" }}>
                <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>Cached Tokens</div>
                <div style={{ fontSize: 18, fontWeight: 700, color: C.emeraldSoft, fontFamily: "var(--font-mono)", marginTop: 4 }}>{fmtCompact(t?.cached_tokens ?? 0)}</div>
              </div>
              <div style={{ background: "#181a20", padding: "10px 12px", borderRadius: 6, border: "1px solid #2d3139" }}>
                <div style={{ fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", fontFamily: "var(--font-mono)" }}>Est. Cost Savings</div>
                <div style={{ fontSize: 18, fontWeight: 700, color: C.emerald, fontFamily: "var(--font-mono)", marginTop: 4 }}>{fmtMoney(t?.cache_savings_usd ?? 0)}</div>
              </div>
            </div>
            <div style={{ fontSize: 12, color: "var(--text-muted)", lineHeight: 1.4 }}>
              Cached prompt tokens automatically receive a <strong style={{ color: "#dde4e5" }}>50% discount</strong> against standard prompt pricing.
            </div>
            <div style={{ background: "#20242c", borderRadius: 4, height: 6, overflow: "hidden" }}>
              <div style={{ background: C.emerald, width: `${Math.min(100, Math.max(0, t?.cache_hit_rate ?? 0))}%`, height: "100%", transition: "width 0.3s ease" }} />
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, fontFamily: "var(--font-mono)", color: "var(--text-muted)" }}>
              <span>Hit Ratio: {t?.cache_hit_rate?.toFixed(1) ?? "0.0"}%</span>
              <span>Discount: 50%</span>
            </div>
          </div>
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Model Performance Matrix <span className="mono">[top 5]</span>
          </div>
          <div style={{ overflowX: "auto", maxHeight: 180 }}>
            <table className="table table--dense" style={{ fontSize: 11 }}>
              <thead>
                <tr>
                  <th>Model</th>
                  <th className="num">Reqs</th>
                  <th className="num">TTFT</th>
                  <th className="num">Latency</th>
                  <th className="num">Cost</th>
                </tr>
              </thead>
              <tbody>
                {(stats?.by_model ?? []).slice(0, 5).map((m) => (
                  <tr key={`${m.provider}:${m.model}`}>
                    <td className="truncate" style={{ maxWidth: 120 }}>{m.model}</td>
                    <td className="num mono">{m.requests}</td>
                    <td className="num mono">{m.avg_ttft_ms ? `${m.avg_ttft_ms.toFixed(0)}ms` : "—"}</td>
                    <td className="num mono">{m.avg_latency_ms ? `${m.avg_latency_ms.toFixed(0)}ms` : "—"}</td>
                    <td className="num mono">{fmtMoney(m.cost_usd)}</td>
                  </tr>
                ))}
                {(!stats?.by_model || stats.by_model.length === 0) && (
                  <tr><td colSpan={5} style={{ textAlign: "center", color: "var(--text-muted)" }}>No model data</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Streaming &amp; Gateway Health <span className="mono">[telemetry]</span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, fontSize: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "6px 0", borderBottom: "1px solid #2d3139" }}>
              <span style={{ color: "var(--text-muted)" }}>Streaming Engine</span>
              <span className="badge" style={{ background: "rgba(34, 211, 238, 0.15)", color: C.cyan, border: `1px solid ${C.cyan}` }}>Atomic TCP Flush</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "6px 0", borderBottom: "1px solid #2d3139" }}>
              <span style={{ color: "var(--text-muted)" }}>Fast SSE Serializer</span>
              <span className="badge" style={{ background: "rgba(16, 185, 129, 0.15)", color: C.emerald, border: `1px solid ${C.emerald}` }}>Zero-Alloc Active</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "6px 0", borderBottom: "1px solid #2d3139" }}>
              <span style={{ color: "var(--text-muted)" }}>Deep Observability</span>
              <span className="badge" style={{ background: "rgba(168, 85, 247, 0.15)", color: C.violet, border: `1px solid ${C.violet}` }}>Prometheus + Grafana</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "6px 0" }}>
              <span style={{ color: "var(--text-muted)" }}>Reasoning Tokens</span>
              <span className="mono" style={{ color: C.amberSoft, fontWeight: 600 }}>{fmtCompact(t?.reasoning_tokens ?? 0)}</span>
            </div>
          </div>
        </div>
      </div>
      <LogDetailDrawer logId={selectedLogId} onClose={() => setSelectedLogId(null)} />
    </div>
  );
}

/* ────────────────────────── Row 1 helpers ────────────────────────── */

function HeroKpi({
  label,
  value,
  accent,
  sub,
  spark,
}: {
  label: string;
  value: string;
  accent: "cyan" | "emerald" | "violet";
  sub?: string;
  spark?: number[];
}) {
  const color = accent === "cyan" ? C.cyan : accent === "emerald" ? C.emeraldSoft : C.violet;
  return (
    <div className="kpi" style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <div className="kpi__label">{label}</div>
      <div className={`kpi__value kpi__value--${accent}`} style={{ fontSize: 28, lineHeight: 1.1 }}>
        {value}
      </div>
      {sub && <div className="kpi__delta">{sub}</div>}
      {spark && spark.length > 1 && <Sparkline values={spark} color={color} height={26} />}
    </div>
  );
}

function SuccessRateCard({ rate }: { rate: number }) {
  const pct = rate * 100;
  const clamped = Math.min(100, Math.max(0, pct));
  const above = pct >= 99;
  return (
    <div className="kpi" style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <div className="kpi__label">Success Rate</div>
      <div className="kpi__value kpi__value--emerald" style={{ fontSize: 28, lineHeight: 1.1 }}>
        {pct.toFixed(2)}%
      </div>
      <div
        style={{
          position: "relative",
          height: 6,
          background: "rgba(255,255,255,0.08)",
          borderRadius: 3,
          marginTop: 6,
        }}
      >
        <div
          style={{
            position: "absolute",
            left: 0,
            top: 0,
            bottom: 0,
            width: `${clamped}%`,
            background: above ? C.emerald : C.amber,
            borderRadius: 3,
          }}
        />
        {/* target marker at 99% */}
        <div style={{ position: "absolute", left: "99%", top: -2, bottom: -2, width: 1, background: C.cyan, opacity: 0.7 }} />
      </div>
      <div className="kpi__delta" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <span style={{ fontSize: 10, letterSpacing: "0.08em" }}>TARGET &gt;99%</span>
        {above ? (
          <span className="badge badge--success">AT TARGET</span>
        ) : (
          <span className="badge badge--warn">{(99 - pct).toFixed(2)}pp LOW</span>
        )}
      </div>
    </div>
  );
}

/* ────────────────────────── Row 2 helpers ────────────────────────── */

function LegendChips({ items }: { items: { c: string; label: string }[] }) {
  return (
    <div style={{ display: "flex", gap: 16, marginTop: 10 }}>
      {items.map((it) => (
        <span key={it.label} style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 11, color: C.muted }}>
          <span style={{ width: 8, height: 8, borderRadius: 2, background: it.c, display: "inline-block" }} />
          {it.label}
        </span>
      ))}
    </div>
  );
}

/* ────────────────────────── Row 3 helpers ────────────────────────── */

function ModelDonut({ rows }: { rows: UsageByModel[] }) {
  if (!rows.length) return <div className="empty">No model usage in the window</div>;

  const data = rows.slice(0, DONUT_COLORS.length).map((m, i) => ({
    name: m.provider ? `${m.provider} · ${m.model}` : m.model,
    value: m.cost_usd,
    color: DONUT_COLORS[i % DONUT_COLORS.length],
  }));
  const total = rows.reduce((s, m) => s + m.cost_usd, 0);
  const rest = rows.length - data.length;

  return (
    <div>
      <div style={{ position: "relative", height: 190 }}>
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius="62%"
              outerRadius="88%"
              paddingAngle={2}
              stroke="none"
              isAnimationActive={false}
            >
              {data.map((d, i) => (
                <Cell key={i} fill={d.color} />
              ))}
            </Pie>
             <Tooltip contentStyle={tooltipStyle} itemStyle={tooltipItemStyle} labelStyle={tooltipLabelStyle} formatter={(v) => fmtMoney(Number(v))} />
          </PieChart>
        </ResponsiveContainer>
        <div
          style={{
            position: "absolute",
            inset: 0,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            pointerEvents: "none",
          }}
        >
          <div style={{ fontSize: 11, color: C.muted, letterSpacing: "0.08em" }}>TOTAL</div>
          <div className="mono" style={{ fontSize: 20, fontWeight: 700, color: C.cyanSoft }}>
            {fmtMoney(total)}
          </div>
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6, marginTop: 6 }}>
        {data.map((d) => (
          <div key={d.name} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12 }}>
            <span style={{ width: 8, height: 8, borderRadius: 2, background: d.color, display: "inline-block", flexShrink: 0 }} />
            <span className="truncate" style={{ flex: 1, color: C.muted }}>
              {d.name}
            </span>
            <span className="mono">{fmtMoney(d.value)}</span>
          </div>
        ))}
        {rest > 0 && (
          <div className="muted" style={{ fontSize: 11 }}>
            +{rest} more model{rest > 1 ? "s" : ""} (see Usage Logs)
          </div>
        )}
      </div>
    </div>
  );
}

function ConsumerBars({ rows }: { rows: UsageByUser[] }) {
  if (!rows.length) return <div className="empty">No usage in the window</div>;
  const top = rows.slice(0, 5);
  const max = Math.max(...top.map((r) => r.cost_usd), 0.0001);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {top.map((r) => (
        <div key={r.user_id}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 8, marginBottom: 4 }}>
            <span className="truncate" style={{ fontSize: 12, color: C.muted, maxWidth: "70%" }}>
              {r.email || r.user_id.slice(0, 8)}
            </span>
            <span className="mono" style={{ fontSize: 12, color: C.emeraldSoft }}>
              {fmtMoney(r.cost_usd)}{" "}
              <span className="muted" style={{ fontSize: 10 }}>
                · {fmtNum(r.requests)} req
              </span>
            </span>
          </div>
          <div style={{ height: 6, background: "rgba(255,255,255,0.06)", borderRadius: 3 }}>
            <div
              style={{
                height: "100%",
                width: `${Math.max(2, (r.cost_usd / max) * 100)}%`,
                background: C.emerald,
                borderRadius: 3,
                opacity: 0.85,
              }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function StatTile({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div style={{ border: `1px solid ${C.border}`, borderRadius: 4, padding: "10px 12px", background: "rgba(255,255,255,0.02)" }}>
      <div className="kpi__label" style={{ marginBottom: 4 }}>
        {label}
      </div>
      <div className="mono" style={{ fontSize: 18, fontWeight: 700, color }}>
        {value}
      </div>
    </div>
  );
}


/* ────────────────────────── Row 4 helpers ────────────────────────── */

function ErrorBreakdown({
  breakdown,
  totalRequests,
}: {
  breakdown?: Record<number, number>;
  totalRequests: number;
}) {
  if (!breakdown || Object.keys(breakdown).length === 0 || totalRequests === 0) {
    return (
      <div className="empty">
        No logged requests in window to bucket
        <div style={{ marginTop: 6, color: C.emeraldSoft }}>All systems operational</div>
      </div>
    );
  }

  // Convert map to sorted entries
  const entries = Object.entries(breakdown)
    .map(([codeStr, count]) => {
      const code = parseInt(codeStr, 10);
      const info = getStatusInfo(code);
      return {
        code,
        count: Number(count),
        info,
        pct: (Number(count) / totalRequests) * 100,
      };
    })
    .sort((a, b) => {
      // Prioritize error codes (>=400) first, then sort by highest count
      if (a.code >= 400 && b.code < 400) return -1;
      if (a.code < 400 && b.code >= 400) return 1;
      return b.count - a.count;
    });

  const totalErrors = entries
    .filter((e) => e.code >= 400)
    .reduce((acc, cur) => acc + cur.count, 0);

  return (
    <div>
      {/* stacked distribution bar */}
      <div
        style={{
          display: "flex",
          height: 10,
          borderRadius: 3,
          overflow: "hidden",
          marginBottom: 12,
          background: "var(--surface-high)",
        }}
      >
        {entries.map((e) =>
          e.count > 0 ? (
            <div
              key={e.code}
              style={{
                width: `${Math.max(1, e.pct)}%`,
                background: e.info.color,
                transition: "width 0.3s ease",
              }}
              title={`${e.code} ${e.info.text}: ${e.count} (${e.pct.toFixed(1)}%)`}
            />
          ) : null,
        )}
      </div>

      {/* Summary health pill */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 10,
          padding: "5px 10px",
          borderRadius: 4,
          background: totalErrors > 0 ? "rgba(248, 113, 113, 0.08)" : "rgba(16, 185, 129, 0.08)",
          border: `1px solid ${totalErrors > 0 ? "rgba(248, 113, 113, 0.25)" : "rgba(16, 185, 129, 0.2)"}`,
          fontFamily: "var(--font-mono)",
          fontSize: 11,
        }}
      >
        <span style={{ color: totalErrors > 0 ? "var(--danger)" : "var(--emerald)", fontWeight: 600 }}>
          {totalErrors > 0 ? `${fmtNum(totalErrors)} errors (${((totalErrors / totalRequests) * 100).toFixed(1)}%)` : "0 errors (100% healthy)"}
        </span>
        <span style={{ color: "var(--text-muted)" }}>
          {fmtNum(totalRequests)} reqs
        </span>
      </div>

      {/* Per-status code breakdown list */}
      <div style={{ display: "flex", flexDirection: "column", gap: 5, maxHeight: 220, overflowY: "auto" }}>
        {entries.map((e) => (
          <div
            key={e.code}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "5px 8px",
              background: "var(--surface-raised)",
              borderRadius: 4,
              border: "1px solid var(--border)",
            }}
          >
            <span
              className="mono"
              style={{
                fontSize: 11,
                padding: "1px 6px",
                borderRadius: 3,
                background: e.info.bg,
                color: e.info.color,
                border: `1px solid ${e.info.border}`,
                fontWeight: 600,
                minWidth: 44,
                textAlign: "center",
              }}
            >
              {e.code}
            </span>
            <span style={{ flex: 1, fontSize: 12, color: e.code >= 400 ? "var(--text)" : "var(--text-muted)" }}>
              {e.info.text}
            </span>
            <span className="mono" style={{ fontSize: 12, fontWeight: 600 }}>
              {fmtNum(e.count)}
            </span>
            <span className="mono" style={{ fontSize: 11, color: C.faint, width: 44, textAlign: "right" }}>
              {e.pct.toFixed(1)}%
            </span>
          </div>
        ))}
      </div>

      <div className="muted" style={{ fontSize: 10, marginTop: 10, borderTop: `1px solid ${C.border}`, paddingTop: 8 }}>
        Exact HTTP status breakdown from all {fmtNum(totalRequests)} logged requests in the current window.
      </div>
    </div>
  );
}

function RecentTable({
  rows,
  onInspect,
}: {
  rows: DashboardStats["recent"];
  onInspect: (id: number) => void;
}) {
  if (!rows.length) return <div className="empty">No requests in the selected window</div>;
  return (
    <div className="table-wrap">
      <table className="table table--dense">
        <thead>
          <tr>
            <th>Time</th>
            <th>Provider</th>
            <th>Model</th>
            <th className="num">Status</th>
            <th className="num">TTFT</th>
            <th className="num">Latency</th>
            <th className="num">Cost</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr
              key={r.id ?? i}
              style={{ cursor: r.id ? "pointer" : "default" }}
              onClick={() => r.id && onInspect(r.id)}
            >
              <td className="mono" style={{ fontSize: 11, whiteSpace: "nowrap" }}>
                {shortTs(r.created_at)}
              </td>
              <td>{r.provider}</td>
              <td className="truncate" style={{ maxWidth: 160 }}>{r.model}</td>
              <td className="num">
                <StatusBadge code={r.status_code} />
              </td>
              <td className="num">{r.ttft_ms ? `${fmtNum(r.ttft_ms)}ms` : "—"}</td>
              <td className="num">{r.latency_ms ? `${fmtNum(r.latency_ms)}ms` : "—"}</td>
              <td className="num">{fmtMoney(r.cost_usd)}</td>
              <td style={{ textAlign: "right", paddingRight: 6 }}>
                {r.id && (
                  <button
                    className="btn btn--ghost btn--sm"
                    style={{
                      padding: "2px 8px",
                      height: "auto",
                      minHeight: "auto",
                      fontSize: 11,
                      fontFamily: "var(--font-mono)",
                    }}
                    onClick={(e) => {
                      e.stopPropagation();
                      onInspect(r.id!);
                    }}
                    title="Inspect request log"
                  >
                    Inspect
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ────────────────────────── Shared helpers ───────────────────────── */



function Sparkline({ values, color, height }: { values: number[]; color: string; height: number }) {
  const w = 120;
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = max - min || 1;
  const pts = values.map((v, i) => {
    const x = (i / (values.length - 1)) * w;
    const y = height - 2 - ((v - min) / span) * (height - 4);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const area = `0,${height} ${pts.join(" ")} ${w},${height}`;
  return (
    <svg width="100%" height={height} viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" style={{ display: "block", marginTop: 2 }}>
      <polygon points={area} fill={color} fillOpacity={0.12} />
      <polyline points={pts.join(" ")} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  );
}

function ChartBox({ height, children }: { height: number; children: ReactNode }) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      {children as ReactElement}
    </ResponsiveContainer>
  );
}

export function StatusBadge({ code }: { code: number }) {
  if (code < 300) return <span className="badge badge--success">{code}</span>;
  if (code < 400) return <span className="badge badge--info">{code}</span>;
  if (code < 500) return <span className="badge badge--warn">{code}</span>;
  return <span className="badge badge--error">{code}</span>;
}
