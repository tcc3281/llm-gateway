import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { AuditLog } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";

export default function AuditLogs() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [actorType, setActorType] = useState("");
  const [action, setAction] = useState("");
  const [targetType, setTargetType] = useState("");
  const [page, setPage] = useState(0);
  const pageSize = 50;

  // Inspect Modal
  const [inspectLog, setInspectLog] = useState<AuditLog | null>(null);

  const loadAuditLogs = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.auditLogs({
        actor_type: actorType || undefined,
        action: action.trim() || undefined,
        target_type: targetType || undefined,
        limit: pageSize,
        offset: page * pageSize,
      });
      setLogs(res.data);
      setTotal(res.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAuditLogs();
  }, [actorType, targetType, page]);

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(0);
    loadAuditLogs();
  };

  const getActionBadge = (act: string) => {
    if (act.includes("delete") || act.includes("revoke") || act.includes("remove")) {
      return {
        bg: "rgba(248, 113, 113, 0.12)",
        color: "var(--danger)",
        border: "rgba(248, 113, 113, 0.3)",
      };
    }
    if (act.includes("create") || act.includes("add")) {
      return {
        bg: "rgba(16, 185, 129, 0.12)",
        color: "var(--emerald)",
        border: "rgba(16, 185, 129, 0.3)",
      };
    }
    if (act.includes("update") || act.includes("set") || act.includes("reactivate")) {
      return {
        bg: "rgba(34, 211, 238, 0.12)",
        color: "var(--primary)",
        border: "rgba(34, 211, 238, 0.3)",
      };
    }
    return {
      bg: "rgba(168, 85, 247, 0.12)",
      color: "var(--violet)",
      border: "rgba(168, 85, 247, 0.3)",
    };
  };

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className="page__content">
      <div className="page__head">
        <div>
          <h1 className="page__title">Audit Trail</h1>
          <p className="page__desc">
            Immutable log of administrative operations, policy changes, and key lifecycle events for compliance and security auditing.
          </p>
        </div>
        <button className="btn btn--secondary" onClick={() => loadAuditLogs()}>
          <span className="material-symbols-outlined" style={{ fontSize: 18 }}>refresh</span>
          Refresh
        </button>
      </div>

      {error && <div className="error-banner">{error}</div>}

      {/* Filter Toolbar */}
      <form
        onSubmit={handleSearch}
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 12,
          alignItems: "center",
          marginBottom: 16,
          background: "var(--surface-raised)",
          padding: 12,
          borderRadius: 6,
          border: "1px solid var(--border)",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <span className="mono text-sm muted">Actor:</span>
          <select
            className="input"
            style={{ width: 130 }}
            value={actorType}
            onChange={(e) => {
              setActorType(e.target.value);
              setPage(0);
            }}
          >
            <option value="">All Actors</option>
            <option value="admin">Admin</option>
            <option value="root">Root</option>
            <option value="user">User</option>
            <option value="system">System</option>
          </select>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <span className="mono text-sm muted">Target:</span>
          <select
            className="input"
            style={{ width: 140 }}
            value={targetType}
            onChange={(e) => {
              setTargetType(e.target.value);
              setPage(0);
            }}
          >
            <option value="">All Targets</option>
            <option value="user">User</option>
            <option value="key">API Key</option>
            <option value="group">Group</option>
            <option value="policy">Policy</option>
            <option value="provider">Provider</option>
            <option value="model">Model</option>
          </select>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 6, flex: 1, minWidth: 200 }}>
          <input
            className="input"
            placeholder="Search action (e.g. key.create, user.delete)…"
            value={action}
            onChange={(e) => setAction(e.target.value)}
          />
          <button type="submit" className="btn btn--secondary">
            Filter
          </button>
        </div>
      </form>

      {/* Audit Log Table */}
      <div className="card">
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Action</th>
                <th>Actor</th>
                <th>Target</th>
                <th>Client IP</th>
                <th style={{ textAlign: "right" }}>Inspect</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={6} className="empty">Loading audit trail…</td>
                </tr>
              ) : logs.length === 0 ? (
                <tr>
                  <td colSpan={6} className="empty">No audit records found matching criteria.</td>
                </tr>
              ) : (
                logs.map((l) => {
                  const b = getActionBadge(l.action);
                  return (
                    <tr key={l.id}>
                      <td className="mono muted" style={{ fontSize: 12 }}>
                        {fmtDate(l.created_at)}
                      </td>
                      <td>
                        <span
                          className="mono"
                          style={{
                            fontSize: 11,
                            padding: "2px 8px",
                            borderRadius: 4,
                            background: b.bg,
                            color: b.color,
                            border: `1px solid ${b.border}`,
                            fontWeight: 600,
                          }}
                        >
                          {l.action}
                        </span>
                      </td>
                      <td>
                        <span className="mono font-bold" style={{ fontSize: 12, textTransform: "uppercase" }}>
                          [{l.actor_type}]
                        </span>
                      </td>
                      <td className="mono" style={{ fontSize: 12 }}>
                        {l.target_type && (
                          <span className="muted" style={{ marginRight: 6 }}>
                            {l.target_type}:
                          </span>
                        )}
                        <span style={{ color: "var(--text)" }}>{l.target_id || "—"}</span>
                      </td>
                      <td className="mono muted" style={{ fontSize: 11 }}>
                        {l.actor_ip || "—"}
                      </td>
                      <td style={{ textAlign: "right" }}>
                        <button
                          className="btn btn--ghost btn--sm"
                          onClick={() => setInspectLog(l)}
                        >
                          View Diff
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination Foot */}
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            padding: "12px 16px",
            borderTop: "1px solid var(--border)",
          }}
        >
          <span className="mono text-sm muted">
            Showing {logs.length} of {total} events
          </span>
          <div style={{ display: "flex", gap: 8 }}>
            <button
              className="btn btn--ghost btn--sm"
              disabled={page <= 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
            >
              Previous
            </button>
            <span className="mono text-sm" style={{ alignSelf: "center" }}>
              Page {page + 1} / {totalPages || 1}
            </span>
            <button
              className="btn btn--ghost btn--sm"
              disabled={page + 1 >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Next
            </button>
          </div>
        </div>
      </div>

      {/* Inspect Modal */}
      {inspectLog && (
        <Modal
          title={`Audit Event #${inspectLog.id} — ${inspectLog.action}`}
          onClose={() => setInspectLog(null)}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }} className="mono text-sm">
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
              <div><span className="muted">Actor:</span> <strong>{inspectLog.actor_type}</strong></div>
              <div><span className="muted">Client IP:</span> {inspectLog.actor_ip || "—"}</div>
              <div><span className="muted">Target:</span> {inspectLog.target_type} ({inspectLog.target_id})</div>
              <div><span className="muted">Recorded:</span> {fmtDate(inspectLog.created_at)}</div>
            </div>

            <div>
              <div className="form-label" style={{ marginBottom: 6 }}>Action Payload / Diff Details:</div>
              <pre
                style={{
                  background: "var(--surface-high)",
                  padding: 12,
                  borderRadius: 6,
                  border: "1px solid var(--border)",
                  fontSize: 12,
                  maxHeight: 280,
                  overflowY: "auto",
                  color: "var(--text)",
                }}
              >
                {inspectLog.details
                  ? JSON.stringify(inspectLog.details, null, 2)
                  : "(No payload recorded for this event)"}
              </pre>
            </div>

            <div className="modal-actions">
              <button className="btn btn--secondary" onClick={() => setInspectLog(null)}>
                Close
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
