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
import type { DashboardStats, UsageByModel, UsageByUser } from "../api/types";
import { fmtMoney, fmtNum, shortTs } from "../lib/format";

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
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [users, setUsers] = useState<{ id: string; email: string }[]>([]);
  const [errStats, setErrStats] = useState<{ success: number; client: number; rateLimit: number; server: number } | null>(null);
  const [errSampled, setErrSampled] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const d = await api.dashboard({
        interval,
        user_id: userFilter || undefined,
        limit: 20,
      });
      setStats(d);

      // Error breakdown needs per-request status codes, which the dashboard
      // aggregates only as totals.error_rate. Pull the raw recent log rows for
      // the same 24h window and bucket them by status class here.
      const to = new Date().toISOString();
      const from = new Date(Date.now() - 24 * 3600 * 1000).toISOString();
      try {
        const page = await api.logs({ from, to, limit: 200 });
        const b = { success: 0, client: 0, rateLimit: 0, server: 0 };
        for (const r of page.data) {
          if (r.status_code < 400) b.success++;
          else if (r.status_code === 429) b.rateLimit++;
          else if (r.status_code < 500) b.client++;
          else b.server++;
        }
        setErrStats(b);
        setErrSampled(page.data.length);
      } catch {
        setErrStats(null);
        setErrSampled(0);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load dashboard");
    }
  }, [interval, userFilter]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    api.listUsers().then(setUsers).catch(() => {});
  }, []);

  const t = stats?.totals;
  const series = stats?.time_series ?? [];
  const successRate = t && t.requests > 0 ? 1 - t.error_rate : 0;

  const reqSpark = useMemo(() => series.map((p) => p.requests), [series]);
  const costSpark = useMemo(() => series.map((p) => p.cost_usd), [series]);

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">Overview</h1>
          <p className="page__sub">Live gateway usage across the last 24h</p>
        </div>
        <div className="row">
          <select
            className="select"
            style={{ width: 150 }}
            value={interval}
            onChange={(e) => setInterval(e.target.value)}
          >
            <option value="hour">Hourly buckets</option>
            <option value="day">Daily buckets</option>
          </select>
          <select
            className="select"
            style={{ width: 180 }}
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
          <button className="btn" onClick={load}>
            Refresh
          </button>
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        {/* ── Row 1 · Hero KPIs ─────────────────────────────────────────── */}
        <div className="kpi-grid" style={{ marginBottom: 0 }}>
          <HeroKpi label="Total Requests" value={fmtNum(t?.requests)} accent="cyan" spark={reqSpark} />
          <HeroKpi label="Total Cost" value={fmtMoney(t?.cost_usd)} accent="emerald" spark={costSpark} />
          <SuccessRateCard rate={successRate} />
          <HeroKpi
            label="Avg Latency"
            value={t ? `${t.avg_latency_ms.toFixed(0)} ms` : "—"}
            accent="violet"
            sub={t ? `AVG TTFT: ${t.avg_ttft_ms.toFixed(0)} ms` : "AVG TTFT: —"}
          />
        </div>

        {/* ── Row 2 · Tokens + cost trend ───────────────────────────────── */}
        <div className="dashboard-grid dashboard-grid--7-5">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Token Analytics <span className="mono">[prompt vs completion]</span>
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
              <Bar dataKey="completion_tokens" name="Completion" stackId="tokens" fill={C.violet} radius={[3, 3, 0, 0]} />
            </BarChart>
          </ChartBox>
          <LegendChips items={[{ c: C.cyan, label: "Prompt tokens" }, { c: C.violet, label: "Completion tokens" }]} />
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
            <StatTile label="AVG TTFT" value={t ? `${t.avg_ttft_ms.toFixed(0)} ms` : "—"} color={C.violet} />
            <StatTile label="AVG Latency" value={t ? `${t.avg_latency_ms.toFixed(0)} ms` : "—"} color={C.violet} />
          </div>
          <div className="stat-tiles-grid stat-tiles-grid--3">
            <StatTile label="P50 Latency" value={t && t.p50_latency_ms ? `${t.p50_latency_ms.toFixed(0)} ms` : "—"} color={C.violet} />
            <StatTile label="P95 Latency" value={t && t.p95_latency_ms ? `${t.p95_latency_ms.toFixed(0)} ms` : "—"} color={C.violet} />
            <StatTile label="P99 Latency" value={t && t.p99_latency_ms ? `${t.p99_latency_ms.toFixed(0)} ms` : "—"} color={C.violet} />
          </div>
        </div>
      </div>

      {/* ── Row 4 · Reliability ───────────────────────────────────────── */}
      <div className="dashboard-grid dashboard-grid--4-8">
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Error Breakdown <span className="mono">[by status]</span>
          </div>
          <ErrorBreakdown stats={errStats} sampled={errSampled} />
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Recent Requests <span className="mono">[recent]</span>
          </div>
          <RecentTable rows={stats?.recent ?? []} />
        </div>
      </div>

      {/* ── Row 5 · Optimization & security ───────────────────────────── */}
      <div className="dashboard-grid dashboard-grid--3-col">
        <NotTracked title="Prompt Cache Hit Rate" tag="PARTIAL">
          <p style={{ margin: 0, marginBottom: 8 }}>
            <span className="mono" style={{ fontSize: 11 }}>
              cached_input_tokens
            </span>{" "}
            are stored per request, so individual cache usage exists, but there is no aggregated hit-rate / $ saved query in
            the backend.
          </p>
          <MutedStat label="Cached input tokens (window)" value={t ? fmtCompact(sumField(stats, "prompt_tokens")) : "—"} />
        </NotTracked>

        <NotTracked title="Gateway Rate Limit Hits" tag="NOT TRACKED">
          <p style={{ margin: 0 }}>
            429 rate-limit and 401/403 quota rejections happen in the auth path{" "}
            <span className="mono" style={{ fontSize: 11 }}>
              before
            </span>{" "}
            telemetry is recorded, so the gateway cannot count its own rejections.
          </p>
        </NotTracked>

        <NotTracked title="Guardrails / Blocked Requests" tag="NOT TRACKED">
          <p style={{ margin: 0 }}>No PII or prompt-injection guardrail feature exists in the gateway codebase.</p>
        </NotTracked>
      </div>
      </div>
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
  stats,
  sampled,
}: {
  stats: { success: number; client: number; rateLimit: number; server: number } | null;
  sampled: number;
}) {
  if (!stats || sampled === 0)
    return (
      <div className="empty">
        No logged requests in window to bucket
        <div style={{ marginTop: 6, color: C.amberSoft }}>Backend aggregates AVG only</div>
      </div>
    );

  const buckets = [
    { key: "success", label: "2xx / 3xx", count: stats.success, color: C.emerald },
    { key: "client", label: "4xx (other)", count: stats.client, color: C.amber },
    { key: "rateLimit", label: "429 rate-limit", count: stats.rateLimit, color: C.amberSoft },
    { key: "server", label: "5xx provider", count: stats.server, color: C.red },
  ];
  const total = Math.max(stats.success + stats.client + stats.rateLimit + stats.server, 1);

  return (
    <div>
      {/* stacked distribution bar */}
      <div style={{ display: "flex", height: 10, borderRadius: 3, overflow: "hidden", marginBottom: 14 }}>
        {buckets.map((b) =>
          b.count > 0 ? (
            <div key={b.key} style={{ width: `${(b.count / total) * 100}%`, background: b.color }} title={`${b.label}: ${b.count}`} />
          ) : null,
        )}
      </div>
      {buckets.map((b) => (
        <div key={b.key} style={{ display: "flex", alignItems: "center", gap: 8, padding: "5px 0" }}>
          <span style={{ width: 8, height: 8, borderRadius: 2, background: b.color, display: "inline-block", flexShrink: 0 }} />
          <span style={{ flex: 1, fontSize: 12, color: C.muted }}>{b.label}</span>
          <span className="mono" style={{ fontSize: 12 }}>
            {fmtNum(b.count)}
          </span>
          <span className="mono" style={{ fontSize: 11, color: C.faint, width: 42, textAlign: "right" }}>
            {((b.count / total) * 100).toFixed(1)}%
          </span>
        </div>
      ))}
      <div className="muted" style={{ fontSize: 10, marginTop: 10, borderTop: `1px solid ${C.border}`, paddingTop: 8 }}>
        Bucketed from last {sampled} logged requests in window. Upstream statuses (400 context-window, 429, 5xx) are
        recorded per request; gateway-side 401/403/429 rejections are not.
      </div>
    </div>
  );
}

