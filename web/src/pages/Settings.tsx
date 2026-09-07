import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { SystemInfo } from "../api/types";

export default function Settings() {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [health, setHealth] = useState<{ status: string }>({ status: "down" });
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    setRefreshing(true);
    setError(null);
    try {
      const [i, h] = await Promise.all([api.systemInfo(), api.health()]);
      setInfo(i);
      setHealth(h);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load settings");
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const flag = (on: boolean) =>
    on ? (
      <span className="badge badge--success">enabled</span>
    ) : (
      <span className="badge badge--warn">not set</span>
    );

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">Settings</h1>
          <p className="page__sub">
            Runtime configuration, secrets redacted. Sensitive values are shown
            only as enabled/disabled flags.
          </p>
        </div>
        <button className="btn" onClick={load} disabled={refreshing}>
          {refreshing ? "Refreshing…" : "Refresh"}
        </button>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Health <span className="mono">[GET /health]</span>
          </div>
          <div
            className="kpi-grid"
            style={{ gridTemplateColumns: "repeat(2, 1fr)" }}
          >
            <div className="kpi">
              <div className="kpi__label">Gateway</div>
              <div className="kpi__value">
                <span className={`dot--${health.status === "ok" ? "ok" : "bad"}`} />{" "}
                {health.status}
              </div>
            </div>
            <div className="kpi">
              <div className="kpi__label">Version</div>
              <div className="kpi__value">{info?.version ?? "—"}</div>
            </div>
          </div>
        </div>

        <div className="card" style={{ margin: 0 }}>
          <div className="card__title">
            Listening ports <span className="mono">[env]</span>
          </div>
          <div
            className="kpi-grid"
            style={{ gridTemplateColumns: "repeat(2, 1fr)" }}
          >
            <div className="kpi">
              <div className="kpi__label">Public (LLM proxy)</div>
              <div className="kpi__value">{info?.port ?? "—"}</div>
            </div>
            <div className="kpi">
              <div className="kpi__label">Admin (dashboard)</div>
              <div className="kpi__value">{info?.admin_port ?? "—"}</div>
            </div>
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card__title">
          Database <span className="mono">[postgres]</span>
        </div>
        <div className="settings-grid">
          <Setting label="Host" value={info?.db_host} />
          <Setting label="Port" value={info?.db_port} />
          <Setting label="Name" value={info?.db_name} />
          <Setting label="User" value={info?.db_user} />
          <Setting label="SSL mode" value={info?.db_sslmode} />
          <Setting label="Redis" value={info?.redis_host} />
        </div>
      </div>

      <div className="card">
        <div className="card__title">
          Security <span className="mono">[flags]</span>
        </div>
        <div className="settings-grid">
          <div className="setting">
            <div className="setting__label">Admin API key</div>
            <div>{info ? flag(info.admin_api_key_set) : "—"}</div>
          </div>
          <div className="setting">
            <div className="setting__label">Provider master key</div>
            <div>{info ? flag(info.provider_master_key_enabled) : "—"}</div>
          </div>
          <div className="setting">
            <div className="setting__label">Admin IP whitelist</div>
            <div className="mono" style={{ fontSize: 13 }}>
              {info?.admin_allowed_ips ?? "—"}
            </div>
          </div>
          <div className="setting">
            <div className="setting__label">Environment</div>
            <div className="mono" style={{ fontSize: 13 }}>
              {info?.env ?? "—"}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function Setting({ label, value }: { label: string; value?: string }) {
  return (
    <div className="setting">
      <div className="setting__label">{label}</div>
      <div className="mono" style={{ fontSize: 13 }}>
        {value || "—"}
      </div>
    </div>
  );
}
