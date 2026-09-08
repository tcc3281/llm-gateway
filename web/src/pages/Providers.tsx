import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { ProviderKey } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";
import { useLanguage } from "../i18n";

const KNOWN = ["openai", "anthropic", "gemini"] as const;
const CUSTOM = "__custom__";

export default function Providers() {
  const [providers, setProviders] = useState<ProviderKey[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [setOpen, setSetOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  // "__custom__" selects a custom (compatible) provider; otherwise a built-in.
  const [providerSel, setProviderSel] = useState<string>(KNOWN[0]);
  const [customName, setCustomName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [protocol, setProtocol] = useState<"openai" | "anthropic">("openai");
  const [apiKey, setApiKey] = useState("");
  const [fallbackPriority, setFallbackPriority] = useState("");

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);

  const isCustom = providerSel === CUSTOM;

  const load = useCallback(async () => {
    setError(null);
    try {
      setProviders(await api.listProviders());
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load providers");
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openSet = (p?: ProviderKey) => {
    if (p) {
      setIsEditing(true);
      if (p.base_url) {
        setProviderSel(CUSTOM);
        setCustomName(p.provider);
        setBaseUrl(p.base_url);
        setProtocol(p.protocol === "anthropic" ? "anthropic" : "openai");
      } else {
        setProviderSel(p.provider);
        setCustomName("");
        setBaseUrl("");
        setProtocol("openai");
      }
      setApiKey("");
      setFallbackPriority(
        p.fallback_priority !== undefined && p.fallback_priority !== null
          ? String(p.fallback_priority)
          : "",
      );
    } else {
      setIsEditing(false);
      setProviderSel(KNOWN[0]);
      setCustomName("");
      setBaseUrl("");
      setProtocol("openai");
      setApiKey("");
      setFallbackPriority("");
    }
    setSetOpen(true);
  };

  const canSubmit = isEditing
    ? (isCustom ? baseUrl.trim() !== "" : true)
    : (isCustom
        ? customName.trim() !== "" && baseUrl.trim() !== "" && apiKey.trim() !== ""
        : apiKey.trim() !== "");

  const submitSet = async (e: FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    try {
      const fbPri =
        fallbackPriority.trim() !== "" ? parseInt(fallbackPriority, 10) : null;
      if (isCustom) {
        await api.setProvider({
          provider: customName.trim(),
          api_key: apiKey.trim(),
          base_url: baseUrl.trim(),
          protocol,
          fallback_priority: fbPri,
        });
      } else {
        await api.setProvider({
          provider: providerSel,
          api_key: apiKey.trim(),
          fallback_priority: fbPri,
        });
      }
      setSetOpen(false);
      setApiKey("");
      setFallbackPriority("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setError(null);
    try {
      await api.deleteProvider(deleteTarget);
      setDeleteTarget(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setBusy(false);
    }
  };

  const { t } = useLanguage();

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">{t("providers.title", "Providers")}</h1>
          <p className="page__sub">
            {t("providers.subtitle", "Upstream LLM credentials. Keys are stored encrypted and never returned to the browser.")}
          </p>
        </div>
        <button className="btn btn--primary" onClick={() => openSet()}>
          {t("providers.configure_btn", "+ Set provider key")}
        </button>
      </div>

      {error && <div className="error-banner">{error}</div>}

      {providers.length === 0 && !error && (
        <div className="card">
          <div className="empty">
            {t("common.no_data", "No provider keys configured. Use \"Set provider key\" to add one.")}
          </div>
        </div>
      )}

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
          gap: 16,
        }}
      >
        {providers.map((p) => (
          <div className="card" key={p.provider} style={{ margin: 0 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <span className="provider-logo">
                {p.provider.slice(0, 2).toUpperCase()}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="card__title" style={{ margin: 0 }}>
                  {p.provider}
                </div>
                <div className="muted" style={{ fontSize: 12 }}>
                  {p.key_preview || t("common.not_set", "key not set")} · {fmtDate(p.updated_at)}
                </div>
                {p.base_url && (
                  <div
                    className="muted"
                    style={{ fontSize: 11, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                    title={p.base_url}
                  >
                    {p.base_url}
                  </div>
                )}
              </div>
              <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 4 }}>
                {p.base_url ? (
                  <span className="badge badge--info">{p.protocol || "openai"}</span>
                ) : (
                  <span className="muted" style={{ fontSize: 11 }}>
                    builtin
                  </span>
                )}
                {p.fallback_priority !== undefined && p.fallback_priority !== null && (
                  <span className="badge badge--warning" style={{ fontSize: 10 }}>
                    fallback #{p.fallback_priority}
                  </span>
                )}
                {p.is_active ? (
                  <span className="badge badge--success">{t("common.active", "active")}</span>
                ) : (
                  <span className="badge badge--error">{t("common.disabled", "inactive")}</span>
                )}
              </div>
            </div>
            <div
              className="row"
              style={{ justifyContent: "flex-end", marginTop: 14 }}
            >
              <button className="btn btn--sm" onClick={() => openSet(p)}>
                {t("common.edit", "Edit / Rotate")}
              </button>
              <button
                className="btn btn--sm btn--danger"
                onClick={() => setDeleteTarget(p.provider)}
              >
                {t("common.delete", "Remove")}
              </button>
            </div>
          </div>
        ))}
      </div>

      {setOpen && (
        <Modal
          title={isEditing ? t("common.edit", "Edit Provider") : t("providers.configure_title", "Set Provider Key")}
          onClose={() => setSetOpen(false)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setSetOpen(false)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={submitSet}
                disabled={busy || !canSubmit}
              >
                {busy ? t("common.saving", "Saving…") : isEditing ? t("common.save", "Save changes") : t("common.save", "Save key")}
              </button>
            </>
          }
        >
          <div className="field">
            <label className="field__label">Provider type</label>
            <select
              className="select"
              value={providerSel}
              onChange={(e) => setProviderSel(e.target.value)}
              disabled={isEditing}
            >
              {KNOWN.map((k) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
              <option value={CUSTOM}>custom (compatible)…</option>
            </select>
          </div>

          {isCustom ? (
            <>
              <div className="field">
                <label className="field__label">
                  Provider name <span style={{ color: "#d33" }}>*</span>
                </label>
                <input
                  className="input"
                  type="text"
                  value={customName}
                  onChange={(e) => setCustomName(e.target.value)}
                  placeholder="e.g. openrouter, groq, vllm"
                  disabled={isEditing}
                />
                <p className="muted" style={{ fontSize: 11, margin: "4px 0 0" }}>
                  Client calls this provider as <code>{"name/model"}</code> — e.g.{" "}
                  <code>{customName || "openrouter"}/anthropic/claude-3.5-sonnet</code>.
                  Lowercase letters, digits, {"_"} and {"-"}.
                </p>
              </div>
              <div className="field">
                <label className="field__label">
                  Base URL <span style={{ color: "#d33" }}>*</span>
                </label>
                <input
                  className="input"
                  type="text"
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                  placeholder="https://api.example.com/v1"
                />
                <p className="muted" style={{ fontSize: 11, margin: "4px 0 0" }}>
                  The gateway appends <code>/chat/completions</code> (openai) or{" "}
                  <code>/v1/messages</code> (anthropic).{" "}
                </p>
              </div>
              <div className="field">
                <label className="field__label">Protocol</label>
                <select
                  className="select"
                  value={protocol}
                  onChange={(e) =>
                    setProtocol(e.target.value as "openai" | "anthropic")
                  }
                >
                  <option value="openai">openai</option>
                  <option value="anthropic">anthropic</option>
                </select>
              </div>
            </>
          ) : null}

          <div className="field">
            <label className="field__label">API key {isEditing && <span className="muted">(leave blank to keep current)</span>}</label>
            <input
              className="input"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={isEditing ? "••••••••••••••••" : "sk-..."}
              autoFocus
            />
          </div>
          <div className="field">
            <label className="field__label">Fallback priority (optional)</label>
            <input
              className="input"
              type="number"
              min="1"
              value={fallbackPriority}
              onChange={(e) => setFallbackPriority(e.target.value)}
              placeholder="e.g. 1, 2 (empty = not in fallback pool)"
            />
            <p className="muted" style={{ fontSize: 11, margin: "4px 0 0" }}>
              Lower numbers are tried first when a primary provider fails with 429 or 5xx.
            </p>
          </div>
          <p className="muted" style={{ fontSize: 12, marginBottom: 0 }}>
            Overwrites any existing key for{" "}
            {isCustom ? customName.trim() || "(custom provider)" : providerSel}.
          </p>
        </Modal>
      )}

      {deleteTarget && (
        <Modal
          title="Remove provider key"
          onClose={() => setDeleteTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setDeleteTarget(null)}>
                Cancel
              </button>
              <button className="btn btn--danger" onClick={confirmDelete} disabled={busy}>
                {busy ? "Removing…" : "Remove"}
              </button>
            </>
          }
        >
          <p>
            Remove the API key for <strong>{deleteTarget}</strong>? Requests to
            this provider will fail until a new key is set.
          </p>
        </Modal>
      )}
    </div>
  );
}
