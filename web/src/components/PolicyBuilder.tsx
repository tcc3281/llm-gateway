import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ActionDef, PolicyDocument, PolicyStatement } from "../api/types";

export const POLICY_TEMPLATES = [
  {
    name: "Full Access",
    desc: "Unrestricted access to all models and administrative resources",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "FullAccess",
          Effect: "Allow",
          Action: ["*:*", "*"],
          Resource: ["*"],
        },
      ],
    },
  },
  {
    name: "Developer Chat (Standard)",
    desc: "Allows standard chat models; denies expensive models (opus, o1)",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "AllowStandardChat",
          Effect: "Allow",
          Action: ["model:use", "model:stream", "model:list"],
          Resource: ["model:gpt-4o", "model:gpt-4o-mini", "model:claude-3-5-sonnet", "model:gemini-*"],
        },
        {
          Sid: "DenyExpensiveModels",
          Effect: "Deny",
          Action: ["model:use", "model:stream"],
          Resource: ["model:claude-3-opus", "model:o1-preview", "model:o1"],
        },
      ],
    },
  },
  {
    name: "Data & Embeddings Team",
    desc: "Permits vector embeddings only; blocks all generative chat models",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "AllowEmbeddings",
          Effect: "Allow",
          Action: ["model:embed", "model:list"],
          Resource: ["model:text-embedding-*", "model:embeddings-*"],
        },
        {
          Sid: "DenyGenerativeChat",
          Effect: "Deny",
          Action: ["model:use", "model:stream"],
          Resource: ["model:gpt-*", "model:claude-*", "model:gemini-*"],
        },
      ],
    },
  },
  {
    name: "Read-Only Auditor",
    desc: "View analytics, logs, and catalogs; no model inference or mutations",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "ReadOnlyAudit",
          Effect: "Allow",
          Action: ["analytics:read", "log:read", "model:list", "audit:read"],
          Resource: ["*"],
        },
        {
          Sid: "DenyModelUsageAndMutations",
          Effect: "Deny",
          Action: ["model:use", "model:stream", "key:create", "user:create", "group:create"],
          Resource: ["*"],
        },
      ],
    },
  },
];

const DEFAULT_ACTIONS: ActionDef[] = [
  { action: "model:use", namespace: "model", label: "Chat / Completion", description: "Generate completions" },
  { action: "model:stream", namespace: "model", label: "Streaming Response", description: "Receive SSE streams" },
  { action: "model:embed", namespace: "model", label: "Embeddings", description: "Generate vector embeddings" },
  { action: "model:list", namespace: "model", label: "List Models", description: "View available models" },
  { action: "model:image", namespace: "model", label: "Image Gen", description: "Multimodal image gen" },
  { action: "model:audio", namespace: "model", label: "Audio & Transcribe", description: "Whisper & TTS" },

  { action: "analytics:read", namespace: "analytics", label: "View Analytics", description: "Inspect dashboards & charts" },
  { action: "log:read", namespace: "log", label: "View Usage Logs", description: "Browse request logs" },
  { action: "audit:read", namespace: "audit", label: "View Audit Trail", description: "Inspect security logs" },

  { action: "key:create", namespace: "key", label: "Create Keys", description: "Issue API keys" },
  { action: "key:read", namespace: "key", label: "View Keys", description: "List API keys" },
  { action: "key:delete", namespace: "key", label: "Revoke Keys", description: "Revoke API keys" },

  { action: "user:create", namespace: "user", label: "Create Users", description: "Add users" },
  { action: "user:read", namespace: "user", label: "View Users", description: "List users" },
  { action: "user:update", namespace: "user", label: "Edit Users", description: "Modify user roles" },
  { action: "user:delete", namespace: "user", label: "Delete Users", description: "Remove users" },

  { action: "group:read", namespace: "group", label: "View Groups", description: "View IAM groups" },
  { action: "group:manage_members", namespace: "group", label: "Manage Members", description: "Add/remove group members" },

  { action: "provider:read", namespace: "provider", label: "View Providers", description: "Inspect providers" },
  { action: "provider:update", namespace: "provider", label: "Update Providers", description: "Edit provider credentials" },
];