function RecentTable({ rows }: { rows: DashboardStats["recent"] }) {
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
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="mono" style={{ fontSize: 11, whiteSpace: "nowrap" }}>
                {shortTs(r.created_at)}
              </td>
              <td>{r.provider}</td>
              <td className="truncate">{r.model}</td>
              <td className="num">
                <StatusBadge code={r.status_code} />
              </td>
              <td className="num">{fmtNum(r.ttft_ms)} ms</td>
              <td className="num">{fmtNum(r.latency_ms)} ms</td>
              <td className="num">{fmtMoney(r.cost_usd)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ────────────────────────── Shared helpers ───────────────────────── */

function sumField(stats: DashboardStats | null, field: "prompt_tokens"): number {
  if (!stats) return 0;
  return stats.by_model.reduce((s, m) => s + (m[field] ?? 0), 0);
}

function NotTracked({ title, tag, children }: { title: string; tag: string; children: ReactNode }) {
  return (
    <div
      className="card"
      style={{ margin: 0, borderStyle: "dashed", borderColor: "rgba(245,158,11,0.35)", background: "rgba(245,158,11,0.03)" }}
    >
      <div className="card__title">{title}</div>
      <span className="badge badge--warn">{tag}</span>
      <div className="muted" style={{ fontSize: 12, marginTop: 10 }}>
        {children}
      </div>
    </div>
  );
}

function MutedStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 8, borderTop: `1px solid ${C.border}`, paddingTop: 8, marginTop: 4 }}>
      <span style={{ fontSize: 11, color: C.muted }}>{label}</span>
      <span className="mono" style={{ fontSize: 11 }}>
        {value}
      </span>
    </div>
  );
}

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
