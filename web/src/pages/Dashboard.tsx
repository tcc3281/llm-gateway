import { ReactElement, ReactNode, useCallback, useEffect, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api/client";
import type { DashboardStats, UsageByUser, User } from "../api/types";
import { fmtMoney, fmtNum, fmtPct, shortTs } from "../lib/format";

const tooltipStyle = {
  background: "#181a20",
  border: "1px solid #2d3139",
  borderRadius: 6,
  fontSize: 12,
  fontFamily: "JetBrains Mono, monospace",
};

export default function Dashboard() {
  const [interval, setInterval] = useState("hour");
  const [userFilter, setUserFilter] = useState("");
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const d = await api.dashboard({
        interval,
        user_id: userFilter || undefined,
        limit: 15,
      });
      setStats(d);
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

      <div className="kpi-grid">
        <Kpi label="Requests" value={fmtNum(t?.requests)} accent="accent" />
        <Kpi label="Cost (USD)" value={fmtMoney(t?.cost_usd)} accent="emerald" />
        <Kpi
          label="Tokens"
          value={fmtNum((t?.prompt_tokens ?? 0) + (t?.completion_tokens ?? 0))}
        />
        <Kpi
          label="Errors"
          value={fmtNum(t?.errors)}
          accent={t && t.errors > 0 ? "danger" : undefined}
          sub={fmtPct(t?.error_rate)}
        />
        <Kpi label="Avg TTFT" value={t ? `${t.avg_ttft_ms.toFixed(0)}ms` : "—"} />
        <Kpi label="Avg Latency" value={t ? `${t.avg_latency_ms.toFixed(0)}ms` : "—"} />
      </div>

      <div className="card">
        <div className="card__title">
          Requests &amp; cost over time <span className="mono">[time_series]</span>
        </div>
        <ChartBox height={240}>
          <ComposedChart data={stats?.time_series ?? []}>
            <CartesianGrid stroke="#2d3139" strokeDasharray="3 3" vertical={false} />
            <XAxis
              dataKey="time_bucket"
              tick={{ fill: "#859397", fontSize: 11 }}
              tickFormatter={(v: string) => shortTs(v)}
              axisLine={{ stroke: "#2d3139" }}
              tickLine={false}
            />
            <YAxis
              yAxisId="left"
              tick={{ fill: "#859397", fontSize: 11 }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              yAxisId="right"
              orientation="right"
              tick={{ fill: "#4edea3", fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              tickFormatter={(v: number) => `$${v}`}
            />
            <Tooltip contentStyle={tooltipStyle} labelFormatter={(v) => shortTs(String(v))} />
            <Bar yAxisId="left" dataKey="requests" fill="#22d3ee" fillOpacity={0.35} radius={[3, 3, 0, 0]} />
            <Line yAxisId="right" type="monotone" dataKey="cost_usd" stroke="#10b981" strokeWidth={2} dot={false} />
          </ComposedChart>
        </ChartBox>
      </div>

      <div className="card">
        <div className="card__title">
          Cost by model <span className="mono">[by_model]</span>
        </div>
        <ChartBox height={220}>
          <BarChart data={stats?.by_model ?? []}>
            <CartesianGrid stroke="#2d3139" strokeDasharray="3 3" vertical={false} />
            <XAxis
              dataKey="model"
              tick={{ fill: "#859397", fontSize: 10 }}
              interval={0}
              angle={-12}
              textAnchor="end"
              height={60}
              axisLine={{ stroke: "#2d3139" }}
              tickLine={false}
            />
            <YAxis
              tick={{ fill: "#859397", fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              tickFormatter={(v: number) => `$${v}`}
            />
            <Tooltip contentStyle={tooltipStyle} cursor={{ fill: "rgba(255,255,255,0.04)" }} />
            <Bar dataKey="cost_usd" fill="#a855f7" radius={[3, 3, 0, 0]} />
          </BarChart>
        </ChartBox>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Usage by user <span className="mono">[by_user]</span>
          </div>
          <ByUserTable rows={stats?.by_user ?? []} />
        </div>
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Recent requests <span className="mono">[recent]</span>
          </div>
          <RecentTable rows={stats?.recent ?? []} />
        </div>
      </div>
    </div>
  );
}

function Kpi({
  label,
  value,
  accent,
  sub,
}: {
  label: string;
  value: string;
  accent?: string;
  sub?: string;
}) {
  return (
    <div className="kpi">
      <div className="kpi__label">{label}</div>
      <div className={`kpi__value${accent ? ` kpi__value--${accent}` : ""}`}>
        {value}
      </div>
      {sub && <div className="kpi__delta">{sub}</div>}
    </div>
  );
}

function ChartBox({ height, children }: { height: number; children: ReactNode }) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      {children as ReactElement}
    </ResponsiveContainer>
  );
}

function ByUserTable({ rows }: { rows: UsageByUser[] }) {
  if (!rows.length)
    return <div className="empty">No usage in the selected window</div>;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th>User</th>
            <th className="num">Requests</th>
            <th className="num">Cost</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.user_id}>
              <td className="truncate">{r.email || r.user_id.slice(0, 8)}</td>
              <td className="num">{fmtNum(r.requests)}</td>
              <td className="num">{fmtMoney(r.cost_usd)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RecentTable({ rows }: { rows: DashboardStats["recent"] }) {
  if (!rows.length)
    return <div className="empty">No requests in the selected window</div>;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Provider</th>
            <th>Model</th>
            <th className="num">Status</th>
            <th className="num">Cost</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="mono" style={{ fontSize: 11 }}>
                {shortTs(r.created_at)}
              </td>
              <td>{r.provider}</td>
              <td className="truncate">{r.model}</td>
              <td className="num">
                <StatusBadge code={r.status_code} />
              </td>
              <td className="num">{fmtMoney(r.cost_usd)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function StatusBadge({ code }: { code: number }) {
  if (code < 300) return <span className="badge badge--success">{code}</span>;
  if (code < 400) return <span className="badge badge--info">{code}</span>;
  if (code < 500) return <span className="badge badge--warn">{code}</span>;
  return <span className="badge badge--error">{code}</span>;
}
