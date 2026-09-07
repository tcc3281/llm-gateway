import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { APIKey, UsageLog, UsageLogsPage, User } from "../api/types";
import { fmtDate, fmtMoney, fmtNum, localToRFC3339 } from "../lib/format";
import { StatusBadge } from "./Dashboard";
import LogDetailDrawer from "../components/LogDetailDrawer";

const PAGE_SIZE = 50;

function formatLocalDatetime(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const yyyy = date.getFullYear();
  const MM = pad(date.getMonth() + 1);
  const dd = pad(date.getDate());
  const hh = pad(date.getHours());
  const mm = pad(date.getMinutes());
  return `${yyyy}-${MM}-${dd}T${hh}:${mm}`;
}

const getInitialFromTime = () => {
  return formatLocalDatetime(new Date(Date.now() - 24 * 3600 * 1000));
};

interface Filters {
  provider: string;
  model: string;
  user_id: string;
  key_id: string;
  status: string;
  inbound: string;
  from: string;
  to: string;
}

const empty: Filters = {
  provider: "",
  model: "",
  user_id: "",
  key_id: "",
  status: "",
  inbound: "",
  from: "",
  to: "",
};

const initialFilters: Filters = {
  ...empty,
  from: getInitialFromTime(),
};

export default function UsageLogs() {
  const [filters, setFilters] = useState<Filters>(initialFilters);
  const [preset, setPreset] = useState<"24h" | "7d" | "30d" | "all" | "custom">("24h");
  const [modelInput, setModelInput] = useState(filters.model);
  const [page, setPage] = useState(0);
  const [data, setData] = useState<UsageLogsPage>({
    data: [],
    total: 0,
    limit: PAGE_SIZE,
    offset: 0,
  });
  const [users, setUsers] = useState<User[]>([]);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [selectedLog, setSelectedLog] = useState<UsageLog | null>(null);
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
          key_id: f.key_id || undefined,
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

  // Debounce search input to avoid API request storm
  useEffect(() => {
    const timer = setTimeout(() => {
      setPage(0);
      setFilters((prev) => {
        if (prev.model === modelInput) return prev;
        return { ...prev, model: modelInput };
      });
    }, 350);
    return () => clearTimeout(timer);
  }, [modelInput]);

  useEffect(() => {
    load(filters, page * PAGE_SIZE);
  }, [load, filters, page]);

  useEffect(() => {
    api.listUsers().then(setUsers).catch(() => {});
  }, []);

  // Load API keys for the selected user (or all keys if no user selected)
  useEffect(() => {
    api.listKeys(filters.user_id || undefined).then(setKeys).catch(() => {});
  }, [filters.user_id]);

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

  const handlePresetChange = (newPreset: "24h" | "7d" | "30d" | "all" | "custom") => {
    setPreset(newPreset);
    setPage(0);
    if (newPreset === "24h") {
      setFilters((prev) => ({
        ...prev,
        from: formatLocalDatetime(new Date(Date.now() - 24 * 3600 * 1000)),
        to: "",
      }));
    } else if (newPreset === "7d") {
      setFilters((prev) => ({
        ...prev,
        from: formatLocalDatetime(new Date(Date.now() - 7 * 24 * 3600 * 1000)),
        to: "",
      }));
    } else if (newPreset === "30d") {
      setFilters((prev) => ({
        ...prev,
        from: formatLocalDatetime(new Date(Date.now() - 30 * 24 * 3600 * 1000)),
        to: "",
      }));
    } else if (newPreset === "all") {
      setFilters((prev) => ({
        ...prev,
        from: "",
        to: "",
      }));
    }
  };

  const handleCustomDateChange = (k: "from" | "to", val: string) => {
    setPreset("custom");
    setPage(0);
    setFilters((prev) => ({ ...prev, [k]: val }));
  };

  const clearFilters = () => {
    setModelInput("");
    setPreset("24h");
    setFilters({
      ...empty,
      from: formatLocalDatetime(new Date(Date.now() - 24 * 3600 * 1000)),
    });
    setPage(0);
  };

  const isDirty =
    filters.provider !== "" ||
    modelInput !== "" ||
    filters.user_id !== "" ||
    filters.key_id !== "" ||
    filters.status !== "" ||
    filters.inbound !== "" ||
    preset !== "24h";

  return (
    <div>
      <div className="page__header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 16 }}>
        <div>
          <h1 className="page__title">Usage Logs</h1>
          <p className="page__sub">
            {fmtNum(data.total)} request(s) match the current filters.
          </p>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          {/* Time presets button group */}
          <div className="time-presets" style={{ display: "flex", background: "var(--surface-raised)", border: "1px solid var(--border)", borderRadius: 4, height: 32, padding: 2 }}>
            {(["24h", "7d", "30d", "all", "custom"] as const).map((p) => {
              const active = preset === p;
              return (
                <button
                  key={p}
                  className={`btn btn--sm ${active ? "btn--primary" : "btn--ghost"}`}
                  style={{
                    border: "none",
                    background: active ? "var(--primary-dim)" : "transparent",
                    color: active ? "var(--primary)" : "var(--text-muted)",
                    fontWeight: active ? 600 : 400,
                    height: "100%",
                    padding: "0 10px",
                    borderRadius: 3,
                    minHeight: "auto",
                    boxShadow: "none",
                  }}
                  onClick={() => handlePresetChange(p)}
                >
                  {p}
                </button>
              );
            })}
          </div>
          <button
            className="btn btn--primary"
            style={{ height: 32, display: "flex", alignItems: "center", gap: 6 }}
            disabled={loading}
            onClick={() => load(filters, page * PAGE_SIZE)}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>refresh</span>
            {loading ? "Refreshing..." : "Refresh"}
          </button>
        </div>
      </div>

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
          value={modelInput}
          onChange={(e) => setModelInput(e.target.value)}
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
          style={{ width: 160 }}
          value={filters.key_id}
          onChange={(e) => set("key_id")(e.target.value)}
        >
          <option value="">Any API key</option>
          {keys.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name} ({k.key_prefix}…)
            </option>
          ))}
        </select>
        <select
          className="select"
          style={{ width: 140 }}
          value={filters.status}
          onChange={(e) => set("status")(e.target.value)}
        >
          <option value="">Any status</option>
          <optgroup label="Classes">
            <option value="2xx">2xx (Success)</option>
            <option value="3xx">3xx (Redirect)</option>
            <option value="4xx">4xx (Client Error)</option>
            <option value="5xx">5xx (Server Error)</option>
          </optgroup>
          <optgroup label="Specific Codes">
            <option value="200">200 OK</option>
            <option value="400">400 Bad Request</option>
            <option value="401">401 Unauthorized</option>
            <option value="403">403 Forbidden</option>
            <option value="404">404 Not Found</option>
            <option value="429">429 Rate Limited</option>
            <option value="500">500 Server Error</option>
            <option value="502">502 Bad Gateway</option>
            <option value="503">503 Unavailable</option>
            <option value="504">504 Gateway Timeout</option>
          </optgroup>
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
        {preset === "custom" && (
          <>
            <input
              className="input"
              style={{ width: 170 }}
              type="datetime-local"
              value={filters.from}
              onChange={(e) => handleCustomDateChange("from", e.target.value)}
              onClick={(e) => {
                try {
                  e.currentTarget.showPicker();
                } catch {}
              }}
              aria-label="From"
            />
            <span className="muted">→</span>
            <input
              className="input"
              style={{ width: 170 }}
              type="datetime-local"
              value={filters.to}
              onChange={(e) => handleCustomDateChange("to", e.target.value)}
              onClick={(e) => {
                try {
                  e.currentTarget.showPicker();
                } catch {}
              }}
              aria-label="To"
            />
          </>
        )}
        {isDirty && (
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
              <th></th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((l) => (
              <tr
                key={l.id}
                style={{ cursor: "pointer" }}
                onClick={() => setSelectedLog(l)}
              >
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
                <td style={{ textAlign: "right", paddingRight: 6 }}>
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
                      setSelectedLog(l);
                    }}
                    title="Inspect full log details"
                  >
                    Inspect
                  </button>
                </td>
              </tr>
            ))}
            {!data.data.length && (
              <tr>
                <td colSpan={12}>
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

      {/* Slide-over Inspection Drawer */}
      <LogDetailDrawer
        logId={selectedLog?.id ?? null}
        initialLog={selectedLog}
        onClose={() => setSelectedLog(null)}
      />
    </div>
  );
}