interface VisualStatement {
  sid: string;
  effect: "Allow" | "Deny";
  actions: string[];
  resourceMode: "all" | "custom";
  resources: string[];
}

interface PolicyBuilderProps {
  initialName?: string;
  initialPriority?: number;
  initialPolicy?: any;
  onSave: (data: { name: string; priority: number; policy: PolicyDocument }) => Promise<void>;
  onCancel: () => void;
  isSubmitting?: boolean;
}

function parseToVisualStatements(doc: any): VisualStatement[] {
  if (!doc || !Array.isArray(doc.Statement) || doc.Statement.length === 0) {
    return [
      {
        sid: "Statement1",
        effect: "Allow",
        actions: ["model:use", "model:stream", "model:list"],
        resourceMode: "all",
        resources: ["*"],
      },
    ];
  }

  return doc.Statement.map((stmt: PolicyStatement, idx: number) => {
    const rawActions = Array.isArray(stmt.Action) ? stmt.Action : [stmt.Action || "*"];
    const rawResources = Array.isArray(stmt.Resource) ? stmt.Resource : [stmt.Resource || "*"];
    const isAll = rawResources.length === 1 && rawResources[0] === "*";

    return {
      sid: stmt.Sid || `Statement${idx + 1}`,
      effect: (stmt.Effect === "Deny" ? "Deny" : "Allow") as "Allow" | "Deny",
      actions: rawActions,
      resourceMode: isAll ? "all" : "custom",
      resources: isAll ? [] : rawResources,
    };
  });
}

function convertVisualToDoc(statements: VisualStatement[]): PolicyDocument {
  return {
    Version: "1",
    Statement: statements.map((s) => ({
      Sid: s.sid || undefined,
      Effect: s.effect,
      Action: s.actions.length > 0 ? s.actions : ["*"],
      Resource: s.resourceMode === "all" || s.resources.length === 0 ? ["*"] : s.resources,
    })),
  };
}

