import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { VirtualModel, Tenant, ModelPricing } from "../api/types";
import { useToast } from "../components/Toast";
import Modal from "../components/Modal";
import { useLanguage } from "../i18n";
import { fmtDate } from "../lib/format";

interface TargetRowState {
  provider: string;
  model_name: string;
  weight: number;
  priority: number;
  is_active: boolean;
}

interface VirtualModelFormData {
  name: string;
  description: string;
  tenant_id: string; // "" = Global
  routing_strategy: "weighted" | "fallback" | "round_robin";
  is_active: boolean;
  targets: TargetRowState[];
}

const emptyForm: VirtualModelFormData = {
  name: "",
  description: "",
  tenant_id: "",
  routing_strategy: "weighted",
  is_active: true,
  targets: [
    { provider: "openai", model_name: "gpt-4o-mini", weight: 100, priority: 1, is_active: true },
  ],
};

const PROVIDER_OPTIONS = ["openai", "anthropic", "gemini"];

export default function VirtualModels() {
  const { t } = useLanguage();
  const toast = useToast();

  const [virtualModels, setVirtualModels] = useState<VirtualModel[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [availableModels, setAvailableModels] = useState<ModelPricing[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [tenantFilter, setTenantFilter] = useState("all");

  // Create / Edit modal states
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<VirtualModelFormData>(emptyForm);
  const [submitting, setSubmitting] = useState(false);

  // Delete modal
  const [deleteTarget, setDeleteTarget] = useState<VirtualModel | null>(null);
  const [deleting, setDeleting] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [vms, tList, mList] = await Promise.all([
        api.listVirtualModels(),
        api.listTenants().catch(() => []),
        api.listModels().catch(() => []),
      ]);
      setVirtualModels(Array.isArray(vms) ? vms : []);
      setTenants(Array.isArray(tList) ? tList : []);
      setAvailableModels(Array.isArray(mList) ? mList : []);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to load virtual models", "Error");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleOpenCreate = () => {
    setEditingId(null);
    setForm(emptyForm);
    setModalOpen(true);
  };

  const handleOpenEdit = (vm: VirtualModel) => {
    setEditingId(vm.id);
    setForm({
      name: vm.name,
      description: vm.description || "",
      tenant_id: vm.tenant_id || "",
      routing_strategy: vm.routing_strategy || "weighted",
      is_active: vm.is_active,
      targets: vm.targets && vm.targets.length > 0
        ? vm.targets.map((t) => ({
            provider: t.provider,
            model_name: t.model_name,
            weight: t.weight || 100,
            priority: t.priority || 1,
            is_active: t.is_active,
          }))
        : [
            { provider: "openai", model_name: "gpt-4o-mini", weight: 100, priority: 1, is_active: true },
          ],
    });
    setModalOpen(true);
  };

  const handleAddTarget = () => {
    setForm((prev) => ({
      ...prev,
      targets: [
        ...prev.targets,
        { provider: "openai", model_name: "", weight: 50, priority: 1, is_active: true },
      ],
    }));
  };

  const handleRemoveTarget = (index: number) => {
    setForm((prev) => ({
      ...prev,
      targets: prev.targets.filter((_, i) => i !== index),
    }));
  };

  const handleTargetChange = (index: number, field: keyof TargetRowState, val: unknown) => {
    setForm((prev) => {
      const nextTargets = [...prev.targets];
      nextTargets[index] = { ...nextTargets[index], [field]: val };
      return { ...prev, targets: nextTargets };
    });
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) {
      toast.error("Virtual model alias name is required", "Validation");
      return;
    }
    if (form.targets.length === 0) {
      toast.error("At least one target model is required", "Validation");
      return;
    }

    setSubmitting(true);
    try {
      const payload = {
        name: form.name.trim(),
        description: form.description.trim(),
        tenant_id: form.tenant_id ? form.tenant_id : null,
        routing_strategy: form.routing_strategy,
        is_active: form.is_active,
        targets: form.targets.map((t) => ({
          provider: t.provider.trim(),
          model_name: t.model_name.trim(),
          weight: Number(t.weight) || 100,
          priority: Number(t.priority) || 1,
          is_active: t.is_active,
        })),
      };

      if (editingId) {
        await api.updateVirtualModel(editingId, payload);
        toast.success(`Virtual model "${form.name}" updated`, "Success");
      } else {
        await api.createVirtualModel(payload);
        toast.success(`Virtual model "${form.name}" created`, "Success");
      }

      setModalOpen(false);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save virtual model", "Error");
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteVirtualModel(deleteTarget.id);
      toast.success(`Virtual model "${deleteTarget.name}" deleted`, "Deleted");
      setDeleteTarget(null);
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete virtual model", "Error");
    } finally {
      setDeleting(false);
    }
  };

  const handleToggleActive = async (vm: VirtualModel) => {
    try {
      await api.updateVirtualModel(vm.id, { is_active: !vm.is_active });
      toast.info(`Virtual model "${vm.name}" ${!vm.is_active ? "activated" : "deactivated"}`, "Status");
      loadData();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update status", "Error");
    }
  };

  // Filtered list
  const filtered = virtualModels.filter((vm) => {
    const matchSearch =
      !search ||
      vm.name.toLowerCase().includes(search.toLowerCase()) ||
      (vm.description && vm.description.toLowerCase().includes(search.toLowerCase())) ||
      (vm.targets && vm.targets.some((t) => t.model_name.toLowerCase().includes(search.toLowerCase())));

    const matchTenant =
      tenantFilter === "all" ||
      (tenantFilter === "global" && !vm.tenant_id) ||
      vm.tenant_id === tenantFilter;

    return matchSearch && matchTenant;
  });

  // Calculate stats
  const totalCount = virtualModels.length;
  const activeCount = virtualModels.filter((vm) => vm.is_active).length;
  const canaryCount = virtualModels.filter(
    (vm) =>
      vm.routing_strategy === "weighted" &&
      (vm.targets || []).filter((t) => t.is_active && (t.priority === 1 || !t.priority)).length > 1
  ).length;

  return (
    <div className="content-container">
      {/* Top Banner / Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: "20px" }}>
        <div>
          <h1 style={{ margin: 0, fontSize: "20px", fontWeight: 700, color: "var(--text)" }}>
            {t("virtual_models.title", "Virtual Models & Dynamic Routing")}
          </h1>
          <p style={{ margin: "4px 0 0", fontSize: "13px", color: "var(--text-muted)" }}>
            {t("virtual_models.subtitle", "Decouple client code from real providers with business aliases, Canary traffic splitting, and automated failover ladders.")}
          </p>
        </div>
        <button className="btn btn--primary" onClick={handleOpenCreate} style={{ display: "flex", alignItems: "center", gap: "6px" }}>
          <span className="material-symbols-outlined" style={{ fontSize: "18px" }}>add</span>
          <span>{t("virtual_models.btn_create", "Create Alias")}</span>
        </button>
      </div>

      {/* KPI Stats Overview */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: "12px", marginBottom: "20px" }}>
        <div className="card card--stat" style={{ padding: "14px 16px" }}>
          <div style={{ fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted)", letterSpacing: "0.5px" }}>
            {t("virtual_models.stat_total", "Total Virtual Models")}
          </div>
          <div style={{ fontSize: "24px", fontWeight: 700, color: "var(--text)", marginTop: "4px" }}>
            {totalCount}
          </div>
        </div>
        <div className="card card--stat" style={{ padding: "14px 16px" }}>
          <div style={{ fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted)", letterSpacing: "0.5px" }}>
            {t("virtual_models.stat_active", "Active Aliases")}
          </div>
          <div style={{ fontSize: "24px", fontWeight: 700, color: "#10b981", marginTop: "4px" }}>
            {activeCount}
          </div>
        </div>
        <div className="card card--stat" style={{ padding: "14px 16px" }}>
          <div style={{ fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted)", letterSpacing: "0.5px" }}>
            {t("virtual_models.stat_canary", "Canary / A-B Testing")}
          </div>
          <div style={{ fontSize: "24px", fontWeight: 700, color: "#f59e0b", marginTop: "4px" }}>
            {canaryCount}
          </div>
        </div>
      </div>

      {/* Filters Bar */}
      <div style={{ display: "flex", gap: "12px", alignItems: "center", marginBottom: "16px", flexWrap: "wrap" }}>
        <div style={{ flex: 1, minWidth: "220px", position: "relative" }}>
          <input
            type="text"
            className="input"
            placeholder={t("virtual_models.search_placeholder", "Search aliases, targets, models...")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{ width: "100%", paddingLeft: "32px" }}
          />
          <span
            className="material-symbols-outlined"
            style={{ position: "absolute", left: "10px", top: "50%", transform: "translateY(-50%)", fontSize: "16px", color: "var(--text-muted)" }}
          >
            search
          </span>
        </div>

        <select
          className="select"
          value={tenantFilter}
          onChange={(e) => setTenantFilter(e.target.value)}
          style={{ minWidth: "180px" }}
        >
          <option value="all">{t("virtual_models.filter_all_tenants", "All Scopes")}</option>
          <option value="global">{t("virtual_models.filter_global_only", "Global (Tenant-Agnostic)")}</option>
          {tenants.map((ten) => (
            <option key={ten.id} value={ten.id}>
              {ten.name}
            </option>
          ))}
        </select>
      </div>

      {/* Main List */}
      {loading ? (
        <div style={{ padding: "40px", textAlign: "center", color: "var(--text-muted)" }}>
          {t("common.loading", "Loading...")}
        </div>
      ) : filtered.length === 0 ? (
        <div className="card" style={{ padding: "48px 24px", textAlign: "center", color: "var(--text-muted)" }}>
          <span className="material-symbols-outlined" style={{ fontSize: "48px", color: "var(--border)", marginBottom: "12px" }}>
            alt_route
          </span>
          <div style={{ fontSize: "15px", fontWeight: 600, color: "var(--text)", marginBottom: "4px" }}>
            {t("virtual_models.no_models_found", "No Virtual Models configured")}
          </div>
          <p style={{ fontSize: "13px", maxWidth: "460px", margin: "0 auto 16px" }}>
            {t("virtual_models.no_models_desc", "Define a Virtual Model Alias like 'production-fast-chat' or 'production-coder' so your apps don't have to hardcode vendor-specific model names.")}
          </p>
          <button className="btn btn--primary" onClick={handleOpenCreate}>
            {t("virtual_models.btn_create", "Create Alias")}
          </button>
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          {filtered.map((vm) => {
            const tenantObj = tenants.find((tn) => tn.id === vm.tenant_id);
            const primaryTargets = (vm.targets || []).filter((t) => t.is_active && (t.priority === 1 || !t.priority));
            const fallbackTargets = (vm.targets || []).filter((t) => t.is_active && t.priority > 1);
            const totalPrimaryWeight = primaryTargets.reduce((acc, cur) => acc + (cur.weight || 1), 0) || 1;

            return (
              <div
                key={vm.id}
                className="card"
                style={{
                  padding: "16px 20px",
                  display: "flex",
                  flexDirection: "column",
                  gap: "12px",
                  borderLeft: vm.is_active ? "3px solid #10b981" : "3px solid var(--border)",
                }}
              >
                {/* Row Header */}
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: "8px" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                    <span
                      style={{
                        fontFamily: "JetBrains Mono, monospace",
                        fontSize: "15px",
                        fontWeight: 700,
                        color: "var(--text)",
                      }}
                    >
                      {vm.name}
                    </span>

                    {/* Scope Badge */}
                    {tenantObj ? (
                      <span className="terminal-badge" style={{ fontSize: "11px", padding: "2px 6px" }}>
                        {tenantObj.name}
                      </span>
                    ) : (
                      <span className="terminal-badge" style={{ fontSize: "11px", padding: "2px 6px", color: "#38bdf8" }}>
                        GLOBAL
                      </span>
                    )}

                    {/* Strategy Badge */}
                    <span
                      className="terminal-badge"
                      style={{
                        fontSize: "11px",
                        padding: "2px 6px",
                        color: vm.routing_strategy === "weighted" ? "#f59e0b" : "#a855f7",
                      }}
                    >
                      {vm.routing_strategy === "weighted"
                        ? primaryTargets.length > 1 ? "Canary / Weighted" : "Direct Alias"
                        : vm.routing_strategy === "fallback"
                        ? "Failover Ladder"
                        : "Round Robin"}
                    </span>

                    {!vm.is_active && (
                      <span className="terminal-badge" style={{ fontSize: "11px", padding: "2px 6px", color: "#ef4444" }}>
                        INACTIVE
                      </span>
                    )}
                  </div>

                  {/* Action Buttons */}
                  <div style={{ display: "flex", gap: "6px" }}>
                    <button
                      className={`btn btn--xs ${vm.is_active ? "btn--ghost" : "btn--secondary"}`}
                      onClick={() => handleToggleActive(vm)}
                    >
                      {vm.is_active ? t("common.deactivate", "Deactivate") : t("common.activate", "Activate")}
                    </button>
                    <button className="btn btn--secondary btn--xs" onClick={() => handleOpenEdit(vm)}>
                      {t("common.edit", "Edit")}
                    </button>
                    <button
                      className="btn btn--ghost btn--xs"
                      style={{ color: "#ef4444" }}
                      onClick={() => setDeleteTarget(vm)}
                    >
                      {t("common.delete", "Delete")}
                    </button>
                  </div>
                </div>

                {/* Description */}
                {vm.description && (
                  <div style={{ fontSize: "12px", color: "var(--text-muted)", marginTop: "-4px" }}>
                    {vm.description}
                  </div>
                )}

                {/* Visual Traffic Splitting Bar (Primary Pool) */}
                {primaryTargets.length > 0 && (
                  <div>
                    <div style={{ display: "flex", justifyContent: "space-between", fontSize: "11px", color: "var(--text-muted)", marginBottom: "4px" }}>
                      <span>
                        {primaryTargets.length === 1
                          ? t("virtual_models.direct_target", "Primary Target:")
                          : t("virtual_models.traffic_distribution", "Primary Traffic Distribution:")}
                      </span>
                      <span>
                        {primaryTargets.length === 1
                          ? "1 target (100%)"
                          : `${primaryTargets.length} targets (Canary Split)`}
                      </span>
                    </div>

                    {/* Multi-color segment bar */}
                    <div
                      style={{
                        display: "flex",
                        height: "8px",
                        borderRadius: "4px",
                        overflow: "hidden",
                        backgroundColor: "var(--bg-secondary)",
                      }}
                    >
                      {primaryTargets.map((tItem, idx) => {
                        const pct = Math.round(((tItem.weight || 1) / totalPrimaryWeight) * 100);
                        const colors = ["#38bdf8", "#10b981", "#f59e0b", "#a855f7", "#ec4899"];
                        const segColor = colors[idx % colors.length];
                        return (
                          <div
                            key={idx}
                            style={{
                              width: `${pct}%`,
                              backgroundColor: segColor,
                              transition: "width 0.2s ease",
                            }}
                            title={`${tItem.provider}/${tItem.model_name}: ${pct}%`}
                          />
                        );
                      })}
                    </div>

                    {/* Targets Legend */}
                    <div style={{ display: "flex", gap: "12px", flexWrap: "wrap", marginTop: "8px" }}>
                      {primaryTargets.map((tItem, idx) => {
                        const pct = Math.round(((tItem.weight || 1) / totalPrimaryWeight) * 100);
                        const colors = ["#38bdf8", "#10b981", "#f59e0b", "#a855f7", "#ec4899"];
                        const dotColor = colors[idx % colors.length];
                        return (
                          <div
                            key={idx}
                            style={{
                              display: "flex",
                              alignItems: "center",
                              gap: "5px",
                              fontSize: "12px",
                              fontFamily: "JetBrains Mono, monospace",
                            }}
                          >
                            <span style={{ width: "8px", height: "8px", borderRadius: "50%", backgroundColor: dotColor }} />
                            <span style={{ color: "var(--text)" }}>{tItem.provider}/<strong>{tItem.model_name}</strong></span>
                            {primaryTargets.length > 1 && (
                              <span style={{ color: "var(--text-muted)" }}>({pct}%)</span>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Fallback Ladder (if any targets have priority > 1) */}
                {fallbackTargets.length > 0 && (
                  <div
                    style={{
                      borderTop: "1px dashed var(--border)",
                      paddingTop: "8px",
                      marginTop: "4px",
                      display: "flex",
                      alignItems: "center",
                      gap: "8px",
                      fontSize: "12px",
                      color: "var(--text-muted)",
                      flexWrap: "wrap",
                    }}
                  >
                    <span style={{ display: "flex", alignItems: "center", gap: "4px" }}>
                      <span className="material-symbols-outlined" style={{ fontSize: "15px", color: "#f59e0b" }}>
                        emergency_share
                      </span>
                      <span>{t("virtual_models.fallback_chain", "Fallback Ladder:")}</span>
                    </span>
                    {fallbackTargets.map((fb, idx) => (
                      <span
                        key={idx}
                        className="terminal-badge"
                        style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", padding: "2px 6px" }}
                      >
                        Tier {fb.priority}: {fb.provider}/{fb.model_name}
                      </span>
                    ))}
                  </div>
                )}

                {/* Footer metadata */}
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: "11px", color: "var(--text-muted)", marginTop: "2px" }}>
                  <span>ID: <code className="mono">{vm.id}</code></span>
                  <span>{fmtDate(vm.created_at)}</span>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Modal Create / Edit */}
      {modalOpen && (
        <Modal
          title={editingId ? t("virtual_models.edit_title", "Edit Virtual Model Alias") : t("virtual_models.create_title", "Create Virtual Model Alias")}
          onClose={() => setModalOpen(false)}
          maxWidth="700px"
        >
          <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
            <div>
              <label className="label">
                {t("virtual_models.alias_name", "Model Alias Name")} *
              </label>
              <input
                type="text"
                className="input"
                placeholder="e.g. production-fast-chat, production-coder"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
                style={{ fontFamily: "JetBrains Mono, monospace" }}
              />
              <span style={{ fontSize: "11px", color: "var(--text-muted)" }}>
                {t("virtual_models.alias_name_hint", "Clients will pass this exact string in the 'model' field of API calls.")}
              </span>
            </div>

            <div>
              <label className="label">{t("common.description", "Description")}</label>
              <input
                type="text"
                className="input"
                placeholder="e.g. Ultra-low latency model for user facing chat"
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <div>
                <label className="label">{t("virtual_models.routing_strategy", "Routing Strategy")}</label>
                <select
                  className="select"
                  value={form.routing_strategy}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      routing_strategy: e.target.value as "weighted" | "fallback" | "round_robin",
                    })
                  }
                  style={{ width: "100%" }}
                >
                  <option value="weighted">{t("virtual_models.strat_weighted", "Canary / Traffic Splitting (Weighted)")}</option>
                  <option value="fallback">{t("virtual_models.strat_fallback", "Active-Passive (Strict Priority)")}</option>
                  <option value="round_robin">{t("virtual_models.strat_rr", "Round Robin")}</option>
                </select>
              </div>

              <div>
                <label className="label">{t("virtual_models.scope_tenant", "Tenant Scope")}</label>
                <select
                  className="select"
                  value={form.tenant_id}
                  onChange={(e) => setForm({ ...form, tenant_id: e.target.value })}
                  style={{ width: "100%" }}
                >
                  <option value="">{t("virtual_models.scope_global", "Global (All tenants & keys)")}</option>
                  {tenants.map((tn) => (
                    <option key={tn.id} value={tn.id}>
                      {tn.name}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {/* Target Models Manager */}
            <div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "8px" }}>
                <label className="label" style={{ margin: 0 }}>
                  {t("virtual_models.targets_title", "Upstream Targets & Fallback Chain")} *
                </label>
                <button
                  type="button"
                  className="btn btn--secondary btn--xs"
                  onClick={handleAddTarget}
                  style={{ display: "flex", alignItems: "center", gap: "4px" }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: "14px" }}>add</span>
                  <span>{t("virtual_models.add_target", "Add Target")}</span>
                </button>
              </div>

              <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                {form.targets.map((row, idx) => (
                  <div
                    key={idx}
                    style={{
                      display: "grid",
                      gridTemplateColumns: "130px 1fr 90px 80px 36px",
                      gap: "8px",
                      alignItems: "center",
                      backgroundColor: "var(--bg-secondary)",
                      padding: "8px 10px",
                      borderRadius: "6px",
                      border: "1px solid var(--border)",
                    }}
                  >
                    {/* Provider Select */}
                    <select
                      className="select"
                      value={row.provider}
                      onChange={(e) => handleTargetChange(idx, "provider", e.target.value)}
                      style={{ padding: "6px 8px", fontSize: "12px" }}
                    >
                      {PROVIDER_OPTIONS.map((p) => (
                        <option key={p} value={p}>
                          {p}
                        </option>
                      ))}
                    </select>

                    {/* Model Name Input / Autocomplete */}
                    <input
                      type="text"
                      className="input"
                      placeholder="e.g. gpt-4o-mini"
                      value={row.model_name}
                      onChange={(e) => handleTargetChange(idx, "model_name", e.target.value)}
                      style={{ padding: "6px 8px", fontSize: "12px", fontFamily: "JetBrains Mono, monospace" }}
                      list={`models-list-${idx}`}
                      required
                    />
                    <datalist id={`models-list-${idx}`}>
                      {availableModels
                        .filter((m) => m.provider === row.provider)
                        .map((m) => (
                          <option key={m.id} value={m.model_name} />
                        ))}
                    </datalist>

                    {/* Weight (for Canary) */}
                    <div style={{ display: "flex", alignItems: "center", gap: "4px" }}>
                      <input
                        type="number"
                        className="input"
                        min="1"
                        max="1000"
                        title="Weight for traffic splitting (e.g. 90 vs 10)"
                        value={row.weight}
                        onChange={(e) => handleTargetChange(idx, "weight", parseInt(e.target.value) || 1)}
                        style={{ padding: "6px 8px", fontSize: "12px" }}
                      />
                      <span style={{ fontSize: "10px", color: "var(--text-muted)" }}>wt</span>
                    </div>

                    {/* Priority Tier */}
                    <div style={{ display: "flex", alignItems: "center", gap: "4px" }}>
                      <input
                        type="number"
                        className="input"
                        min="1"
                        max="10"
                        title="1 = Primary pool, 2 = 1st Fallback, 3 = 2nd Fallback"
                        value={row.priority}
                        onChange={(e) => handleTargetChange(idx, "priority", parseInt(e.target.value) || 1)}
                        style={{ padding: "6px 8px", fontSize: "12px" }}
                      />
                      <span style={{ fontSize: "10px", color: "var(--text-muted)" }}>pri</span>
                    </div>

                    {/* Delete row button */}
                    <button
                      type="button"
                      className="btn btn--ghost btn--xs"
                      onClick={() => handleRemoveTarget(idx)}
                      disabled={form.targets.length <= 1}
                      style={{ padding: "4px", color: form.targets.length <= 1 ? "var(--border)" : "#ef4444" }}
                    >
                      <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>delete</span>
                    </button>
                  </div>
                ))}
              </div>
              <span style={{ fontSize: "11px", color: "var(--text-muted)", marginTop: "6px", display: "block" }}>
                {t("virtual_models.priority_hint", "Targets with Priority = 1 are in the Primary Pool (split by weight). Targets with Priority > 1 are tried sequentially if Primary returns 429, 5xx, or times out.")}
              </span>
            </div>

            {/* Active Toggle */}
            <div style={{ display: "flex", alignItems: "center", gap: "8px", marginTop: "4px" }}>
              <input
                type="checkbox"
                id="vm-active"
                checked={form.is_active}
                onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
              />
              <label htmlFor="vm-active" style={{ fontSize: "13px", color: "var(--text)", cursor: "pointer" }}>
                {t("virtual_models.is_active", "Enable this virtual model immediately")}
              </label>
            </div>

            {/* Modal Actions */}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: "8px", marginTop: "12px" }}>
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() => setModalOpen(false)}
                disabled={submitting}
              >
                {t("common.cancel", "Cancel")}
              </button>
              <button type="submit" className="btn btn--primary" disabled={submitting}>
                {submitting ? t("common.saving", "Saving...") : t("common.save", "Save Virtual Model")}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <Modal
          title={t("virtual_models.delete_confirm_title", "Confirm Deletion")}
          onClose={() => setDeleteTarget(null)}
          maxWidth="450px"
        >
          <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
            <p style={{ margin: 0, fontSize: "13px", color: "var(--text-muted)" }}>
              {t("virtual_models.delete_confirm_msg", "Are you sure you want to delete virtual model alias")}{" "}
              <strong style={{ color: "var(--text)" }}>"{deleteTarget.name}"</strong>?
              {t("virtual_models.delete_warn", " Client applications calling this alias will start receiving Unknown Model errors.")}
            </p>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: "8px" }}>
              <button
                className="btn btn--secondary"
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
              >
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                style={{ backgroundColor: "#ef4444" }}
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? t("common.deleting", "Deleting...") : t("common.delete", "Delete")}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
