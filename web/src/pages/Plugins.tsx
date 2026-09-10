import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Plugin, PluginBinding, Tenant } from "../api/types";
import { useToast } from "../components/Toast";
import PluginBindingModal from "../components/PluginBindingModal";
import Modal from "../components/Modal";

const CATEGORIES = [
  { id: "all", label: "All Plugins" },
  { id: "traffic", label: "Traffic & Quota" },
  { id: "security", label: "Security & Guardrails" },
  { id: "performance", label: "Performance & Cache" },
  { id: "observability", label: "Observability" },
];

const SCOPE_HIERARCHY = [
  { level: "L1", key: "api_key", label: "API Key", icon: "key", color: "#22d3ee" },
  { level: "L2", key: "user", label: "User", icon: "person", color: "#8aebff" },
  { level: "L3", key: "role", label: "Role", icon: "shield_person", color: "#4edea3" },
  { level: "L4", key: "tenant", label: "Tenant", icon: "domain", color: "#ffb13b" },
  { level: "L5", key: "model", label: "Model", icon: "memory", color: "#ffd6a3" },
  { level: "L6", key: "provider", label: "Provider", icon: "hub", color: "#bbc9cd" },
  { level: "L7", key: "global", label: "Global", icon: "public", color: "#859397" },
];

export default function Plugins() {
  const toast = useToast();

  const [activeTab, setActiveTab] = useState<"catalog" | "bindings">("catalog");
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [bindings, setBindings] = useState<PluginBinding[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(false);

  // Filters
  const [categoryFilter, setCategoryFilter] = useState<string>("all");
  const [scopeFilter, setScopeFilter] = useState<string>("all");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "inactive">("all");
  const [search, setSearch] = useState<string>("");

  // Modal states
  const [modalOpen, setModalOpen] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);
  const [editingBinding, setEditingBinding] = useState<PluginBinding | null>(null);

  // Delete modal state
  const [deleteTarget, setDeleteTarget] = useState<PluginBinding | null>(null);
  const [deleting, setDeleting] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [pList, bList, tList] = await Promise.all([
        api.listPlugins(),
        api.listPluginBindings(),
        api.listTenants().catch(() => []),
      ]);
      setPlugins(pList);
      setBindings(bList);
      setTenants(tList);
    } catch (err: any) {
      toast.error(err.message || "Failed to load plugins", "Error");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleToggleBindingActive = async (b: PluginBinding) => {
    try {
      await api.updatePluginBinding(b.id, { is_active: !b.is_active });
      toast.success(`Binding ${!b.is_active ? "activated" : "deactivated"}`);
      loadData();
    } catch (err: any) {
      toast.error(err.message || "Failed to toggle binding status");
    }
  };

  const handleDeleteBinding = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deletePluginBinding(deleteTarget.id);
      toast.success("Plugin binding removed");
      setDeleteTarget(null);
      loadData();
    } catch (err: any) {
      toast.error(err.message || "Failed to delete binding");
    } finally {
      setDeleting(false);
    }
  };

  // Scope target badge helper
  const renderScopeTarget = (b: PluginBinding) => {
    switch (b.scope_level) {
      case "global":
        return <span className="badge badge--neutral">Global (All Requests)</span>;
      case "provider":
        return <span className="badge badge--info">Provider: {b.scope_provider}</span>;
      case "model":
        return <span className="badge badge--info">Model: {b.scope_model}</span>;
      case "tenant": {
        const t = tenants.find((item) => item.id === b.scope_tenant_id);
        return <span className="badge badge--primary">Tenant: {t?.name || b.scope_tenant_id}</span>;
      }
      case "role":
        return <span className="badge badge--warning">Role: {b.scope_role}</span>;
      case "user":
        return <span className="badge badge--success">User: {b.scope_user_id?.slice(0, 8)}...</span>;
      case "api_key":
        return <span className="badge badge--neutral">Key: {b.scope_key_id?.slice(0, 8)}...</span>;
      default:
        return <span>{b.scope_level}</span>;
    }
  };

  const activeBindingsCount = bindings.filter((b) => b.is_active).length;

  const filteredPlugins = plugins.filter((p) => {
    if (categoryFilter !== "all" && p.category !== categoryFilter) return false;
    if (search.trim()) {
      const q = search.toLowerCase();
      return (
        p.name.toLowerCase().includes(q) ||
        p.display_name.toLowerCase().includes(q) ||
        p.description.toLowerCase().includes(q) ||
        p.hooks.some((h) => h.toLowerCase().includes(q))
      );
    }
    return true;
  });

  const filteredBindings = bindings.filter((b) => {
    if (scopeFilter !== "all" && b.scope_level !== scopeFilter) return false;
    if (statusFilter === "active" && !b.is_active) return false;
    if (statusFilter === "inactive" && b.is_active) return false;
    if (search.trim()) {
      const q = search.toLowerCase();
      return (
        b.plugin_name.toLowerCase().includes(q) ||
        (b.plugin_display_name && b.plugin_display_name.toLowerCase().includes(q)) ||
        b.scope_level.toLowerCase().includes(q) ||
        (b.scope_tenant_id && b.scope_tenant_id.toLowerCase().includes(q)) ||
        (b.scope_model && b.scope_model.toLowerCase().includes(q)) ||
        (b.scope_provider && b.scope_provider.toLowerCase().includes(q)) ||
        (b.scope_role && b.scope_role.toLowerCase().includes(q))
      );
    }
    return true;
  });

  const hasActiveFilters =
    search.trim() !== "" ||
    (activeTab === "catalog" && categoryFilter !== "all") ||
    (activeTab === "bindings" && (scopeFilter !== "all" || statusFilter !== "all"));

  const handleResetFilters = () => {
    setSearch("");
    setCategoryFilter("all");
    setScopeFilter("all");
    setStatusFilter("all");
  };

  return (
    <div className="page" style={{ maxWidth: 1440, margin: "0 auto", display: "flex", flexDirection: "column", gap: "1.25rem" }}>
      {/* Header Section */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: "1rem" }}>
        <div>
          <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
            <h1 className="page__title" style={{ margin: 0, fontSize: "1.75rem", letterSpacing: "-0.02em" }}>
              Plugins &amp; Middleware
            </h1>
            <span
              className="mono"
              style={{
                fontSize: "11px",
                fontWeight: 700,
                color: "var(--primary, #22d3ee)",
                background: "rgba(34, 211, 238, 0.1)",
                border: "1px solid rgba(34, 211, 238, 0.3)",
                padding: "2px 8px",
                borderRadius: "4px",
              }}
            >
              PIPELINE v0.3.1
            </span>
          </div>
          <p className="page__subtitle" style={{ marginTop: "0.25rem", color: "var(--text-muted, #859397)" }}>
            Kong-like request/response hooks, 7-level scope inheritance, and low-latency gateway middleware.
          </p>
        </div>

        <div style={{ display: "flex", gap: "0.75rem" }}>
          <button
            type="button"
            className="btn btn--primary"
            style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontWeight: 600 }}
            onClick={() => {
              setEditingBinding(null);
              setSelectedPlugin(plugins.length > 0 ? plugins[0] : null);
              setModalOpen(true);
            }}
          >
            <span className="material-symbols-outlined" style={{ fontSize: "1.1rem" }}>add</span>
            Bind Plugin
          </button>
        </div>
      </div>

      {/* Top Telemetry KPI Strip */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: "1rem" }}>
        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "6px",
            padding: "1rem 1.25rem",
            position: "relative",
            overflow: "hidden",
          }}
        >
          <div style={{ fontSize: "11px", fontWeight: 700, color: "var(--text-muted, #859397)", textTransform: "uppercase", letterSpacing: "0.05em", fontFamily: "var(--font-mono)" }}>
            Total Plugins
          </div>
          <div style={{ fontSize: "1.75rem", fontWeight: 700, color: "var(--primary-soft, #8aebff)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
            {plugins.length}
          </div>
          <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: "3px", background: "linear-gradient(90deg, #22d3ee, transparent)" }} />
        </div>

        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "6px",
            padding: "1rem 1.25rem",
            position: "relative",
            overflow: "hidden",
            boxShadow: "0 0 16px rgba(16, 185, 129, 0.12)",
          }}
        >
          <div style={{ fontSize: "11px", fontWeight: 700, color: "var(--text-muted, #859397)", textTransform: "uppercase", letterSpacing: "0.05em", fontFamily: "var(--font-mono)" }}>
            Active Scope Bindings
          </div>
          <div style={{ fontSize: "1.75rem", fontWeight: 700, color: "var(--emerald-soft, #4edea3)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
            {activeBindingsCount} <span style={{ fontSize: "0.9rem", color: "var(--text-muted, #859397)", fontWeight: 400 }}>/ {bindings.length}</span>
          </div>
          <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: "3px", background: "linear-gradient(90deg, #10b981, transparent)" }} />
        </div>

        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "6px",
            padding: "1rem 1.25rem",
            position: "relative",
            overflow: "hidden",
          }}
        >
          <div style={{ fontSize: "11px", fontWeight: 700, color: "var(--text-muted, #859397)", textTransform: "uppercase", letterSpacing: "0.05em", fontFamily: "var(--font-mono)" }}>
            Filtered Throughput
          </div>
          <div style={{ fontSize: "1.75rem", fontWeight: 700, color: "var(--amber-soft, #ffb957)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
            4,850 <span style={{ fontSize: "0.85rem", color: "var(--text-muted, #859397)" }}>RPM</span>
          </div>
          <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: "3px", background: "linear-gradient(90deg, #f59e0b, transparent)" }} />
        </div>

        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "6px",
            padding: "1rem 1.25rem",
            position: "relative",
            overflow: "hidden",
          }}
        >
          <div style={{ fontSize: "11px", fontWeight: 700, color: "var(--text-muted, #859397)", textTransform: "uppercase", letterSpacing: "0.05em", fontFamily: "var(--font-mono)" }}>
            Pipeline Overhead
          </div>
          <div style={{ fontSize: "1.75rem", fontWeight: 700, color: "var(--primary, #22d3ee)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
            +1.2 <span style={{ fontSize: "0.85rem", color: "var(--text-muted, #859397)" }}>ms avg</span>
          </div>
          <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: "3px", background: "linear-gradient(90deg, #22d3ee, transparent)" }} />
        </div>
      </div>

      {/* View Switcher Tabs Bar */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          flexWrap: "wrap",
          gap: "1rem",
          borderBottom: "1px solid var(--border, #2d3139)",
          paddingBottom: "0.25rem",
          marginBottom: "0.75rem",
        }}
      >
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button
            type="button"
            style={{
              display: "flex",
              alignItems: "center",
              gap: "0.5rem",
              padding: "8px 16px",
              borderRadius: "6px 6px 0 0",
              border: "none",
              borderBottom: activeTab === "catalog" ? "2px solid var(--primary, #22d3ee)" : "2px solid transparent",
              background: activeTab === "catalog" ? "var(--surface-raised, #181a20)" : "transparent",
              color: activeTab === "catalog" ? "var(--primary, #22d3ee)" : "var(--text-muted, #859397)",
              fontWeight: 700,
              fontSize: "13px",
              cursor: "pointer",
              transition: "all 0.15s ease",
            }}
            onClick={() => setActiveTab("catalog")}
          >
            <span className="material-symbols-outlined" style={{ fontSize: "1.1rem" }}>extension</span>
            Plugin Catalog
            <span
              className="mono"
              style={{
                fontSize: "11px",
                padding: "1px 7px",
                borderRadius: "10px",
                background: activeTab === "catalog" ? "rgba(34, 211, 238, 0.15)" : "var(--surface-high, #1f2229)",
                color: activeTab === "catalog" ? "var(--primary, #22d3ee)" : "var(--text-muted, #859397)",
                fontWeight: 700,
              }}
            >
              {plugins.length}
            </span>
          </button>

          <button
            type="button"
            style={{
              display: "flex",
              alignItems: "center",
              gap: "0.5rem",
              padding: "8px 16px",
              borderRadius: "6px 6px 0 0",
              border: "none",
              borderBottom: activeTab === "bindings" ? "2px solid var(--primary, #22d3ee)" : "2px solid transparent",
              background: activeTab === "bindings" ? "var(--surface-raised, #181a20)" : "transparent",
              color: activeTab === "bindings" ? "var(--primary, #22d3ee)" : "var(--text-muted, #859397)",
              fontWeight: 700,
              fontSize: "13px",
              cursor: "pointer",
              transition: "all 0.15s ease",
            }}
            onClick={() => setActiveTab("bindings")}
          >
            <span className="material-symbols-outlined" style={{ fontSize: "1.1rem" }}>layers</span>
            Scope Bindings
            <span
              className="mono"
              style={{
                fontSize: "11px",
                padding: "1px 7px",
                borderRadius: "10px",
                background: activeTab === "bindings" ? "rgba(34, 211, 238, 0.15)" : "var(--surface-high, #1f2229)",
                color: activeTab === "bindings" ? "var(--primary, #22d3ee)" : "var(--text-muted, #859397)",
                fontWeight: 700,
              }}
            >
              {activeBindingsCount} / {bindings.length}
            </span>
          </button>
        </div>
      </div>

      {/* Dedicated Filter & Search Toolbar */}
      <div
        style={{
          background: "var(--surface-raised, #181a20)",
          border: "1px solid var(--border, #2d3139)",
          borderRadius: "8px",
          padding: "0.75rem 1rem",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: "1rem",
          marginBottom: "1.75rem",
        }}
      >
        {/* Left Side: Search input + View-specific filters */}
        <div style={{ display: "flex", alignItems: "center", gap: "1rem", flexWrap: "wrap", flex: "1 1 500px" }}>
          {/* Search Box */}
          <div style={{ position: "relative", minWidth: 240, flex: "1 1 240px", maxWidth: 380 }}>
            <span
              className="material-symbols-outlined"
              style={{
                position: "absolute",
                left: "10px",
                top: "50%",
                transform: "translateY(-50%)",
                fontSize: "18px",
                color: "var(--text-muted, #859397)",
                pointerEvents: "none",
              }}
            >
              search
            </span>
            <input
              type="text"
              className="input mono"
              placeholder={
                activeTab === "catalog"
                  ? "Search plugins by name, category, hook..."
                  : "Search bindings by plugin, scope, target..."
              }
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{
                paddingLeft: "34px",
                paddingRight: search ? "32px" : "12px",
                fontSize: "13px",
                height: "36px",
                width: "100%",
                borderRadius: "6px",
                background: "var(--surface, #111317)",
                border: "1px solid var(--border, #2d3139)",
              }}
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch("")}
                style={{
                  position: "absolute",
                  right: "8px",
                  top: "50%",
                  transform: "translateY(-50%)",
                  background: "transparent",
                  border: "none",
                  color: "var(--text-muted, #859397)",
                  cursor: "pointer",
                  display: "flex",
                  alignItems: "center",
                  padding: 2,
                }}
                title="Clear search"
              >
                <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>close</span>
              </button>
            )}
          </div>

          {/* Contextual Filters */}
          {activeTab === "catalog" ? (
            /* Category Pills */
            <div style={{ display: "flex", alignItems: "center", gap: "0.4rem", flexWrap: "wrap" }}>
              <span
                className="mono"
                style={{
                  fontSize: "11px",
                  color: "var(--text-muted, #859397)",
                  fontWeight: 700,
                  textTransform: "uppercase",
                  marginRight: "2px",
                }}
              >
                Category:
              </span>
              {CATEGORIES.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  style={{
                    background: categoryFilter === c.id ? "var(--surface-highest, #242b2d)" : "transparent",
                    color: categoryFilter === c.id ? "var(--primary, #22d3ee)" : "var(--text-muted, #859397)",
                    border:
                      categoryFilter === c.id
                        ? "1px solid var(--primary, #22d3ee)"
                        : "1px solid var(--border, #2d3139)",
                    padding: "4px 10px",
                    borderRadius: "4px",
                    fontSize: "12px",
                    fontWeight: 600,
                    cursor: "pointer",
                    transition: "all 0.15s ease",
                  }}
                  onClick={() => setCategoryFilter(c.id)}
                >
                  {c.label}
                </button>
              ))}
            </div>
          ) : (
            /* Bindings Specific Filters: Scope + Status */
            <div style={{ display: "flex", alignItems: "center", gap: "0.75rem", flexWrap: "wrap" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                <span
                  className="mono"
                  style={{
                    fontSize: "11px",
                    color: "var(--text-muted, #859397)",
                    fontWeight: 700,
                    textTransform: "uppercase",
                  }}
                >
                  Scope:
                </span>
                <select
                  className="select mono"
                  value={scopeFilter}
                  onChange={(e) => setScopeFilter(e.target.value)}
                  style={{ height: "36px", fontSize: "12px", minWidth: "140px" }}
                >
                  <option value="all">All Scopes</option>
                  <option value="global">Global (L7)</option>
                  <option value="provider">Provider (L6)</option>
                  <option value="model">Model (L5)</option>
                  <option value="tenant">Tenant (L4)</option>
                  <option value="role">Role (L3)</option>
                  <option value="user">User (L2)</option>
                  <option value="api_key">API Key (L1)</option>
                </select>
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
                <span
                  className="mono"
                  style={{
                    fontSize: "11px",
                    color: "var(--text-muted, #859397)",
                    fontWeight: 700,
                    textTransform: "uppercase",
                  }}
                >
                  Status:
                </span>
                <select
                  className="select mono"
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value as any)}
                  style={{ height: "36px", fontSize: "12px", minWidth: "120px" }}
                >
                  <option value="all">All Statuses</option>
                  <option value="active">Active Only</option>
                  <option value="inactive">Inactive Only</option>
                </select>
              </div>
            </div>
          )}
        </div>

        {/* Right Side: Results Counter & Reset Button */}
        <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
          <span
            className="mono"
            style={{
              fontSize: "11px",
              color: "var(--text-muted, #859397)",
              background: "rgba(255, 255, 255, 0.04)",
              border: "1px solid var(--border, #2d3139)",
              padding: "4px 10px",
              borderRadius: "4px",
            }}
          >
            {activeTab === "catalog"
              ? `Showing ${filteredPlugins.length} of ${plugins.length} plugins`
              : `Showing ${filteredBindings.length} of ${bindings.length} bindings`}
          </span>

          {hasActiveFilters && (
            <button
              type="button"
              className="btn btn--ghost btn--sm"
              style={{
                display: "flex",
                alignItems: "center",
                gap: "4px",
                fontSize: "12px",
                color: "var(--text-muted, #859397)",
                padding: "4px 8px",
              }}
              onClick={handleResetFilters}
              title="Reset all search queries and filters"
            >
              <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>filter_alt_off</span>
              Reset
            </button>
          )}
        </div>
      </div>

      {/* Loading state */}
      {loading && (
        <div style={{ textAlign: "center", padding: "3rem", color: "var(--text-muted, #859397)", fontFamily: "var(--font-mono)" }}>
          [INITIALIZING OBSIDIAN PLUGIN PIPELINE...]
        </div>
      )}

      {/* Scope Hierarchy Banner (Horizontal Bar) */}
      {activeTab === "catalog" && !loading && (
        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "8px",
            padding: "0.75rem 1.25rem",
            marginBottom: "1.25rem",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexWrap: "wrap",
            gap: "0.75rem",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <span className="material-symbols-outlined" style={{ fontSize: "16px", color: "var(--primary, #22d3ee)" }}>
              account_tree
            </span>
            <span style={{ fontSize: "12px", fontWeight: 700, color: "var(--text, #dde4e5)", textTransform: "uppercase", letterSpacing: "0.05em" }}>
              7-Tier Scope Hierarchy:
            </span>
          </div>

          <div style={{ display: "flex", alignItems: "center", gap: "6px", flexWrap: "wrap" }} className="mono">
            {SCOPE_HIERARCHY.map((s, idx) => (
              <div key={s.key} style={{ display: "flex", alignItems: "center", gap: "6px" }}>
                <span
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: "4px",
                    padding: "2px 8px",
                    borderRadius: "4px",
                    background: "rgba(255, 255, 255, 0.03)",
                    border: `1px solid ${s.color}33`,
                    fontSize: "11px",
                    color: s.color,
                  }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: "12px" }}>{s.icon}</span>
                  <strong>{s.label}</strong> ({s.level})
                </span>
                {idx < SCOPE_HIERARCHY.length - 1 && (
                  <span style={{ color: "var(--text-faint, #5c6a6e)", fontSize: "10px", fontWeight: 700 }}>&gt;</span>
                )}
              </div>
            ))}
          </div>

          <span style={{ fontSize: "11px", color: "var(--text-muted, #859397)" }}>
            High-level scopes override general scopes
          </span>
        </div>
      )}

      {/* Full-Width Plugin Cards Grid */}
      {activeTab === "catalog" && !loading && (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))",
            gap: "1.25rem",
            width: "100%",
          }}
        >
          {filteredPlugins.map((p) => {
            const pluginBindings = bindings.filter((b) => b.plugin_name === p.name);
            const activeCount = pluginBindings.filter((b) => b.is_active).length;
            const isEnabled = activeCount > 0;

            return (
              <div
                key={p.name}
                style={{
                  background: "var(--surface-raised, #181a20)",
                  border: isEnabled ? "1px solid var(--border-strong, #3c494c)" : "1px solid var(--border, #2d3139)",
                  borderRadius: "8px",
                  padding: "1.25rem",
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "space-between",
                  gap: "1rem",
                  transition: "border-color 0.2s ease, box-shadow 0.2s ease",
                }}
              >
                <div className="stack stack--gap-sm">
                  {/* Card Header */}
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "0.5rem" }}>
                    <div>
                      <h3 style={{ margin: 0, fontSize: "1.1rem", fontWeight: 600, color: "var(--text, #dde4e5)" }}>
                        {p.display_name}
                      </h3>
                      <code style={{ fontSize: "11px", color: "var(--primary, #22d3ee)", opacity: 0.85 }}>
                        {p.name}
                      </code>
                    </div>

                    <span
                      className="mono"
                      style={{
                        fontSize: "11px",
                        fontWeight: 700,
                        padding: "2px 6px",
                        borderRadius: "4px",
                        color: isEnabled ? "#10b981" : "var(--text-muted, #859397)",
                        background: isEnabled ? "rgba(16, 185, 129, 0.12)" : "rgba(133, 147, 151, 0.1)",
                        border: `1px solid ${isEnabled ? "rgba(16, 185, 129, 0.3)" : "rgba(133, 147, 151, 0.2)"}`,
                      }}
                    >
                      {isEnabled ? "[ENABLED]" : "[STANDBY]"}
                    </span>
                  </div>

                  {/* Description */}
                  <p style={{ margin: "0.25rem 0", fontSize: "13px", color: "var(--text-muted, #859397)", minHeight: "38px", lineHeight: 1.4 }}>
                    {p.description}
                  </p>

                  {/* Metadata & Hooks */}
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: "12px", borderTop: "1px dashed var(--border, #2d3139)", paddingTop: "0.6rem" }}>
                    <span style={{ color: "var(--text-muted, #859397)" }}>Category:</span>
                    <span className="mono" style={{ textTransform: "uppercase", fontSize: "11px", color: "var(--text, #dde4e5)", fontWeight: 600 }}>
                      {p.category}
                    </span>
                  </div>

                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: "12px", gap: "0.5rem" }}>
                    <span style={{ color: "var(--text-muted, #859397)", whiteSpace: "nowrap", flexShrink: 0 }}>Hooks:</span>
                    <div style={{ display: "flex", gap: "4px", flexWrap: "wrap", justifyContent: "flex-end" }}>
                      {p.hooks.map((h) => (
                        <span
                          key={h}
                          className="mono"
                          style={{
                            fontSize: "10px",
                            color: "var(--primary-soft, #8aebff)",
                            background: "rgba(34, 211, 238, 0.08)",
                            border: "1px solid rgba(34, 211, 238, 0.2)",
                            padding: "1px 5px",
                            borderRadius: "3px",
                            whiteSpace: "nowrap",
                          }}
                        >
                          [{h.toUpperCase()}]
                        </span>
                      ))}
                    </div>
                  </div>

                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", fontSize: "12px" }}>
                    <span style={{ color: "var(--text-muted, #859397)" }}>Active Bindings:</span>
                    <span className="mono" style={{ fontWeight: 700, color: isEnabled ? "var(--emerald, #10b981)" : "var(--text-muted, #859397)" }}>
                      {activeCount} {activeCount === 1 ? "scope" : "scopes"}
                    </span>
                  </div>
                </div>

                {/* Card Actions */}
                <div
                  style={{
                    borderTop: "1px solid var(--border, #2d3139)",
                    paddingTop: "0.75rem",
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                  }}
                >
                  <button
                    type="button"
                    className="btn btn--sm btn--ghost"
                    style={{ fontSize: "12px", padding: "4px 8px", color: "var(--text-muted, #859397)" }}
                    onClick={() => {
                      setScopeFilter("all");
                      setStatusFilter("all");
                      setSearch(p.name);
                      setActiveTab("bindings");
                    }}
                  >
                    View Bindings ({activeCount})
                  </button>

                  <button
                    type="button"
                    className="btn btn--sm btn--primary"
                    style={{ fontSize: "12px", padding: "4px 12px" }}
                    onClick={() => {
                      setEditingBinding(null);
                      setSelectedPlugin(p);
                      setModalOpen(true);
                    }}
                  >
                    + Bind to Scope
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Tab 2: Full Bindings Management Table */}
      {activeTab === "bindings" && !loading && (
        <div
          style={{
            background: "var(--surface-raised, #181a20)",
            border: "1px solid var(--border, #2d3139)",
            borderRadius: "8px",
            overflow: "hidden",
          }}
        >
          <div style={{ overflowX: "auto" }}>
            <table className="table" style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--border, #2d3139)", background: "var(--surface-high, #1f2229)", textAlign: "left" }}>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Plugin</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Scope Tier</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Target Entity</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Config Summary</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Priority</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)" }}>Status</th>
                  <th style={{ padding: "0.75rem 1rem", fontFamily: "var(--font-mono)", fontSize: "11px", textTransform: "uppercase", color: "var(--text-muted, #859397)", textAlign: "right" }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredBindings.length === 0 ? (
                  <tr>
                    <td colSpan={7} style={{ textAlign: "center", padding: "3rem", color: "var(--text-muted, #859397)", fontFamily: "var(--font-mono)" }}>
                      No plugin bindings match the selected criteria. Click &quot;+ Bind Plugin&quot; to attach one.
                    </td>
                  </tr>
                ) : (
                  filteredBindings.map((b) => (
                    <tr key={b.id} style={{ borderBottom: "1px solid var(--border, #2d3139)" }}>
                      <td style={{ padding: "0.75rem 1rem" }}>
                        <strong style={{ color: "var(--text, #dde4e5)" }}>{b.plugin_display_name || b.plugin_name}</strong>
                        <div className="mono" style={{ fontSize: "11px", color: "var(--primary, #22d3ee)", opacity: 0.85 }}>
                          {b.plugin_name}
                        </div>
                      </td>
                      <td style={{ padding: "0.75rem 1rem" }}>
                        <span className="mono" style={{ textTransform: "uppercase", fontSize: "11px", fontWeight: 700, color: "var(--text-muted, #859397)" }}>
                          {b.scope_level}
                        </span>
                      </td>
                      <td style={{ padding: "0.75rem 1rem" }}>{renderScopeTarget(b)}</td>
                      <td style={{ padding: "0.75rem 1rem", maxWidth: 280 }}>
                        <pre
                          className="mono"
                          style={{
                            margin: 0,
                            fontSize: "11px",
                            whiteSpace: "pre-wrap",
                            background: "rgba(0, 0, 0, 0.25)",
                            border: "1px solid var(--border, #2d3139)",
                            padding: "0.25rem 0.5rem",
                            borderRadius: "4px",
                            maxHeight: 60,
                            overflow: "hidden",
                            color: "var(--text-muted, #859397)",
                          }}
                        >
                          {JSON.stringify(b.config)}
                        </pre>
                      </td>
                      <td style={{ padding: "0.75rem 1rem" }} className="mono">
                        <span style={{ color: "var(--primary, #22d3ee)", fontWeight: 600 }}>{b.priority}</span>
                      </td>
                      <td style={{ padding: "0.75rem 1rem" }}>
                        <button
                          type="button"
                          className="mono"
                          style={{
                            cursor: "pointer",
                            fontSize: "11px",
                            fontWeight: 700,
                            padding: "2px 8px",
                            borderRadius: "4px",
                            color: b.is_active ? "#10b981" : "var(--text-muted, #859397)",
                            background: b.is_active ? "rgba(16, 185, 129, 0.12)" : "rgba(133, 147, 151, 0.1)",
                            border: `1px solid ${b.is_active ? "rgba(16, 185, 129, 0.3)" : "rgba(133, 147, 151, 0.2)"}`,
                          }}
                          onClick={() => handleToggleBindingActive(b)}
                          title="Click to toggle status"
                        >
                          {b.is_active ? "[ACTIVE]" : "[DISABLED]"}
                        </button>
                      </td>
                      <td style={{ padding: "0.75rem 1rem", textAlign: "right" }}>
                        <div style={{ display: "flex", gap: "0.5rem", justifyContent: "flex-end" }}>
                          <button
                            type="button"
                            className="btn btn--sm btn--ghost"
                            onClick={() => {
                              setEditingBinding(b);
                              const pl = plugins.find((p) => p.name === b.plugin_name) || null;
                              setSelectedPlugin(pl);
                              setModalOpen(true);
                            }}
                          >
                            Edit
                          </button>
                          <button
                            type="button"
                            className="btn btn--sm btn--danger"
                            onClick={() => setDeleteTarget(b)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Modal: Bind or Edit */}
      {modalOpen && (
        <PluginBindingModal
          plugin={selectedPlugin}
          allPlugins={plugins}
          binding={editingBinding}
          onClose={() => setModalOpen(false)}
          onSuccess={() => {
            setModalOpen(false);
            loadData();
          }}
        />
      )}

      {/* Delete confirmation modal */}
      {deleteTarget && (
        <Modal
          title="Delete Plugin Binding"
          onClose={() => setDeleteTarget(null)}
          maxWidth={460}
        >
          <div className="stack stack--gap-md">
            <p>
              Are you sure you want to remove the binding for{" "}
              <strong>{deleteTarget.plugin_display_name || deleteTarget.plugin_name}</strong> on scope{" "}
              <strong>{deleteTarget.scope_level}</strong>?
            </p>
            <p style={{ fontSize: "0.85rem", color: "var(--text-muted, #64748b)" }}>
              Requests matching this scope will no longer execute this plugin.
            </p>

            <div style={{ display: "flex", justifyContent: "flex-end", gap: "0.75rem", marginTop: "1rem" }}>
              <button
                type="button"
                className="btn btn--outline"
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn--danger"
                onClick={handleDeleteBinding}
                disabled={deleting}
              >
                {deleting ? "Deleting..." : "Confirm Delete"}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