export default function PolicyBuilder({
  initialName = "",
  initialPriority = 10,
  initialPolicy,
  onSave,
  onCancel,
  isSubmitting = false,
}: PolicyBuilderProps) {
  const [activeTab, setActiveTab] = useState<"visual" | "json">("visual");
  const [name, setName] = useState(initialName);
  const [priority, setPriority] = useState(initialPriority);
  const [statements, setStatements] = useState<VisualStatement[]>(() =>
    parseToVisualStatements(initialPolicy)
  );
  const [jsonText, setJsonText] = useState(() => {
    if (initialPolicy) {
      return typeof initialPolicy === "string"
        ? initialPolicy
        : JSON.stringify(initialPolicy, null, 2);
    }
    return JSON.stringify(POLICY_TEMPLATES[1].policy, null, 2);
  });
  const [catalogActions, setCatalogActions] = useState<ActionDef[]>(DEFAULT_ACTIONS);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [customResourceInput, setCustomResourceInput] = useState<{ [stmtIdx: number]: string }>({});
  const [error, setError] = useState<string | null>(null);

  // Fetch catalog actions and registered models
  useEffect(() => {
    api
      .getIAMActions()
      .then((actions) => {
        if (actions && actions.length > 0) setCatalogActions(actions);
      })
      .catch(() => {
        /* fallback to DEFAULT_ACTIONS */
      });

    api
      .listModels()
      .then((res) => {
        if (Array.isArray(res)) {
          setAvailableModels(res.map((m) => m.model_name));
        }
      })
      .catch(() => {});
  }, []);

  // Sync to JSON when switching to JSON tab
  const handleSwitchTab = (tab: "visual" | "json") => {
    if (tab === "json" && activeTab === "visual") {
      const doc = convertVisualToDoc(statements);
      setJsonText(JSON.stringify(doc, null, 2));
    } else if (tab === "visual" && activeTab === "json") {
      try {
        const parsed = JSON.parse(jsonText);
        setStatements(parseToVisualStatements(parsed));
        setError(null);
      } catch {
        setError("Invalid JSON format. Please fix JSON syntax before switching to Visual Builder.");
        return;
      }
    }
    setActiveTab(tab);
  };

  const applyTemplate = (tmpl: (typeof POLICY_TEMPLATES)[0]) => {
    setName(tmpl.name);
    setStatements(parseToVisualStatements(tmpl.policy));
    setJsonText(JSON.stringify(tmpl.policy, null, 2));
    setError(null);
  };

  const updateStatement = (idx: number, patch: Partial<VisualStatement>) => {
    setStatements((prev) => {
      const copy = [...prev];
      copy[idx] = { ...copy[idx], ...patch };
      return copy;
    });
  };

  const addStatement = () => {
    setStatements((prev) => [
      ...prev,
      {
        sid: `Statement${prev.length + 1}`,
        effect: "Allow",
        actions: ["model:use"],
        resourceMode: "all",
        resources: [],
      },
    ]);
  };

  const removeStatement = (idx: number) => {
    if (statements.length <= 1) return;
    setStatements((prev) => prev.filter((_, i) => i !== idx));
  };

  const toggleAction = (stmtIdx: number, actionStr: string) => {
    const stmt = statements[stmtIdx];
    const exists = stmt.actions.includes(actionStr);
    const updated = exists
      ? stmt.actions.filter((a) => a !== actionStr)
      : [...stmt.actions, actionStr];
    updateStatement(stmtIdx, { actions: updated });
  };

  const toggleNamespaceActions = (stmtIdx: number, namespace: string) => {
    const nsActions = catalogActions.filter((a) => a.namespace === namespace).map((a) => a.action);
    const stmt = statements[stmtIdx];
    const allSelected = nsActions.every((a) => stmt.actions.includes(a));

    let updated: string[];
    if (allSelected) {
      updated = stmt.actions.filter((a) => !nsActions.includes(a));
    } else {
      updated = Array.from(new Set([...stmt.actions, ...nsActions]));
    }
    updateStatement(stmtIdx, { actions: updated });
  };

  const addResource = (stmtIdx: number, resPattern: string) => {
    const trimmed = resPattern.trim();
    if (!trimmed) return;
    const formatted = trimmed.startsWith("model:") || trimmed === "*" ? trimmed : `model:${trimmed}`;
    const stmt = statements[stmtIdx];
    if (!stmt.resources.includes(formatted)) {
      updateStatement(stmtIdx, { resources: [...stmt.resources, formatted] });
    }
    setCustomResourceInput((prev) => ({ ...prev, [stmtIdx]: "" }));
  };

  const removeResource = (stmtIdx: number, resPattern: string) => {
    const stmt = statements[stmtIdx];
    updateStatement(stmtIdx, {
      resources: stmt.resources.filter((r) => r !== resPattern),
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError("Policy Name is required");
      return;
    }

    let finalDoc: PolicyDocument;
    if (activeTab === "json") {
      try {
        finalDoc = JSON.parse(jsonText);
      } catch {
        setError("Invalid JSON format. Please verify syntax.");
        return;
      }
    } else {
      finalDoc = convertVisualToDoc(statements);
    }

    try {
      setError(null);
      await onSave({
        name: name.trim(),
        priority,
        policy: finalDoc,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save policy");
    }
  };

  // Group actions by namespace
  const namespaces = Array.from(new Set(catalogActions.map((a) => a.namespace)));

  return (
    <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {/* Template Quick Picker */}
      <div>
        <div className="form-label" style={{ marginBottom: 6, fontSize: 12 }}>
          Quick Templates:
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
          {POLICY_TEMPLATES.map((tmpl) => (
            <button
              key={tmpl.name}
              type="button"
              className="btn btn--ghost btn--sm"
              style={{ fontSize: 11, padding: "3px 8px" }}
              onClick={() => applyTemplate(tmpl)}
              title={tmpl.desc}
            >
              ✦ {tmpl.name}
            </button>
          ))}
        </div>
      </div>

      {/* Basic Metadata */}
      <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 12 }}>
        <div className="form-group">
          <label className="form-label">Policy Name *</label>
          <input
            className="input"
            required
            placeholder="e.g. StandardDeveloperAccess"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="form-group">
          <label className="form-label">Priority (Higher runs first)</label>
          <input
            className="input"
            type="number"
            value={priority}
            onChange={(e) => setPriority(parseInt(e.target.value, 10) || 0)}
          />
        </div>
      </div>

      {/* Tab Switcher: Visual Builder vs Raw JSON */}
      <div
        style={{
          display: "flex",
          borderBottom: "1px solid var(--border)",
          gap: 8,
          marginBottom: 4,
        }}
      >
        <button
          type="button"
          onClick={() => handleSwitchTab("visual")}
          style={{
            background: "none",
            border: "none",
            borderBottom: activeTab === "visual" ? "2px solid var(--primary)" : "2px solid transparent",
            color: activeTab === "visual" ? "var(--primary)" : "var(--text-muted)",
            fontWeight: activeTab === "visual" ? 600 : 400,
            padding: "8px 14px",
            cursor: "pointer",
            fontSize: 13,
            display: "flex",
            alignItems: "center",
            gap: 6,
          }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
            checklist
          </span>
          Visual Policy Builder
        </button>
        <button
          type="button"
          onClick={() => handleSwitchTab("json")}
          style={{
            background: "none",
            border: "none",
            borderBottom: activeTab === "json" ? "2px solid var(--primary)" : "2px solid transparent",
            color: activeTab === "json" ? "var(--primary)" : "var(--text-muted)",
            fontWeight: activeTab === "json" ? 600 : 400,
            padding: "8px 14px",
            cursor: "pointer",
            fontSize: 13,
            display: "flex",
            alignItems: "center",
            gap: 6,
          }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
            data_object
          </span>
          Raw JSON Editor
        </button>
      </div>

      {error && <div className="error-banner">{error}</div>}

      {/* Tab 1: Visual Policy Builder */}
      {activeTab === "visual" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {statements.map((stmt, sIdx) => {
            const isDeny = stmt.effect === "Deny";
            return (
              <div
                key={sIdx}
                style={{
                  border: `1px solid ${isDeny ? "rgba(248, 113, 113, 0.4)" : "var(--border)"}`,
                  borderRadius: 8,
                  padding: 16,
                  background: isDeny ? "rgba(248, 113, 113, 0.04)" : "var(--surface-high)",
                  display: "flex",
                  flexDirection: "column",
                  gap: 14,
                }}
              >
                {/* Statement Header */}
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <span className="mono" style={{ fontSize: 12, fontWeight: 700, color: "var(--text-muted)" }}>
                      #{sIdx + 1}
                    </span>
                    <input
                      className="input"
                      style={{ width: 200, padding: "4px 8px", fontSize: 12 }}
                      placeholder="Statement Identifier (Sid)"
                      value={stmt.sid}
                      onChange={(e) => updateStatement(sIdx, { sid: e.target.value })}
                    />
                  </div>

                  <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                    {/* Effect Switcher */}
                    <div
                      style={{
                        display: "flex",
                        borderRadius: 6,
                        overflow: "hidden",
                        border: "1px solid var(--border)",
                      }}
                    >
                      <button
                        type="button"
                        style={{
                          border: "none",
                          padding: "4px 12px",
                          fontSize: 11,
                          fontWeight: 700,
                          cursor: "pointer",
                          background: stmt.effect === "Allow" ? "var(--emerald)" : "transparent",
                          color: stmt.effect === "Allow" ? "#000" : "var(--text-muted)",
                        }}
                        onClick={() => updateStatement(sIdx, { effect: "Allow" })}
                      >
                        ALLOW
                      </button>
                      <button
                        type="button"
                        style={{
                          border: "none",
                          padding: "4px 12px",
                          fontSize: 11,
                          fontWeight: 700,
                          cursor: "pointer",
                          background: stmt.effect === "Deny" ? "var(--danger)" : "transparent",
                          color: stmt.effect === "Deny" ? "#fff" : "var(--text-muted)",
                        }}
                        onClick={() => updateStatement(sIdx, { effect: "Deny" })}
                      >
                        DENY (Override)
                      </button>
                    </div>

                    {statements.length > 1 && (
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        style={{ color: "var(--danger)", padding: "4px 8px" }}
                        onClick={() => removeStatement(sIdx)}
                        title="Remove Statement"
                      >
                        <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                          delete
                        </span>
                      </button>
                    )}
                  </div>
                </div>

                {/* Actions Selector */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 8, color: "var(--text)" }}>
                    Permitted / Denied Actions:
                  </div>
                  <div
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 10,
                      background: "rgba(0,0,0,0.15)",
                      padding: 12,
                      borderRadius: 6,
                    }}
                  >
                    {namespaces.map((ns) => {
                      const nsActions = catalogActions.filter((a) => a.namespace === ns);
                      const allChecked = nsActions.every((a) => stmt.actions.includes(a.action));

                      return (
                        <div key={ns} style={{ borderBottom: "1px solid rgba(255,255,255,0.05)", paddingBottom: 8 }}>
                          <div
                            style={{
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "space-between",
                              marginBottom: 6,
                            }}
                          >
                            <span
                              className="mono"
                              style={{
                                fontSize: 11,
                                fontWeight: 700,
                                textTransform: "uppercase",
                                color: "var(--primary)",
                              }}
                            >
                              {ns} actions
                            </span>
                            <button
                              type="button"
                              className="btn btn--ghost btn--sm"
                              style={{ fontSize: 10, padding: "1px 6px" }}
                              onClick={() => toggleNamespaceActions(sIdx, ns)}
                            >
                              {allChecked ? "Deselect All" : "Select All"}
                            </button>
                          </div>

                          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(210px, 1fr))", gap: 6 }}>
                            {nsActions.map((act) => {
                              const checked = stmt.actions.includes(act.action) || stmt.actions.includes("*");
                              return (
                                <label
                                  key={act.action}
                                  style={{
                                    display: "flex",
                                    alignItems: "flex-start",
                                    gap: 8,
                                    fontSize: 12,
                                    cursor: "pointer",
                                    background: checked ? "rgba(99, 102, 241, 0.1)" : "transparent",
                                    padding: "4px 6px",
                                    borderRadius: 4,
                                  }}
                                >
                                  <input
                                    type="checkbox"
                                    checked={checked}
                                    onChange={() => toggleAction(sIdx, act.action)}
                                    style={{ marginTop: 2 }}
                                  />
                                  <div>
                                    <div style={{ fontWeight: checked ? 600 : 400 }}>{act.label}</div>
                                    <div className="mono muted" style={{ fontSize: 10 }}>
                                      {act.action}
                                    </div>
                                  </div>
                                </label>
                              );
                            })}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>

                {/* Resource Target Selector */}
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>Target Resources (Models):</div>
                  <div style={{ display: "flex", gap: 16, alignItems: "center", marginBottom: 8 }}>
                    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, cursor: "pointer" }}>
                      <input
                        type="radio"
                        name={`resMode_${sIdx}`}
                        checked={stmt.resourceMode === "all"}
                        onChange={() => updateStatement(sIdx, { resourceMode: "all" })}
                      />
                      <span>All Resources (<code>*</code>)</span>
                    </label>

                    <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, cursor: "pointer" }}>
                      <input
                        type="radio"
                        name={`resMode_${sIdx}`}
                        checked={stmt.resourceMode === "custom"}
                        onChange={() => updateStatement(sIdx, { resourceMode: "custom" })}
                      />
                      <span>Specific Models / Patterns</span>
                    </label>
                  </div>

                  {stmt.resourceMode === "custom" && (
                    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                      {/* Active Resource Chips */}
                      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                        {stmt.resources.length === 0 && (
                          <div className="muted" style={{ fontSize: 11, fontStyle: "italic" }}>
                            No model pattern added yet. Add model patterns below (e.g. model:gpt-4o, model:claude-*).
                          </div>
                        )}
                        {stmt.resources.map((res) => (
                          <span
                            key={res}
                            className="mono"
                            style={{
                              fontSize: 11,
                              background: "rgba(99, 102, 241, 0.15)",
                              color: "var(--primary)",
                              border: "1px solid rgba(99, 102, 241, 0.3)",
                              padding: "2px 8px",
                              borderRadius: 4,
                              display: "inline-flex",
                              alignItems: "center",
                              gap: 6,
                            }}
                          >
                            {res}
                            <button
                              type="button"
                              onClick={() => removeResource(sIdx, res)}
                              style={{
                                background: "none",
                                border: "none",
                                cursor: "pointer",
                                color: "var(--text-muted)",
                                padding: 0,
                                display: "flex",
                                alignItems: "center",
                              }}
                            >
                              <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                                close
                              </span>
                            </button>
                          </span>
                        ))}
                      </div>

                      {/* Custom Input + Model Quick Add Chips */}
                      <div style={{ display: "flex", gap: 8 }}>
                        <input
                          className="input mono"
                          style={{ flex: 1, fontSize: 12 }}
                          placeholder="e.g. model:gpt-4o, model:claude-*, model:gemini-1.5-pro"
                          value={customResourceInput[sIdx] || ""}
                          onChange={(e) =>
                            setCustomResourceInput((prev) => ({ ...prev, [sIdx]: e.target.value }))
                          }
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault();
                              addResource(sIdx, customResourceInput[sIdx] || "");
                            }
                          }}
                        />
                        <button
                          type="button"
                          className="btn btn--secondary btn--sm"
                          onClick={() => addResource(sIdx, customResourceInput[sIdx] || "")}
                        >
                          Add Pattern
                        </button>
                      </div>

                      {/* Quick-add available models */}
                      {availableModels.length > 0 && (
                        <div style={{ display: "flex", flexWrap: "wrap", gap: 4, alignItems: "center" }}>
                          <span className="muted" style={{ fontSize: 10 }}>
                            Quick add:
                          </span>
                          {availableModels.slice(0, 8).map((m) => (
                            <button
                              key={m}
                              type="button"
                              className="btn btn--ghost btn--sm mono"
                              style={{ fontSize: 10, padding: "1px 6px" }}
                              onClick={() => addResource(sIdx, `model:${m}`)}
                            >
                              +{m}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            );
          })}

          <button
            type="button"
            className="btn btn--ghost btn--sm"
            style={{ alignSelf: "flex-start", gap: 6 }}
            onClick={addStatement}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
              add_circle
            </span>
            Add Another Statement
          </button>
        </div>
      )}

      {/* Tab 2: Raw JSON Editor */}
      {activeTab === "json" && (
        <div className="form-group">
          <label className="form-label">IAM JSON Document</label>
          <textarea
            className="input mono"
            style={{ height: 320, fontSize: 12, lineHeight: 1.4 }}
            required
            value={jsonText}
            onChange={(e) => setJsonText(e.target.value)}
          />
        </div>
      )}

      {/* Form Action Buttons */}
      <div className="modal-actions" style={{ marginTop: 8 }}>
        <button type="button" className="btn btn--ghost" onClick={onCancel} disabled={isSubmitting}>
          Cancel
        </button>
        <button type="submit" className="btn btn--primary" disabled={isSubmitting}>
          {isSubmitting ? "Saving..." : "Save Policy"}
        </button>
      </div>
    </form>
  );
}
