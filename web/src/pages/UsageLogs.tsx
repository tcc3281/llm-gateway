import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { UsageLogsPage, User } from "../api/types";
import { fmtDate, fmtMoney, fmtNum, localToRFC3339 } from "../lib/format";
import { StatusBadge } from "./Dashboard";

const PAGE_SIZE = 50;

interface Filters {
  provider: string;
  model: string;
  user_id: string;
  status: string;
  inbound: string;
  from: string;
  to: string;
}

const empty: Filters = {
  provider: "",
  model: "",
  user_id: "",
  status: "",
  inbound: "",
  from: "",
  to: "",
};

export default function UsageLogs() {
  const [filters, setFilters] = useState<Filters>(empty);
  const [page, setPage] = useState(0);
  const [data, setData] = useState<UsageLogsPage>({
    data: [],
    total: 0,
    limit: PAGE_SIZE,
    offset: 0,
  });
  const [users, setUsers] = useState<User[]>([]);
  const [providerOptions, setProviderOptions] = useState<string[]>([
    "openai",
    "anthropic",
    "gemini",
  ]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    async (f: Filters, offset: number) => {
      setError(null);
      setLoading(true);
      try {
        const res = await api.logs({
          from: localToRFC3339(f.from) || undefined,
          to: localToRFC3339(f.to) || undefined,
          user_id: f.user_id || undefined,
          provider: f.provider || undefined,
          model: f.model || undefined,
          status: f.status || undefined,
          inbound: f.inbound || undefined,
          limit: PAGE_SIZE,
          offset,
        });
        setData(res);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load logs");
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  useEffect(() => {
    load(filters, page * PAGE_SIZE);
  }, [load, filters, page]);

  useEffect(() => {
    api.listUsers().then(setUsers).catch(() => {});
  }, []);

  // Include custom compatible providers in the filter (logs store their name).
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

  const pages = Math.max(1, Math.ceil(data.total / PAGE_SIZE));
  const set = (k: keyof Filters) => (v: string) => {
    setPage(0);
    setFilters((prev) => ({ ...prev, [k]: v }));
  };

  return (
    <div>
      <div className="page__header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <h1 className="page__title">Usage Logs</h1>
          <p className="page__sub">
            {fmtNum(data.total)} request(s) match the current filters.
          </p>
        </div>
        <button
          className="btn btn--sm"
          disabled={loading}
          onClick={() => load(filters, page * PAGE_SIZE)}
        >
          {loading ? "Refreshing..." : "🔄 Refresh"}
        </button>
      </div>

      <div className="card">
        <div className="filter-bar">
          <select
            className="select"
            style={{ width: 130 }}
            value={filters.provider}
            onChange={(e) => set("provider")(e.target.value)}
          >
            <option value="">Any provider</option>
            {providerOptions.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
          <input
            className="input"
            style={{ width: 180 }}
            placeholder="Model…"
            value={filters.model}
            onChange={(e) => set("model")(e.target.value)}
          />
          <select
            className="select"
            style={{ width: 170 }}
            value={filters.user_id}
            onChange={(e) => set("user_id")(e.target.value)}
          >
            <option value="">Any user</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.email}
              </option>
            ))}
          </select>
          <select
            className="select"
            style={{ width: 110 }}
            value={filters.status}
            onChange={(e) => set("status")(e.target.value)}
          >
            <option value="">Any status</option>
            <option value="2xx">2xx</option>
            <option value="3xx">3xx</option>
            <option value="4xx">4xx</option>
            <option value="5xx">5xx</option>
            <option value="429">429</option>
          </select>
          <select
            className="select"
            style={{ width: 120 }}
            value={filters.inbound}
            onChange={(e) => set("inbound")(e.target.value)}
          >
            <option value="">Any format</option>
            <option value="openai">openai</option>
            <option value="anthropic">anthropic</option>
          </select>
          <input
            className="input"
            style={{ width: 170 }}
            type="datetime-local"
            value={filters.from}
            onChange={(e) => set("from")(e.target.value)}
            aria-label="From"
          />
          <span className="muted">→</span>
          <input
            className="input"
            style={{ width: 170 }}
            type="datetime-local"
            value={filters.to}
            onChange={(e) => set("to")(e.target.value)}
            aria-label="To"
          />
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div className="table-wrap">
        <table className="table table--dense">
          <thead>
            <tr>
              <th>Time</th>
              <th>Provider</th>
              <th>Model</th>
              <th className="num">Status</th>
              <th>In</th>
              <th className="num">In Tk</th>
              <th className="num">Out Tk</th>
              <th className="num">TTFT</th>
              <th className="num">Latency</th>
              <th className="num">Cost</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((l) => (
              <tr key={l.id}>
                <td className="mono" style={{ fontSize: 11 }}>
                  {fmtDate(l.created_at)}
                </td>
                <td>{l.provider}</td>
                <td className="truncate" style={{ maxWidth: 220 }}>
                  {l.model}
                </td>
                <td className="num">
                  <StatusBadge code={l.status_code} />
                </td>
                <td className="muted">{l.inbound_format}</td>
                <td className="num">{fmtNum(l.prompt_tokens)}</td>
                <td className="num">{fmtNum(l.completion_tokens)}</td>
                <td className="num">{l.ttft_ms ? `${l.ttft_ms}ms` : "—"}</td>
                <td className="num">
                  {l.total_latency_ms ? `${l.total_latency_ms}ms` : "—"}
                </td>
                <td className="num">{fmtMoney(l.cost)}</td>
                <td className="truncate muted" style={{ maxWidth: 180 }}>
                  {l.error_message}
                </td>
              </tr>
            ))}
            {!data.data.length && (
              <tr>
                <td colSpan={11}>
                  <div className="empty">No logs match the current filters</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {pages > 1 && (
        <div className="pagination">
          <button
            className="btn btn--sm"
            disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            ← Prev
          </button>
          <span className="mono" style={{ fontSize: 12 }}>
            {page + 1} / {pages}
          </span>
          <button
            className="btn btn--sm"
            disabled={page + 1 >= pages}
            onClick={() => setPage((p) => Math.min(pages - 1, p + 1))}
          >
            Next →
          </button>
        </div>
      )}
    </div>
  );
}
