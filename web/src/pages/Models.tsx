import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { ModelPricing } from "../api/types";
import { fmtDate, fmtNum } from "../lib/format";
import Modal from "../components/Modal";

const KNOWN = ["openai", "anthropic", "gemini"] as const;

interface ModelForm {
  provider: string;
  model_name: string;
  prompt_price_per_1m: string;
  completion_price_per_1m: string;
}

const empty: ModelForm = {
  provider: "openai",
  model_name: "",
  prompt_price_per_1m: "",
  completion_price_per_1m: "",
};

export default function Models() {
  const [models, setModels] = useState<ModelPricing[]>([]);
  const [providerOptions, setProviderOptions] = useState<string[]>([]);
  const [configuredProviders, setConfiguredProviders] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [discovering, setDiscovering] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<ModelForm>(empty);
  const [editTarget, setEditTarget] = useState<ModelPricing | null>(null);
  const [editForm, setEditForm] = useState<ModelForm>(empty);
  const [deleteTarget, setDeleteTarget] = useState<ModelPricing | null>(null);
  const [syncMsg, setSyncMsg] = useState<string | null>(null);

  // Filter & Search states
  const [search, setSearch] = useState("");
  const [providerFilter, setProviderFilter] = useState("configured"); // Default to configured (active in DB)
  const [priceFilter, setPriceFilter] = useState("all");
  const [sortBy, setSortBy] = useState("model_asc");

  const load = useCallback(async () => {
    setError(null);
    try {
      setModels(await api.listModels());
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load models");
    }
  }, []);

  useEffect(() => {
    load();
    // Load configured providers (built-in + custom compatible) so pricing can be
    // registered under a custom provider name for accurate cost calculation.
    api
      .listProviders()
      .then((ps) => {
        const active = ps.filter((p) => p.is_active).map((p) => p.provider);
        setConfiguredProviders(active);

        const seen = new Set<string>(KNOWN);
        const opts: string[] = [...KNOWN];
        for (const p of ps) {
          if (!seen.has(p.provider)) {
            seen.add(p.provider);
            opts.push(p.provider);
          }
        }
        setProviderOptions(opts);
      })
      .catch(() => {
        /* providers list is best-effort for the dropdown */
      });
  }, [load]);

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.model_name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.upsertModel({
        provider: form.provider,
        model_name: form.model_name.trim(),
        prompt_price_per_1m: parseFloat(form.prompt_price_per_1m) || 0,
        completion_price_per_1m: parseFloat(form.completion_price_per_1m) || 0,
      });
      setCreateOpen(false);
      setForm(empty);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setBusy(false);
    }
  };

  const submitEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editTarget || !editForm.model_name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.upsertModel({
        provider: editForm.provider,
        model_name: editForm.model_name.trim(),
        prompt_price_per_1m: parseFloat(editForm.prompt_price_per_1m) || 0,
        completion_price_per_1m:
          parseFloat(editForm.completion_price_per_1m) || 0,
      });
      setEditTarget(null);
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
      await api.deleteModel(deleteTarget.id);
      setDeleteTarget(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setBusy(false);
    }
  };

  const runSync = async () => {
    setSyncing(true);
    setSyncMsg(null);
    setError(null);
    try {
      const res = await api.syncModels();
      setSyncMsg(
        `${res.status} — synced ${res.synced_models} model(s) from the LiteLLM catalog.`,
      );
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sync failed");
    } finally {
      setSyncing(false);
    }
  };

  const runDiscover = async () => {
    setDiscovering(true);
    setSyncMsg(null);
    setError(null);
    try {
      const res = await api.discoverModels();
      const errKeys = Object.keys(res.errors || {});
      setSyncMsg(
        errKeys.length === 0
          ? `${res.status} — discovered ${res.synced_models} model(s) from provider /models APIs into the routing catalog.`
          : `${res.status} — discovered ${res.synced_models} model(s), but ${errKeys.length} provider(s) failed: ${errKeys.join(", ")}.`,
      );
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Discovery failed");
    } finally {
      setDiscovering(false);
    }
  };

  // Client side filtering
  const filteredModels = models.filter((m) => {
    // 1. Search filter
    if (search.trim() !== "" && !m.model_name.toLowerCase().includes(search.toLowerCase())) {
      return false;
    }

    // 2. Provider filter
    if (providerFilter === "configured") {
      const activeSet = new Set(configuredProviders);
      if (!activeSet.has(m.provider)) {
        return false;
      }
    } else if (providerFilter !== "" && m.provider !== providerFilter) {
      return false;
    }

    // 3. Price filter
    if (priceFilter === "priced") {
      if (m.prompt_price_per_1m === 0 && m.completion_price_per_1m === 0) {
        return false;
      }
    } else if (priceFilter === "free") {
      if (m.prompt_price_per_1m > 0 || m.completion_price_per_1m > 0) {
        return false;
      }
    }

    return true;
  });

  // Client side sorting
  const sortedModels = [...filteredModels].sort((a, b) => {
    switch (sortBy) {
      case "model_asc":
        return a.model_name.localeCompare(b.model_name);
      case "model_desc":
        return b.model_name.localeCompare(a.model_name);
      case "provider_asc":
        return a.provider.localeCompare(b.provider);
      case "provider_desc":
        return b.provider.localeCompare(a.provider);
      case "prompt_asc":
        return a.prompt_price_per_1m - b.prompt_price_per_1m;
      case "prompt_desc":
        return b.prompt_price_per_1m - a.prompt_price_per_1m;
      case "updated_desc":
        return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
      default:
        return 0;
    }
  });

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">Models &amp; Pricing</h1>
          <p className="page__sub">
            Showing {filteredModels.length} of {models.length} model(s). Prices are USD per 1M tokens.
          </p>
        </div>
        <div className="row">
          <button className="btn" onClick={runDiscover} disabled={discovering}>
            {discovering ? "Discovering…" : "🔎 Discover models"}
          </button>
          <button className="btn" onClick={runSync} disabled={syncing}>
            {syncing ? "Syncing…" : "↻ Sync from catalog"}
          </button>
          <button className="btn btn--primary" onClick={() => setCreateOpen(true)}>
            + Add model
          </button>
        </div>
      </div>

      <div className="filter-bar" style={{ marginBottom: 15 }}>
        <input
          className="input"
          style={{ width: 220 }}
          placeholder="Search model name…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          className="select"
          style={{ width: 200 }}
          value={providerFilter}
          onChange={(e) => setProviderFilter(e.target.value)}
        >
          <option value="">All Providers</option>
          <option value="configured">Only Configured Keys</option>
          {providerOptions.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
        <select
          className="select"
          style={{ width: 150 }}
          value={priceFilter}
          onChange={(e) => setPriceFilter(e.target.value)}
        >
          <option value="all">Any Price</option>
          <option value="priced">Priced (&gt; $0)</option>
          <option value="free">No Price ($0)</option>
        </select>
        <select
          className="select"
          style={{ width: 180 }}
          value={sortBy}
          onChange={(e) => setSortBy(e.target.value)}
        >
          <option value="model_asc">Sort by Name (A-Z)</option>
          <option value="model_desc">Sort by Name (Z-A)</option>
          <option value="provider_asc">Sort by Provider (A-Z)</option>
          <option value="provider_desc">Sort by Provider (Z-A)</option>
          <option value="prompt_asc">Prompt Price (Low-High)</option>
          <option value="prompt_desc">Prompt Price (High-Low)</option>
          <option value="updated_desc">Recently Updated</option>
        </select>
        {(search !== "" || providerFilter !== "configured" || priceFilter !== "all" || sortBy !== "model_asc") && (
          <button
            className="btn btn--ghost btn--sm"
            style={{ padding: "0 8px", minHeight: 28 }}
            onClick={() => {
              setSearch("");
              setProviderFilter("configured");
              setPriceFilter("all");
              setSortBy("model_asc");
            }}
          >
            Clear filters
          </button>
        )}
      </div>

      {syncMsg && <div className="error-banner">{syncMsg}</div>}
      {error && <div className="error-banner">{error}</div>}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Provider</th>
              <th>Model</th>
              <th className="num">Prompt /1M</th>
              <th className="num">Completion /1M</th>
              <th>Updated</th>
              <th style={{ textAlign: "right" }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {sortedModels.map((m) => (
              <tr key={m.id}>
                <td>
                  <span className="badge badge--info">{m.provider}</span>
                </td>
                <td className="mono" style={{ fontSize: 12 }}>
                  {m.model_name}
                </td>
                <td className="num">${fmtNum(m.prompt_price_per_1m)}</td>
                <td className="num">${fmtNum(m.completion_price_per_1m)}</td>
                <td className="muted">{fmtDate(m.updated_at)}</td>
                <td>
                  <div className="table__actions">
                    <button
                      className="btn btn--sm"
                      onClick={() => {
                        setEditTarget(m);
                        setEditForm({
                          provider: m.provider,
                          model_name: m.model_name,
                          prompt_price_per_1m: String(m.prompt_price_per_1m),
                          completion_price_per_1m: String(
                            m.completion_price_per_1m,
                          ),
                        });
                      }}
                    >
                      Edit
                    </button>
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={() => setDeleteTarget(m)}
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {!sortedModels.length && (
              <tr>
                <td colSpan={6}>
                  <div className="empty">No models match the filters</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {createOpen && (
        <Modal
          title="Add model"
          onClose={() => setCreateOpen(false)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setCreateOpen(false)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={submitCreate}
                disabled={busy || !form.model_name.trim()}
              >
                {busy ? "Saving…" : "Add"}
              </button>
            </>
          }
        >
          <ModelFields
            providerOptions={providerOptions}
            form={form}
            setForm={setForm}
          />
        </Modal>
      )}

      {editTarget && (
        <Modal
          title="Edit model pricing"
          onClose={() => setEditTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setEditTarget(null)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={submitEdit}
                disabled={busy || !editForm.model_name.trim()}
              >
                {busy ? "Saving…" : "Save"}
              </button>
            </>
          }
        >
          <ModelFields
            providerOptions={providerOptions}
            form={editForm}
            setForm={setEditForm}
          />
        </Modal>
      )}

      {deleteTarget && (
        <Modal
          title="Delete model"
          onClose={() => setDeleteTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setDeleteTarget(null)}>
                Cancel
              </button>
              <button className="btn btn--danger" onClick={confirmDelete} disabled={busy}>
                {busy ? "Deleting…" : "Delete"}
              </button>
            </>
          }
        >
          <p>
            Delete <strong>{deleteTarget.model_name}</strong>? Requests to this
            model will fall back to provider master-key pricing.
          </p>
        </Modal>
      )}
    </div>
  );
}

function ModelFields({
  providerOptions,
  form,
  setForm,
}: {
  providerOptions: string[];
  form: ModelForm;
  setForm: (f: ModelForm) => void;
}) {
  return (
    <>
      <div className="field">
        <label className="field__label">Provider</label>
        <select
          className="select"
          value={form.provider}
          onChange={(e) => setForm({ ...form, provider: e.target.value })}
        >
          {providerOptions.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label className="field__label">Model name</label>
        <input
          className="input"
          value={form.model_name}
          onChange={(e) => setForm({ ...form, model_name: e.target.value })}
          placeholder="gpt-4o"
        />
      </div>
      <div className="row">
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">Prompt price /1M (USD)</label>
          <input
            className="input"
            type="number"
            min="0"
            step="0.000001"
            value={form.prompt_price_per_1m}
            onChange={(e) =>
              setForm({ ...form, prompt_price_per_1m: e.target.value })
            }
          />
        </div>
        <div className="field" style={{ flex: 1 }}>
          <label className="field__label">Completion price /1M (USD)</label>
          <input
            className="input"
            type="number"
            min="0"
            step="0.000001"
            value={form.completion_price_per_1m}
            onChange={(e) =>
              setForm({ ...form, completion_price_per_1m: e.target.value })
            }
          />
        </div>
      </div>
    </>
  );
}
