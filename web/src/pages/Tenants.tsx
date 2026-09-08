import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { ModelPricing, Tenant, TenantCreditTransaction, TopUpCreditRequest } from "../api/types";
import { fmtDate, fmtMoney } from "../lib/format";
import Modal from "../components/Modal";
import { useToast } from "../components/Toast";
import { useLanguage } from "../i18n";

interface TenantForm {
  name: string;
  type: "internal" | "external";
  billing_mode: "prepaid" | "postpaid";
  budget_limit_monthly: number;
  allowed_models: string[];
  is_active: boolean;
}

const emptyForm: TenantForm = {
  name: "",
  type: "external",
  billing_mode: "prepaid",
  budget_limit_monthly: 0,
  allowed_models: [],
  is_active: true,
};

export default function Tenants() {
  const { t } = useLanguage();
  const toast = useToast();
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Search & Filters
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("all");
  const [billingFilter, setBillingFilter] = useState<string>("all");

  // Create Modal
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<TenantForm>(emptyForm);

  // Edit Modal
  const [editTarget, setEditTarget] = useState<Tenant | null>(null);
  const [editForm, setEditForm] = useState<TenantForm>(emptyForm);

  // Top Up Modal
  const [topUpTarget, setTopUpTarget] = useState<Tenant | null>(null);
  const [topUpForm, setTopUpForm] = useState<TopUpCreditRequest>({
    amount: 100,
    contract_ref: "",
    notes: "",
  });

  // Ledger / Transaction History Modal
  const [ledgerTarget, setLedgerTarget] = useState<Tenant | null>(null);
  const [ledgerTransactions, setLedgerTransactions] = useState<TenantCreditTransaction[]>([]);
  const [ledgerLoading, setLedgerLoading] = useState(false);

  // Delete Modal
  const [deleteTarget, setDeleteTarget] = useState<Tenant | null>(null);

  const loadTenants = useCallback(async () => {
    setError(null);
    try {
      const data = await api.listTenants();
      setTenants(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load tenants");
    }
  }, []);

  const loadModels = useCallback(async () => {
    try {
      const models = await api.listModels();
      const uniqueNames = Array.from(new Set(models.map((m: ModelPricing) => m.model_name)));
      setAvailableModels(uniqueNames);
    } catch {
      // Non-critical if models fail to load
    }
  }, []);

  useEffect(() => {
    loadTenants();
    loadModels();
  }, [loadTenants, loadModels]);

  // Open Ledger Modal & load transactions
  const openLedger = async (tenant: Tenant) => {
    setLedgerTarget(tenant);
    setLedgerLoading(true);
    try {
      const res = await api.listTenantTransactions(tenant.id, { limit: 100, offset: 0 });
      setLedgerTransactions(res.data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load transactions");
    } finally {
      setLedgerLoading(false);
    }
  };

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!createForm.name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.createTenant({
        name: createForm.name.trim(),
        type: createForm.type,
        billing_mode: createForm.billing_mode,
        budget_limit_monthly: Number(createForm.budget_limit_monthly) || 0,
        allowed_models: createForm.allowed_models,
        is_active: createForm.is_active,
      });
      toast.success(`Tenant "${createForm.name}" created`, "[200 OK]");
      setCreateOpen(false);
      setCreateForm(emptyForm);
      await loadTenants();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create tenant";
      setError(msg);
      toast.error(msg, "[CREATE FAILED]");
    } finally {
      setBusy(false);
    }
  };

  const startEdit = (tenant: Tenant) => {
    setEditTarget(tenant);
    setEditForm({
      name: tenant.name,
      type: tenant.type,
      billing_mode: tenant.billing_mode,
      budget_limit_monthly: tenant.budget_limit_monthly,
      allowed_models: tenant.allowed_models || [],
      is_active: tenant.is_active,
    });
  };

  const submitEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editTarget || !editForm.name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.updateTenant(editTarget.id, {
        name: editForm.name.trim(),
        type: editForm.type,
        billing_mode: editForm.billing_mode,
        budget_limit_monthly: Number(editForm.budget_limit_monthly) || 0,
        allowed_models: editForm.allowed_models,
        is_active: editForm.is_active,
      });
      toast.success(`Tenant "${editForm.name}" updated`, "[200 OK]");
      setEditTarget(null);
      await loadTenants();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to update tenant";
      setError(msg);
      toast.error(msg, "[UPDATE FAILED]");
    } finally {
      setBusy(false);
    }
  };

  const submitTopUp = async (e: FormEvent) => {
    e.preventDefault();
    if (!topUpTarget || topUpForm.amount <= 0) return;
    setBusy(true);
    setError(null);
    try {
      await api.topUpTenantCredit(topUpTarget.id, {
        amount: Number(topUpForm.amount),
        contract_ref: (topUpForm.contract_ref || "").trim() || undefined,
        notes: (topUpForm.notes || "").trim() || undefined,
      });
      toast.success(
        `Deposited $${topUpForm.amount.toFixed(2)} to ${topUpTarget.name}`,
        "[CREDIT UPDATED]"
      );
      setTopUpTarget(null);
      setTopUpForm({ amount: 100, contract_ref: "", notes: "" });
      await loadTenants();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to deposit credit";
      setError(msg);
      toast.error(msg, "[TOP UP FAILED]");
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setError(null);
    try {
      await api.deleteTenant(deleteTarget.id);
      toast.info(`Tenant "${deleteTarget.name}" deleted`, "[200 OK]");
      setDeleteTarget(null);
      await loadTenants();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to delete tenant";
      setError(msg);
      toast.error(msg, "[DELETE FAILED]");
    } finally {
      setBusy(false);
    }
  };

  const toggleModelSelection = (
    modelName: string,
    currentList: string[],
    updater: (models: string[]) => void
  ) => {
    if (currentList.includes(modelName)) {
      updater(currentList.filter((m) => m !== modelName));
    } else {
      updater([...currentList, modelName]);
    }
  };

  // Filtered tenants
  const filteredTenants = tenants.filter((tItem) => {
    if (search.trim()) {
      const q = search.toLowerCase();
      const matchName = tItem.name.toLowerCase().includes(q);
      const matchModels = tItem.allowed_models?.some((m) => m.toLowerCase().includes(q));
      if (!matchName && !matchModels) return false;
    }
    if (typeFilter !== "all" && tItem.type !== typeFilter) return false;
    if (billingFilter !== "all" && tItem.billing_mode !== billingFilter) return false;
    return true;
  });

  // Aggregated Stats
  const totalInternal = tenants.filter((tItem) => tItem.type === "internal").length;
  const totalExternal = tenants.filter((tItem) => tItem.type === "external").length;
  const totalPrepaidBalance = tenants.reduce((acc, tItem) => acc + (tItem.billing_mode === "prepaid" ? tItem.credit_balance : 0), 0);
  const activeCount = tenants.filter((tItem) => tItem.is_active).length;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      {/* Metric Cards Banner */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14 }}>
        <div className="card" style={{ padding: 16 }}>
          <div className="muted" style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase" }}>
            {t("tenants.kpi_total", "Total Tenants")}
          </div>
          <div style={{ fontSize: 24, fontWeight: 700, marginTop: 4 }}>
            {tenants.length}
          </div>
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            {totalInternal} {t("tenants.kpi_internal", "Internal")} &bull; {totalExternal} {t("tenants.kpi_external", "External B2B")}
          </div>
        </div>

        <div className="card" style={{ padding: 16 }}>
          <div className="muted" style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase" }}>
            {t("tenants.kpi_balance", "Prepaid Credit Balance")}
          </div>
          <div style={{ fontSize: 24, fontWeight: 700, marginTop: 4, color: totalPrepaidBalance > 0 ? "var(--success)" : "var(--text)" }}>
            {fmtMoney(totalPrepaidBalance, 2)}
          </div>
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            {t("tenants.kpi_escrow_desc", "Held in escrow across prepaid clients")}
          </div>
        </div>

        <div className="card" style={{ padding: 16 }}>
          <div className="muted" style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase" }}>
            {t("tenants.kpi_active", "Active Tenants")}
          </div>
          <div style={{ fontSize: 24, fontWeight: 700, marginTop: 4, color: "var(--primary)" }}>
            {activeCount} / {tenants.length}
          </div>
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            {t("tenants.kpi_active_desc", "Status enforcement active")}
          </div>
        </div>
      </div>

      {error && (
        <div
          style={{
            padding: "10px 16px",
            background: "rgba(239, 68, 68, 0.1)",
            border: "1px solid var(--danger)",
            borderRadius: 6,
            color: "var(--danger)",
            fontSize: 13,
          }}
        >
          {error}
        </div>
      )}

      {/* Action and Filter Bar */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          flexWrap: "wrap",
          gap: 12,
        }}
      >
        <div style={{ display: "flex", gap: 10, flexWrap: "wrap", alignItems: "center" }}>
          <input
            type="text"
            placeholder={t("tenants.filter_search", "Search tenant or model...")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              padding: "7px 12px",
              borderRadius: 6,
              border: "1px solid var(--border)",
              background: "var(--surface)",
              color: "var(--text)",
              fontSize: 13,
              minWidth: 220,
            }}
          />

          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            style={{
              padding: "7px 10px",
              borderRadius: 6,
              border: "1px solid var(--border)",
              background: "var(--surface)",
              color: "var(--text)",
              fontSize: 13,
            }}
          >
            <option value="all">{t("tenants.filter_all_types", "All Types")}</option>
            <option value="internal">{t("tenants.filter_internal", "Internal Teams")}</option>
            <option value="external">{t("tenants.filter_external", "External Clients")}</option>
          </select>

          <select
            value={billingFilter}
            onChange={(e) => setBillingFilter(e.target.value)}
            style={{
              padding: "7px 10px",
              borderRadius: 6,
              border: "1px solid var(--border)",
              background: "var(--surface)",
              color: "var(--text)",
              fontSize: 13,
            }}
          >
            <option value="all">{t("tenants.filter_all_billing", "All Billing Modes")}</option>
            <option value="prepaid">{t("tenants.filter_prepaid", "Prepaid (Credit Ledger)")}</option>
            <option value="postpaid">{t("tenants.filter_postpaid", "Postpaid (Monthly Invoiced)")}</option>
          </select>
        </div>

        <button
          className="btn btn--primary"
          onClick={() => {
            setCreateForm(emptyForm);
            setCreateOpen(true);
          }}
          style={{ display: "flex", alignItems: "center", gap: 6 }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: 18 }}>add_business</span>
          <span>{t("tenants.create_btn", "+ Add Tenant")}</span>
        </button>
      </div>

      {/* Tenants Table */}
      <div className="card" style={{ padding: 0, overflow: "hidden" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", textAlign: "left", fontSize: 13 }}>
          <thead>
            <tr style={{ background: "var(--surface-high)", borderBottom: "1px solid var(--border)" }}>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_name", "Tenant")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_type", "Type")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_billing", "Billing Mode")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_balance", "Credit Balance")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_budget", "Monthly Ceiling")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_models", "Model Boundary (Layer 1)")}</th>
              <th style={{ padding: "12px 16px" }}>{t("tenants.col_status", "Status")}</th>
              <th style={{ padding: "12px 16px", textAlign: "right" }}>{t("tenants.col_actions", "Actions")}</th>
            </tr>
          </thead>
          <tbody>
            {filteredTenants.length === 0 ? (
              <tr>
                <td colSpan={8} style={{ padding: 32, textAlign: "center" }} className="muted">
                  {t("tenants.empty", "No tenants found. Click \"Add Tenant\" to establish an internal department or external B2B client.")}
                </td>
              </tr>
            ) : (
              filteredTenants.map((tItem) => {
                const isPrepaid = tItem.billing_mode === "prepaid";
                const isDepleted = isPrepaid && tItem.credit_balance <= 0;
                return (
                  <tr
                    key={tItem.id}
                    style={{
                      borderBottom: "1px solid var(--border)",
                      opacity: tItem.is_active ? 1 : 0.6,
                    }}
                  >
                    <td style={{ padding: "12px 16px" }}>
                      <div style={{ fontWeight: 600, color: "var(--text)" }}>{tItem.name}</div>
                      <div className="muted mono" style={{ fontSize: 11 }}>{tItem.id.slice(0, 8)}...</div>
                    </td>

                    <td style={{ padding: "12px 16px" }}>
                      <span
                        className={`terminal-badge ${tItem.type === "internal" ? "terminal-badge--internal" : "terminal-badge--external"}`}
                      >
                        [{tItem.type.toUpperCase()}]
                      </span>
                    </td>

                    <td style={{ padding: "12px 16px" }}>
                      <span
                        style={{
                          display: "inline-block",
                          padding: "2px 8px",
                          borderRadius: 4,
                          fontSize: 11,
                          fontWeight: 500,
                          background: "var(--surface-high)",
                          color: "var(--text-muted)",
                        }}
                      >
                        {isPrepaid ? t("tenants.prepaid", "Prepaid") : t("tenants.postpaid", "Postpaid")}
                      </span>
                    </td>

                    <td style={{ padding: "12px 16px" }}>
                      {isPrepaid ? (
                        <div style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                          <span
                            className="mono"
                            style={{
                              fontWeight: 700,
                              color: isDepleted ? "var(--danger)" : "var(--success)",
                            }}
                          >
                            {fmtMoney(tItem.credit_balance, 2)}
                          </span>
                          {isDepleted ? (
                            <span className="terminal-badge terminal-badge--root glow-amber">
                              {t("tenants.depleted", "[402 DEPLETED]")}
                            </span>
                          ) : tItem.credit_balance < 10 ? (
                            <span className="terminal-badge" style={{ color: "var(--amber)", borderColor: "rgba(245, 158, 11, 0.4)" }}>
                              {t("tenants.low_credits", "[LOW CREDITS]")}
                            </span>
                          ) : null}
                        </div>
                      ) : (
                        <span className="muted mono">— ({t("tenants.postpaid", "Postpaid")})</span>
                      )}
                    </td>

                    <td style={{ padding: "12px 16px" }}>
                      <span className="mono">
                        {tItem.budget_limit_monthly > 0 ? fmtMoney(tItem.budget_limit_monthly, 0) + " / mo" : t("common.unlimited", "Unlimited")}
                      </span>
                    </td>

                    <td style={{ padding: "12px 16px", maxWidth: 260 }}>
                      {!tItem.allowed_models || tItem.allowed_models.length === 0 ? (
                        <span className="muted" style={{ fontSize: 12 }}>
                          {t("tenants.all_models_allowed", "All Models Allowed (Unrestricted)")}
                        </span>
                      ) : (
                        <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
                          {tItem.allowed_models.map((m) => (
                            <span
                              key={m}
                              className="mono"
                              style={{
                                fontSize: 11,
                                padding: "2px 6px",
                                borderRadius: 4,
                                background: "rgba(34, 211, 238, 0.1)",
                                color: "var(--primary)",
                                border: "1px solid rgba(34, 211, 238, 0.2)",
                              }}
                            >
                              {m}
                            </span>
                          ))}
                        </div>
                      )}
                    </td>

                    <td style={{ padding: "12px 16px" }}>
                      <span
                        style={{
                          display: "inline-block",
                          width: 8,
                          height: 8,
                          borderRadius: "50%",
                          background: tItem.is_active ? "var(--success)" : "var(--text-muted)",
                          marginRight: 6,
                        }}
                      />
                      <span style={{ fontSize: 12, color: tItem.is_active ? "var(--text)" : "var(--text-muted)" }}>
                        {tItem.is_active ? t("common.active", "Active") : t("common.disabled", "Disabled")}
                      </span>
                    </td>

                    <td style={{ padding: "12px 16px", textAlign: "right" }}>
                      <div style={{ display: "inline-flex", gap: 6 }}>
                        {isPrepaid && (
                          <button
                            className="btn btn--sm"
                            onClick={() => {
                              setTopUpTarget(tItem);
                              setTopUpForm({ amount: 100, contract_ref: "", notes: "" });
                            }}
                            title="Top-up Contract Credit"
                            style={{
                              background: "rgba(16, 185, 129, 0.15)",
                              color: "var(--success)",
                              borderColor: "transparent",
                            }}
                          >
                            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>payments</span>
                            <span>{t("tenants.topup_btn", "Top Up")}</span>
                          </button>
                        )}

                        <button
                          className="btn btn--sm"
                          onClick={() => openLedger(tItem)}
                          title="View Credit Ledger & Transactions"
                        >
                          <span className="material-symbols-outlined" style={{ fontSize: 16 }}>receipt_long</span>
                          <span>{t("tenants.ledger_btn", "Ledger")}</span>
                        </button>

                        <button
                          className="btn btn--sm"
                          onClick={() => startEdit(tItem)}
                          title="Edit Tenant"
                        >
                          <span className="material-symbols-outlined" style={{ fontSize: 16 }}>edit</span>
                        </button>

                        <button
                          className="btn btn--sm btn--danger"
                          onClick={() => setDeleteTarget(tItem)}
                          title="Delete Tenant"
                        >
                          <span className="material-symbols-outlined" style={{ fontSize: 16 }}>delete</span>
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* CREATE TENANT MODAL */}
      {createOpen && (
        <Modal title={t("tenants.create_modal_title", "Create New Tenant")} onClose={() => setCreateOpen(false)}>
          <form onSubmit={submitCreate} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_name", "Tenant Organization / Department Name")} *
              </label>
              <input
                type="text"
                required
                placeholder="e.g. Enterprise Client Acme Corp or AI Department 1"
                value={createForm.name}
                onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
              <div>
                <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                  {t("tenants.col_type", "Tenant Classification")} *
                </label>
                <select
                  value={createForm.type}
                  onChange={(e) => setCreateForm({ ...createForm, type: e.target.value as "internal" | "external" })}
                  style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
                >
                  <option value="external">{t("tenants.external", "External SaaS Client (Isolated Tenant)")}</option>
                  <option value="internal">{t("tenants.internal", "Internal Department (Engineering / Product)")}</option>
                </select>
              </div>

              <div>
                <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                  {t("tenants.col_billing", "Billing Mode")} *
                </label>
                <select
                  value={createForm.billing_mode}
                  onChange={(e) => setCreateForm({ ...createForm, billing_mode: e.target.value as "prepaid" | "postpaid" })}
                  style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
                >
                  <option value="prepaid">{t("tenants.prepaid", "Prepaid (Cut off when credit balance ≤ 0)")}</option>
                  <option value="postpaid">{t("tenants.postpaid", "Postpaid (Invoiced Monthly Ceiling)")}</option>
                </select>
              </div>
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_budget", "Monthly Budget Ceiling ($ USD)")}
              </label>
              <input
                type="number"
                min="0"
                step="0.01"
                placeholder="0 = unlimited"
                value={createForm.budget_limit_monthly || ""}
                onChange={(e) => setCreateForm({ ...createForm, budget_limit_monthly: parseFloat(e.target.value) || 0 })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
              <span className="muted" style={{ fontSize: 11 }}>0 = {t("common.unlimited", "no monthly spend limit")}</span>
            </div>

            {/* Layer 1 Model Boundary Multi-Select */}
            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_models", "System Boundary: Allowed Models (Layer 1)")}
              </label>
              <div className="muted" style={{ fontSize: 11, marginBottom: 8 }}>
                Contract-level restriction. Tenant IAM policies CANNOT grant models outside this set. Leave empty to allow all models.
              </div>

              <div
                style={{
                  maxHeight: 140,
                  overflowY: "auto",
                  border: "1px solid var(--border)",
                  borderRadius: 6,
                  padding: 8,
                  background: "var(--surface-high)",
                  display: "flex",
                  flexDirection: "column",
                  gap: 6,
                }}
              >
                {availableModels.length === 0 ? (
                  <span className="muted" style={{ fontSize: 12 }}>{t("common.no_data", "No models registered in catalog")}</span>
                ) : (
                  availableModels.map((m) => {
                    const selected = createForm.allowed_models.includes(m);
                    return (
                      <label key={m} style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 12 }}>
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={() =>
                            toggleModelSelection(m, createForm.allowed_models, (models) =>
                              setCreateForm({ ...createForm, allowed_models: models })
                            )
                          }
                        />
                        <span className="mono">{m}</span>
                      </label>
                    );
                  })
                )}
              </div>
            </div>

            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 8 }}>
              <button type="button" className="btn" onClick={() => setCreateOpen(false)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button type="submit" className="btn btn--primary" disabled={busy}>
                {busy ? t("common.creating", "Creating...") : t("tenants.create_btn", "Create Tenant")}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* EDIT TENANT MODAL */}
      {editTarget && (
        <Modal title={`${t("tenants.edit_modal_title", "Edit Tenant")}: ${editTarget.name}`} onClose={() => setEditTarget(null)}>
          <form onSubmit={submitEdit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_name", "Tenant Organization / Department Name")} *
              </label>
              <input
                type="text"
                required
                value={editForm.name}
                onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
              <div>
                <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                  {t("tenants.col_type", "Tenant Classification")} *
                </label>
                <select
                  value={editForm.type}
                  onChange={(e) => setEditForm({ ...editForm, type: e.target.value as "internal" | "external" })}
                  style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
                >
                  <option value="external">{t("tenants.external", "External SaaS Client")}</option>
                  <option value="internal">{t("tenants.internal", "Internal Department")}</option>
                </select>
              </div>

              <div>
                <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                  {t("tenants.col_billing", "Billing Mode")} *
                </label>
                <select
                  value={editForm.billing_mode}
                  onChange={(e) => setEditForm({ ...editForm, billing_mode: e.target.value as "prepaid" | "postpaid" })}
                  style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
                >
                  <option value="prepaid">{t("tenants.prepaid", "Prepaid (Credit Balance Check)")}</option>
                  <option value="postpaid">{t("tenants.postpaid", "Postpaid (Monthly Invoiced)")}</option>
                </select>
              </div>
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_budget", "Monthly Budget Ceiling ($ USD)")}
              </label>
              <input
                type="number"
                min="0"
                step="0.01"
                value={editForm.budget_limit_monthly || ""}
                onChange={(e) => setEditForm({ ...editForm, budget_limit_monthly: parseFloat(e.target.value) || 0 })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.col_models", "System Boundary: Allowed Models (Layer 1)")}
              </label>
              <div
                style={{
                  maxHeight: 140,
                  overflowY: "auto",
                  border: "1px solid var(--border)",
                  borderRadius: 6,
                  padding: 8,
                  background: "var(--surface-high)",
                  display: "flex",
                  flexDirection: "column",
                  gap: 6,
                }}
              >
                {availableModels.map((m) => {
                  const selected = editForm.allowed_models.includes(m);
                  return (
                    <label key={m} style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 12 }}>
                      <input
                        type="checkbox"
                        checked={selected}
                        onChange={() =>
                          toggleModelSelection(m, editForm.allowed_models, (models) =>
                            setEditForm({ ...editForm, allowed_models: models })
                          )
                        }
                      />
                      <span className="mono">{m}</span>
                    </label>
                  );
                })}
              </div>
            </div>

            <div>
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13 }}>
                <input
                  type="checkbox"
                  checked={editForm.is_active}
                  onChange={(e) => setEditForm({ ...editForm, is_active: e.target.checked })}
                />
                <span style={{ fontWeight: 600 }}>{t("common.active", "Active Status")}</span>
              </label>
            </div>

            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 8 }}>
              <button type="button" className="btn" onClick={() => setEditTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button type="submit" className="btn btn--primary" disabled={busy}>
                {busy ? t("common.saving", "Saving...") : t("common.save", "Save Changes")}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* TOP-UP CREDIT MODAL */}
      {topUpTarget && (
        <Modal title={`${t("tenants.topup_modal_title", "Top-up Contract Credit")}: ${topUpTarget.name}`} onClose={() => setTopUpTarget(null)}>
          <form onSubmit={submitTopUp} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div
              style={{
                background: "var(--surface-high)",
                padding: 12,
                borderRadius: 6,
                border: "1px solid var(--border)",
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
              }}
            >
              <div>
                <div className="muted" style={{ fontSize: 11 }}>{t("tenants.col_balance", "Current Credit Balance")}</div>
                <div className="mono" style={{ fontSize: 18, fontWeight: 700, color: "var(--success)" }}>
                  {fmtMoney(topUpTarget.credit_balance, 2)}
                </div>
              </div>
              <div style={{ textAlign: "right" }}>
                <div className="muted" style={{ fontSize: 11 }}>{t("tenants.col_billing", "Billing Mode")}</div>
                <div style={{ fontWeight: 600, fontSize: 13, textTransform: "capitalize" }}>
                  {topUpTarget.billing_mode}
                </div>
              </div>
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                {t("tenants.kpi_balance", "Credit Amount to Add ($ USD)")} *
              </label>
              <input
                type="number"
                min="0.01"
                step="0.01"
                required
                value={topUpForm.amount || ""}
                onChange={(e) => setTopUpForm({ ...topUpForm, amount: parseFloat(e.target.value) || 0 })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                Contract Reference / PO Number
              </label>
              <input
                type="text"
                placeholder="e.g. HĐ-2026/AI-01, WIRE-TX-88219"
                value={topUpForm.contract_ref || ""}
                onChange={(e) => setTopUpForm({ ...topUpForm, contract_ref: e.target.value })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)" }}
              />
            </div>

            <div>
              <label style={{ display: "block", fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                Notes / Audit Memo
              </label>
              <textarea
                rows={2}
                placeholder="e.g. Approved by CFO for Q2 AI consumption via bank wire"
                value={topUpForm.notes || ""}
                onChange={(e) => setTopUpForm({ ...topUpForm, notes: e.target.value })}
                style={{ width: "100%", padding: 8, borderRadius: 6, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", resize: "vertical" }}
              />
            </div>

            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 8 }}>
              <button type="button" className="btn" onClick={() => setTopUpTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button type="submit" className="btn btn--primary" disabled={busy || topUpForm.amount <= 0}>
                {busy ? t("common.saving", "Processing...") : `${t("common.confirm", "Confirm Deposit")} ($${topUpForm.amount.toFixed(2)})`}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* TRANSACTION HISTORY LEDGER MODAL */}
      {ledgerTarget && (
        <Modal title={`${t("tenants.ledger_modal_title", "Credit Ledger")}: ${ledgerTarget.name}`} onClose={() => setLedgerTarget(null)}>
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div className="muted" style={{ fontSize: 12 }}>
                Current Escrow Balance:{" "}
                <strong className="mono" style={{ color: "var(--success)" }}>
                  {fmtMoney(ledgerTarget.credit_balance, 2)}
                </strong>
              </div>
              <button
                className="btn btn--sm"
                onClick={() => openLedger(ledgerTarget)}
                disabled={ledgerLoading}
              >
                <span className="material-symbols-outlined" style={{ fontSize: 14 }}>refresh</span>
                <span>{t("common.refresh", "Refresh")}</span>
              </button>
            </div>

            <div style={{ maxHeight: 360, overflowY: "auto", border: "1px solid var(--border)", borderRadius: 6 }}>
              <table style={{ width: "100%", borderCollapse: "collapse", textAlign: "left", fontSize: 12 }}>
                <thead>
                  <tr style={{ background: "var(--surface-high)", borderBottom: "1px solid var(--border)" }}>
                    <th style={{ padding: "8px 12px" }}>{t("common.created_at", "Date")}</th>
                    <th style={{ padding: "8px 12px" }}>{t("common.role", "Type")}</th>
                    <th style={{ padding: "8px 12px" }}>Contract Ref</th>
                    <th style={{ padding: "8px 12px", textAlign: "right" }}>Amount</th>
                    <th style={{ padding: "8px 12px", textAlign: "right" }}>Balance After</th>
                    <th style={{ padding: "8px 12px" }}>Notes</th>
                  </tr>
                </thead>
                <tbody>
                  {ledgerLoading ? (
                    <tr>
                      <td colSpan={6} style={{ padding: 24, textAlign: "center" }} className="muted">
                        {t("common.loading", "Loading ledger transactions...")}
                      </td>
                    </tr>
                  ) : ledgerTransactions.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ padding: 24, textAlign: "center" }} className="muted">
                        {t("common.no_data", "No transactions recorded yet for this tenant.")}
                      </td>
                    </tr>
                  ) : (
                    ledgerTransactions.map((tx) => {
                      const isPositive = tx.amount > 0;
                      return (
                        <tr key={tx.id} style={{ borderBottom: "1px solid var(--border)" }}>
                          <td style={{ padding: "8px 12px" }} className="mono muted">
                            {fmtDate(tx.created_at)}
                          </td>
                          <td style={{ padding: "8px 12px" }}>
                            <span
                              style={{
                                display: "inline-block",
                                padding: "1px 6px",
                                borderRadius: 4,
                                fontSize: 10,
                                fontWeight: 600,
                                textTransform: "uppercase",
                                background: tx.type === "contract_deposit" ? "rgba(16, 185, 129, 0.15)" : "var(--surface-high)",
                                color: tx.type === "contract_deposit" ? "var(--success)" : "var(--text)",
                              }}
                            >
                              {tx.type.replace(/_/g, " ")}
                            </span>
                          </td>
                          <td style={{ padding: "8px 12px" }} className="mono">
                            {tx.contract_ref || "—"}
                          </td>
                          <td
                            style={{
                              padding: "8px 12px",
                              textAlign: "right",
                              fontWeight: 700,
                              color: isPositive ? "var(--success)" : "var(--danger)",
                            }}
                            className="mono"
                          >
                            {isPositive ? `+${fmtMoney(tx.amount, 4)}` : fmtMoney(tx.amount, 4)}
                          </td>
                          <td style={{ padding: "8px 12px", textAlign: "right", fontWeight: 600 }} className="mono">
                            {fmtMoney(tx.balance_after, 4)}
                          </td>
                          <td style={{ padding: "8px 12px", maxWidth: 180 }} className="muted">
                            {tx.notes || "—"}
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>

            <div style={{ display: "flex", justifyContent: "flex-end" }}>
              <button className="btn" onClick={() => setLedgerTarget(null)}>
                {t("common.close", "Close")}
              </button>
            </div>
          </div>
        </Modal>
      )}

      {/* DELETE CONFIRMATION MODAL */}
      {deleteTarget && (
        <Modal title={t("tenants.delete_modal_title", "Confirm Tenant Deletion")} onClose={() => setDeleteTarget(null)}>
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <p style={{ margin: 0, fontSize: 13 }}>
              Are you sure you want to delete tenant <strong>{deleteTarget.name}</strong>?
            </p>
            <p className="muted" style={{ margin: 0, fontSize: 12 }}>
              Associated users will be unlinked, and API requests using this tenant will be revoked.
            </p>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 8 }}>
              <button className="btn" onClick={() => setDeleteTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button className="btn btn--danger" onClick={confirmDelete} disabled={busy}>
                {busy ? t("common.deleting", "Deleting...") : t("common.delete", "Delete Tenant")}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
