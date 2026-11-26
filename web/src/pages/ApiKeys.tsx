import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { APIKey, KeyModelPolicy, User } from "../api/types";
import { fmtDate, fmtMoney, fmtNum } from "../lib/format";
import Modal from "../components/Modal";
import { useToast } from "../components/Toast";
import { useLanguage } from "../i18n";

interface KeyForm {
  user_id: string;
  name: string;
  budget_limit: string;
  rate_limit_rpm: string;
  cache_ttl_seconds: string;
  expires_at: string;
}

const empty: KeyForm = {
  user_id: "",
  name: "",
  budget_limit: "0",
  rate_limit_rpm: "60",
  cache_ttl_seconds: "0",
  expires_at: "",
};

export default function ApiKeys() {
  const toast = useToast();
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [userFilter, setUserFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<KeyForm>(empty);
  const [createdSecret, setCreatedSecret] = useState<string | null>(null);

  const [editTarget, setEditTarget] = useState<APIKey | null>(null);
  const [editForm, setEditForm] = useState<KeyForm>(empty);

  const [confirmTarget, setConfirmTarget] = useState<{
    key: APIKey;
    action: "revoke" | "reactivate";
  } | null>(null);

  const [modelTarget, setModelTarget] = useState<APIKey | null>(null);
  const [modelPolicies, setModelPolicies] = useState<KeyModelPolicy[]>([]);
  const [newModelPattern, setNewModelPattern] = useState("");
  const [newIsAllowlist, setNewIsAllowlist] = useState(true);
  const [modelPolicyLoading, setModelPolicyLoading] = useState(false);

  const openModelRules = async (k: APIKey) => {
    setModelTarget(k);
    setNewModelPattern("");
    setNewIsAllowlist(true);
    try {
      setModelPolicyLoading(true);
      const res = await api.listKeyModelPolicies(k.id);
      setModelPolicies(res);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load model rules";
      toast.error(msg, "[RULES ERROR]");
    } finally {
      setModelPolicyLoading(false);
    }
  };

  const handleAddModelRule = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!modelTarget || !newModelPattern.trim()) return;
    try {
      await api.createKeyModelPolicy(modelTarget.id, {
        model_pattern: newModelPattern.trim(),
        is_allowlist: newIsAllowlist,
      });
      toast.success(`Model policy "${newModelPattern.trim()}" added`, "[200 OK]");
      setNewModelPattern("");
      const res = await api.listKeyModelPolicies(modelTarget.id);
      setModelPolicies(res);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to add model rule";
      toast.error(msg, "[POLICY ERROR]");
    }
  };

  const handleDeleteModelRule = async (policyId: string) => {
    if (!modelTarget) return;
    try {
      await api.deleteKeyModelPolicy(modelTarget.id, policyId);
      toast.info("Model rule removed", "[200 OK]");
      const res = await api.listKeyModelPolicies(modelTarget.id);
      setModelPolicies(res);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to remove model rule";
      toast.error(msg, "[DELETE ERROR]");
    }
  };

  const load = useCallback(async () => {
    setError(null);
    try {
      setKeys(await api.listKeys(userFilter || undefined));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load API keys");
    }
  }, [userFilter]);

  const loadUsers = useCallback(async () => {
    try {
      setUsers(await api.listUsers());
    } catch (e) {
      console.error("Failed to load users", e);
    }
  }, []);

  useEffect(() => {
    load();
    loadUsers();
  }, [load, loadUsers]);

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.user_id || !form.name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.createKey({
        user_id: form.user_id,
        name: form.name.trim(),
        budget_limit: parseFloat(form.budget_limit) || 0,
        rate_limit_rpm: parseInt(form.rate_limit_rpm, 10) || 60,
        cache_ttl_seconds: parseInt(form.cache_ttl_seconds, 10) ?? 0,
      });
      setCreatedSecret(res.api_key);
      toast.success(`Virtual API key "${form.name}" created`, "[200 OK]");
      setForm(empty);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Create failed";
      setError(msg);
      toast.error(msg, "[CREATE ERROR]");
    } finally {
      setBusy(false);
    }
  };

  const closeCreate = () => {
    setCreateOpen(false);
    setCreatedSecret(null);
  };

  const submitEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editTarget) return;
    setBusy(true);
    setError(null);
    try {
      await api.updateKey(editTarget.id, {
        name: editForm.name,
        budget_limit: parseFloat(editForm.budget_limit) || 0,
        rate_limit_rpm: parseInt(editForm.rate_limit_rpm, 10) || 0,
        cache_ttl_seconds: parseInt(editForm.cache_ttl_seconds, 10) ?? 0,
        expires_at: editForm.expires_at
          ? `${editForm.expires_at}:00Z`
          : null,
      });
      toast.success("API key settings updated", "[200 OK]");
      setEditTarget(null);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Update failed";
      setError(msg);
      toast.error(msg, "[UPDATE ERROR]");
    } finally {
      setBusy(false);
    }
  };

  const runConfirm = async () => {
    if (!confirmTarget) return;
    setBusy(true);
    setError(null);
    try {
      if (confirmTarget.action === "revoke") {
        await api.revokeKey(confirmTarget.key.id);
        toast.info(`Key ${confirmTarget.key.name} revoked`, "[200 OK]");
      } else {
        await api.reactivateKey(confirmTarget.key.id);
        toast.success(`Key ${confirmTarget.key.name} reactivated`, "[200 OK]");
      }
      setConfirmTarget(null);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Operation failed";
      setError(msg);
      toast.error(msg, "[OPERATION ERROR]");
    } finally {
      setBusy(false);
    }
  };

  const { t } = useLanguage();

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">{t("keys.title", "API Keys")}</h1>
          <p className="page__sub">
            {keys.length} {t("keys.subtitle", "Manage gateway client credentials, RPM limits, and model access policies")}
          </p>
        </div>
        <div className="row">
          <select
            className="select"
            style={{ width: 180 }}
            value={userFilter}
            onChange={(e) => setUserFilter(e.target.value)}
          >
            <option value="">{t("keys.filter_all_users", "All users")}</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.email}
              </option>
            ))}
          </select>
          <button className="btn btn--primary" onClick={() => setCreateOpen(true)}>
            {t("keys.create_btn", "+ New key")}
          </button>
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>{t("keys.col_name", "Name")}</th>
              <th>{t("keys.col_prefix", "Prefix")}</th>
              <th>{t("keys.col_user", "User")}</th>
              <th className="num">{t("keys.col_budget", "Budget")}</th>
              <th className="num">{t("keys.col_rpm", "RPM")}</th>
              <th>{t("keys.col_cache", "Cache")}</th>
              <th>{t("keys.col_status", "Status")}</th>
              <th>{t("keys.col_expires", "Expires")}</th>
              <th style={{ textAlign: "right" }}>{t("keys.col_actions", "Actions")}</th>
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.id}>
                <td>{k.name || "—"}</td>
                <td className="mono" style={{ fontSize: 11 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <span>{k.key_prefix}…</span>
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm"
                      style={{ padding: "0 4px", minHeight: 20, fontSize: 11 }}
                      title="Copy key prefix"
                      onClick={() => {
                        navigator.clipboard.writeText(k.key_prefix);
                        toast.info(`Key prefix ${k.key_prefix} copied`, "[COPIED]");
                      }}
                    >
                      {t("common.copy", "copy")}
                    </button>
                  </div>
                </td>
                <td className="muted">{userLabel(k.user_id, users)}</td>
                <td className="num">{fmtMoney(k.budget_limit, 2)}</td>
                <td className="num">{fmtNum(k.rate_limit_rpm)}</td>
                <td>
                  <span className="muted" style={{ fontSize: 12 }}>
                    {fmtCache(k.cache_ttl_seconds ?? 0)}
                  </span>
                </td>
                <td>
                  {k.is_active ? (
                    <span className="badge badge--success">{t("common.active", "active")}</span>
                  ) : (
                    <span className="badge badge--error">{t("keys.revoke", "revoked")}</span>
                  )}
                </td>
                <td className="muted">{fmtDate(k.expires_at)}</td>
                <td>
                  <div className="table__actions">
                    <button
                      className="btn btn--sm"
                      onClick={() => openModelRules(k)}
                    >
                      {t("keys.models_btn", "Models")}
                    </button>
                    <button
                      className="btn btn--sm"
                      onClick={() => {
                        setEditTarget(k);
                        setEditForm({
                          user_id: k.user_id,
                          name: k.name,
                          budget_limit: String(k.budget_limit),
                          rate_limit_rpm: String(k.rate_limit_rpm),
                          cache_ttl_seconds: String(k.cache_ttl_seconds ?? 0),
                          expires_at: k.expires_at
                            ? k.expires_at.replace("Z", "").slice(0, 16)
                            : "",
                        });
                      }}
                    >
                      {t("common.edit", "Edit")}
                    </button>
                    {k.is_active ? (
                      <button
                        className="btn btn--sm btn--danger"
                        onClick={() => setConfirmTarget({ key: k, action: "revoke" })}
                      >
                        {t("keys.revoke", "Revoke")}
                      </button>
                    ) : (
                      <button
                        className="btn btn--sm"
                        onClick={() =>
                          setConfirmTarget({ key: k, action: "reactivate" })
                        }
                      >
                        {t("keys.reactivate", "Re-enable")}
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {!keys.length && (
              <tr>
                <td colSpan={8}>
                  <div className="empty">{t("keys.empty", "No API keys yet")}</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {createOpen && (
        <Modal
          title={createdSecret ? t("keys.modal_created_title", "Key created — copy it now") : t("keys.modal_create_title", "New API key")}
          onClose={closeCreate}
          footer={
            createdSecret ? (
              <button className="btn btn--primary" onClick={closeCreate}>
                {t("common.done", "Done")}
              </button>
            ) : (
              <>
                <button className="btn btn--ghost" onClick={closeCreate}>
                  {t("common.cancel", "Cancel")}
                </button>
                <button
                  className="btn btn--primary"
                  onClick={submitCreate}
                  disabled={busy || !form.user_id || !form.name.trim()}
                >
                  {busy ? t("common.creating", "Creating…") : t("common.create", "Create key")}
                </button>
              </>
            )
          }
        >
          {createdSecret ? (
            <div className="field">
              <label className="field__label">
                {t("keys.secret_warning", "Secret key (shown once — store it securely)")}
              </label>
              <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
                <div className="secret-box" style={{ flex: 1, wordBreak: "break-all" }}>{createdSecret}</div>
                <button
                  type="button"
                  className="btn btn--primary"
                  style={{ flexShrink: 0 }}
                  onClick={() => {
                    navigator.clipboard.writeText(createdSecret);
                    toast.info("Virtual API key copied to clipboard", "[COPIED]");
                  }}
                >
                  {t("common.copy", "Copy Key")}
                </button>
              </div>

              <div style={{ marginTop: 14 }}>
                <label className="field__label">🚀 Quick Test with cURL (https://vin-ai-t205.io.vn)</label>
                <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                  <div
                    style={{
                      flex: 1,
                      background: "#111318",
                      border: "1px solid #232730",
                      borderRadius: 6,
                      padding: "8px 12px",
                      fontFamily: "var(--font-mono, monospace)",
                      fontSize: 11,
                      color: "#a5b4fc",
                      overflowX: "auto",
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-all",
                    }}
                  >
                    {`curl.exe -X POST https://vin-ai-t205.io.vn/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${createdSecret}" \\\n  -d '{"model": "deepseek-chat", "messages": [{"role": "user", "content": "Hello LLM Gateway!"}]}'`}
                  </div>
                  <button
                    type="button"
                    className="btn btn--ghost"
                    style={{ flexShrink: 0 }}
                    onClick={() => {
                      const curlStr = `curl.exe -X POST https://vin-ai-t205.io.vn/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${createdSecret}" \\\n  -d '{\n    "model": "deepseek-chat",\n    "messages": [{"role": "user", "content": "Hello LLM Gateway!"}]\n  }'`;
                      navigator.clipboard.writeText(curlStr);
                      toast.info("cURL command copied to clipboard", "[COPIED]");
                    }}
                  >
                    Copy cURL
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <KeyFields form={form} setForm={setForm} users={users} withUser />
          )}
        </Modal>
      )}

      {editTarget && (
        <Modal
          title={t("keys.modal_edit_title", "Edit key")}
          onClose={() => setEditTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setEditTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={submitEdit}
                disabled={busy}
              >
                {busy ? t("common.saving", "Saving…") : t("common.save", "Save")}
              </button>
            </>
          }
        >
          <KeyFields form={editForm} setForm={setEditForm} users={users} />
        </Modal>
      )}

      {confirmTarget && (
        <Modal
          title={confirmTarget.action === "revoke" ? t("keys.revoke", "Revoke key") : t("keys.reactivate", "Re-enable key")}
          onClose={() => setConfirmTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setConfirmTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className={
                  confirmTarget.action === "revoke"
                    ? "btn btn--danger"
                    : "btn btn--primary"
                }
                onClick={runConfirm}
                disabled={busy}
              >
                {busy
                  ? t("common.loading", "Working…")
                  : confirmTarget.action === "revoke"
                    ? t("keys.revoke", "Revoke")
                    : t("keys.reactivate", "Re-enable")}
              </button>
            </>
          }
        >
          <p>
            {confirmTarget.action === "revoke"
              ? `Revoke ${confirmTarget.key.name || confirmTarget.key.key_prefix}? Revoked keys stop working immediately (Redis cache is invalidated).`
              : `Re-enable ${confirmTarget.key.name || confirmTarget.key.key_prefix}?`}
          </p>
        </Modal>
      )}

      {modelTarget && (
        <Modal
          title={`Model Restrictions: ${modelTarget.name || modelTarget.key_prefix}`}
          onClose={() => setModelTarget(null)}
          footer={
            <button className="btn btn--secondary" onClick={() => setModelTarget(null)}>
              Done
            </button>
          }
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <p className="muted" style={{ margin: 0, fontSize: 13 }}>
              Configure per-key allowlist (whitelist) or denylist (blacklist) wildcard rules (e.g. <code>gpt-4o</code>, <code>claude-*</code>).
              If allowlist rules exist, the key may only access models matching at least one allowlist rule. Denylist rules always reject matching models.
            </p>

            {/* Add Rule Form */}
            <form
              onSubmit={handleAddModelRule}
              style={{
                display: "grid",
                gridTemplateColumns: "2fr 1.2fr auto",
                gap: 8,
                background: "var(--surface-high)",
                padding: 12,
                borderRadius: 6,
                border: "1px solid var(--border)",
                alignItems: "end",
              }}
            >
              <div className="field" style={{ margin: 0 }}>
                <label className="field__label">Model Pattern</label>
                <input
                  className="input"
                  required
                  placeholder="e.g. claude-* or gpt-4o"
                  value={newModelPattern}
                  onChange={(e) => setNewModelPattern(e.target.value)}
                />
              </div>

              <div className="field" style={{ margin: 0 }}>
                <label className="field__label">Rule Type</label>
                <select
                  className="select"
                  value={newIsAllowlist ? "allow" : "deny"}
                  onChange={(e) => setNewIsAllowlist(e.target.value === "allow")}
                >
                  <option value="allow">Allowlist (Only this)</option>
                  <option value="deny">Denylist (Block this)</option>
                </select>
              </div>

              <button type="submit" className="btn btn--primary" style={{ height: 38 }}>
                Add Rule
              </button>
            </form>

            {/* Rules List */}
            <div style={{ border: "1px solid var(--border)", borderRadius: 6, overflow: "hidden" }}>
              <table className="table" style={{ margin: 0 }}>
                <thead>
                  <tr>
                    <th>Pattern</th>
                    <th>Type</th>
                    <th style={{ textAlign: "right" }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {modelPolicyLoading ? (
                    <tr>
                      <td colSpan={3} className="empty">Loading rules…</td>
                    </tr>
                  ) : modelPolicies.length === 0 ? (
                    <tr>
                      <td colSpan={3} className="empty">
                        No model restrictions configured (all models allowed by default subject to user IAM policies).
                      </td>
                    </tr>
                  ) : (
                    modelPolicies.map((p) => (
                      <tr key={p.id}>
                        <td className="mono font-bold">{p.model_pattern}</td>
                        <td>
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 8px",
                              borderRadius: 4,
                              background: p.is_allowlist ? "rgba(16, 185, 129, 0.12)" : "rgba(248, 113, 113, 0.12)",
                              color: p.is_allowlist ? "var(--emerald)" : "var(--danger)",
                              border: `1px solid ${p.is_allowlist ? "rgba(16, 185, 129, 0.3)" : "rgba(248, 113, 113, 0.3)"}`,
                              fontWeight: 600,
                            }}
                          >
                            {p.is_allowlist ? "ALLOWLIST" : "DENYLIST"}
                          </span>
                        </td>
                        <td style={{ textAlign: "right" }}>
                          <button
                            className="btn btn--sm btn--danger"
                            onClick={() => handleDeleteModelRule(p.id)}
                          >
                            Remove
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

function KeyFields({
  form,
  setForm,
  users,
  withUser,
}: {
  form: KeyForm;
  setForm: (f: KeyForm) => void;
  users: User[];
  withUser?: boolean;
}) {
  return (
    <>
      {withUser && (
        <div className="field">
          <label className="field__label">User</label>
          <select
            className="select"
            value={form.user_id}
            onChange={(e) => setForm({ ...form, user_id: e.target.value })}
          >
            <option value="">Select user…</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.email}
              </option>
            ))}
          </select>
        </div>
      )}
      <div className="field">
        <label className="field__label">Name</label>
        <input
          className="input"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="App Key"
        />
      </div>
      <div className="row">
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">Budget limit (USD, 0 = unlimited)</label>
          <input
            className="input"
            type="number"
            min="0"
            step="0.01"
            value={form.budget_limit}
            onChange={(e) => setForm({ ...form, budget_limit: e.target.value })}
          />
        </div>
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">Rate limit (RPM)</label>
          <input
            className="input"
            type="number"
            min="1"
            value={form.rate_limit_rpm}
            onChange={(e) => setForm({ ...form, rate_limit_rpm: e.target.value })}
          />
        </div>
      </div>
      <div className="row">
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">
            Cache TTL (seconds: 0=global 1h, -1=off)
          </label>
          <input
            className="input"
            type="number"
            step="1"
            value={form.cache_ttl_seconds}
            onChange={(e) =>
              setForm({ ...form, cache_ttl_seconds: e.target.value })
            }
          />
        </div>
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">Expires (empty = never)</label>
          <input
            className="input"
            type="datetime-local"
            value={form.expires_at}
            onChange={(e) => setForm({ ...form, expires_at: e.target.value })}
            onClick={(e) => {
              try {
                e.currentTarget.showPicker();
              } catch {}
            }}
          />
        </div>
      </div>
    </>
  );
}

function fmtCache(ttl: number): string {
  if (ttl === -1) return "Off";
  if (ttl === 0) return "Default (1h)";
  if (ttl % 3600 === 0) return `${ttl / 3600}h`;
  if (ttl % 60 === 0) return `${ttl / 60}m`;
  return `${ttl}s`;
}

function userLabel(id: string, users: User[]): string {
  const u = users.find((x) => x.id === id);
  return u ? u.email : id.slice(0, 8);
}
