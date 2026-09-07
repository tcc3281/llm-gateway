import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Group, GroupPolicy, User } from "../api/types";
import { fmtDate, fmtMoney, fmtNum } from "../lib/format";
import Modal from "../components/Modal";

const POLICY_TEMPLATES = [
  {
    name: "Full Access",
    desc: "Unrestricted access to all gateway models and management resources",
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
    desc: "Allows standard chat models; denies expensive models like opus/128k",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "AllowStandardChat",
          Effect: "Allow",
          Action: ["model:use", "model:list"],
          Resource: ["model:gpt-4o", "model:gpt-4o-mini", "model:claude-3-5-sonnet", "model:gemini-*"],
        },
        {
          Sid: "DenyExpensiveModels",
          Effect: "Deny",
          Action: ["model:use"],
          Resource: ["model:gpt-4o-128k", "model:claude-3-opus", "model:o1-preview"],
        },
      ],
    },
  },
  {
    name: "Data & Embeddings Team",
    desc: "Permits text embedding models only; blocks all generative chat models",
    policy: {
      Version: "1",
      Statement: [
        {
          Sid: "AllowEmbeddings",
          Effect: "Allow",
          Action: ["model:use", "model:list"],
          Resource: ["model:text-embedding-*", "model:embeddings-*"],
        },
        {
          Sid: "DenyChatModels",
          Effect: "Deny",
          Action: ["model:use"],
          Resource: ["model:gpt-*", "model:claude-*", "model:gemini-*"],
        },
      ],
    },
  },
  {
    name: "Read-Only Auditor",
    desc: "View analytics, logs, and catalogs; no model usage or mutations permitted",
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
          Sid: "DenyModelUsage",
          Effect: "Deny",
          Action: ["model:use", "key:create", "user:create"],
          Resource: ["*"],
        },
      ],
    },
  },
];

export default function Groups() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [allUsers, setAllUsers] = useState<User[]>([]);
  const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
  const [policies, setPolicies] = useState<GroupPolicy[]>([]);
  const [members, setMembers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Group Create Modal
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [groupName, setGroupName] = useState("");
  const [groupDesc, setGroupDesc] = useState("");
  const [groupBudget, setGroupBudget] = useState("");
  const [groupRPM, setGroupRPM] = useState("");

  // Policy Modal
  const [policyModalOpen, setPolicyModalOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<GroupPolicy | null>(null);
  const [policyName, setPolicyName] = useState("");
  const [policyJSON, setPolicyJSON] = useState("");
  const [policyPriority, setPolicyPriority] = useState(0);
  const [policyError, setPolicyError] = useState<string | null>(null);

  // Add Member Modal
  const [memberModalOpen, setMemberModalOpen] = useState(false);
  const [selectedUserId, setSelectedUserId] = useState("");

  const loadGroups = async () => {
    try {
      setLoading(true);
      const [gData, uData] = await Promise.all([api.listGroups(), api.listUsers()]);
      setGroups(gData);
      setAllUsers(uData);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load groups");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadGroups();
  }, []);

  const loadGroupDetails = async (g: Group) => {
    try {
      setSelectedGroup(g);
      setDetailLoading(true);
      const [pData, mData] = await Promise.all([
        api.listGroupPolicies(g.id),
        api.listGroupMembers(g.id),
      ]);
      setPolicies(pData);
      setMembers(mData);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load group details");
    } finally {
      setDetailLoading(false);
    }
  };

  const handleCreateGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const budget = parseFloat(groupBudget) || 0;
      const rpm = parseInt(groupRPM, 10) || 0;
      await api.createGroup({
        name: groupName.trim(),
        description: groupDesc.trim(),
        budget_limit_monthly: budget,
        rate_limit_rpm: rpm,
        is_active: true,
      });
      setCreateModalOpen(false);
      setGroupName("");
      setGroupDesc("");
      setGroupBudget("");
      setGroupRPM("");
      await loadGroups();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to create group");
    }
  };

  const handleDeleteGroup = async (id: string, name: string) => {
    if (!confirm(`Are you sure you want to delete group "${name}"? Policies and member associations will be removed.`)) {
      return;
    }
    try {
      await api.deleteGroup(id);
      if (selectedGroup?.id === id) {
        setSelectedGroup(null);
      }
      await loadGroups();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete group");
    }
  };

  const handleSavePolicy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedGroup) return;

    setPolicyError(null);
    let parsed: any;
    try {
      parsed = JSON.parse(policyJSON);
    } catch {
      setPolicyError("Invalid JSON syntax. Please check brackets, quotes, and commas.");
      return;
    }

    try {
      if (editingPolicy) {
        await api.updateGroupPolicy(selectedGroup.id, editingPolicy.id, {
          name: policyName.trim(),
          policy: parsed,
          priority: policyPriority,
          is_active: true,
        });
      } else {
        await api.createGroupPolicy(selectedGroup.id, {
          name: policyName.trim(),
          policy: parsed,
          priority: policyPriority,
          is_active: true,
        });
      }
      setPolicyModalOpen(false);
      setEditingPolicy(null);
      setPolicyName("");
      setPolicyJSON("");
      setPolicyPriority(0);
      await loadGroupDetails(selectedGroup);
      await loadGroups();
    } catch (err) {
      setPolicyError(err instanceof Error ? err.message : "Failed to save policy");
    }
  };

  const handleDeletePolicy = async (policyId: string) => {
    if (!selectedGroup) return;
    if (!confirm("Are you sure you want to remove this IAM policy?")) return;
    try {
      await api.deleteGroupPolicy(selectedGroup.id, policyId);
      await loadGroupDetails(selectedGroup);
      await loadGroups();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete policy");
    }
  };

  const handleAddMember = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedGroup || !selectedUserId) return;
    try {
      await api.addGroupMember(selectedGroup.id, selectedUserId);
      setMemberModalOpen(false);
      setSelectedUserId("");
      await loadGroupDetails(selectedGroup);
      await loadGroups();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to add member");
    }
  };

  const handleRemoveMember = async (userId: string) => {
    if (!selectedGroup) return;
    if (!confirm("Remove this user from the group?")) return;
    try {
      await api.removeGroupMember(selectedGroup.id, userId);
      await loadGroupDetails(selectedGroup);
      await loadGroups();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to remove member");
    }
  };

  const openCreatePolicy = () => {
    setEditingPolicy(null);
    setPolicyName("");
    setPolicyJSON(JSON.stringify(POLICY_TEMPLATES[1].policy, null, 2));
    setPolicyPriority(10);
    setPolicyError(null);
    setPolicyModalOpen(true);
  };

  const openEditPolicy = (p: GroupPolicy) => {
    setEditingPolicy(p);
    setPolicyName(p.name);
    setPolicyJSON(typeof p.policy === "string" ? p.policy : JSON.stringify(p.policy, null, 2));
    setPolicyPriority(p.priority);
    setPolicyError(null);
    setPolicyModalOpen(true);
  };

  const loadTemplate = (tmpl: typeof POLICY_TEMPLATES[0]) => {
    setPolicyName(tmpl.name);
    setPolicyJSON(JSON.stringify(tmpl.policy, null, 2));
  };

  const availableUsersToAdd = allUsers.filter(
    (u) => !members.some((m) => m.id === u.id)
  );

  return (
    <div className="page__content">
      <div className="page__head" style={{ marginBottom: 20 }}>
        <div>
          <h1 className="page__title">IAM Groups & Policies</h1>
          <p className="page__desc">
            Organize users into teams, pool monthly budgets, and evaluate AWS IAM-style JSON policies with Deny-overrides-Allow logic.
          </p>
        </div>
        <button className="btn btn--primary" onClick={() => setCreateModalOpen(true)}>
          <span className="material-symbols-outlined" style={{ fontSize: 18 }}>add</span>
          New Group
        </button>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div style={{ display: "grid", gridTemplateColumns: selectedGroup ? "1fr 1.3fr" : "1fr", gap: 20, alignItems: "start" }}>
        {/* Groups List Table */}
        <div className="card">
          <div className="card__head">
            <h2 className="card__title">Groups ({groups.length})</h2>
          </div>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Group Name</th>
                  <th>Monthly Budget</th>
                  <th>Members</th>
                  <th>Policies</th>
                  <th>Status</th>
                  <th style={{ textAlign: "right" }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={6} className="empty">Loading IAM groups…</td>
                  </tr>
                ) : groups.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="empty">No IAM groups configured. Click "New Group" to create one.</td>
                  </tr>
                ) : (
                  groups.map((g) => {
                    const isSelected = selectedGroup?.id === g.id;
                    return (
                      <tr
                        key={g.id}
                        style={{ cursor: "pointer", background: isSelected ? "var(--surface-high)" : undefined }}
                        onClick={() => loadGroupDetails(g)}
                      >
                        <td>
                          <div style={{ fontWeight: 600, color: isSelected ? "var(--primary)" : "var(--text)" }}>
                            {g.name}
                          </div>
                          {g.description && (
                            <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
                              {g.description}
                            </div>
                          )}
                        </td>
                        <td className="mono">
                          {g.budget_limit_monthly > 0 ? fmtMoney(g.budget_limit_monthly) : "Unlimited"}
                        </td>
                        <td className="mono">{g.member_count ?? 0}</td>
                        <td className="mono">{g.policy_count ?? 0}</td>
                        <td>
                          <span
                            className="mono"
                            style={{
                              fontSize: 11,
                              padding: "2px 6px",
                              borderRadius: 4,
                              background: g.is_active ? "rgba(16, 185, 129, 0.1)" : "rgba(248, 113, 113, 0.1)",
                              color: g.is_active ? "var(--emerald)" : "var(--danger)",
                              border: `1px solid ${g.is_active ? "rgba(16, 185, 129, 0.3)" : "rgba(248, 113, 113, 0.3)"}`,
                            }}
                          >
                            {g.is_active ? "ACTIVE" : "INACTIVE"}
                          </span>
                        </td>
                        <td style={{ textAlign: "right" }} onClick={(e) => e.stopPropagation()}>
                          <button
                            className="btn btn--ghost btn--sm"
                            style={{ color: "var(--danger)" }}
                            onClick={() => handleDeleteGroup(g.id, g.name)}
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>

        {/* Selected Group Inspection & Policy Editor */}
        {selectedGroup && (
          <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
            {/* Header / Summary Card */}
            <div className="card">
              <div className="card__head">
                <div>
                  <span className="mono" style={{ fontSize: 11, color: "var(--primary)", textTransform: "uppercase" }}>
                    Group Details
                  </span>
                  <h2 className="card__title">{selectedGroup.name}</h2>
                </div>
                <button className="btn btn--ghost btn--sm" onClick={() => setSelectedGroup(null)}>
                  Close
                </button>
              </div>
              <div style={{ padding: 16, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }} className="mono text-sm">
                <div>
                  <span className="muted">Monthly Ceiling:</span>{" "}
                  <strong>{selectedGroup.budget_limit_monthly > 0 ? fmtMoney(selectedGroup.budget_limit_monthly) : "Unlimited"}</strong>
                </div>
                <div>
                  <span className="muted">Rate Limit RPM:</span>{" "}
                  <strong>{selectedGroup.rate_limit_rpm > 0 ? `${selectedGroup.rate_limit_rpm} RPM` : "Key Default"}</strong>
                </div>
                <div>
                  <span className="muted">Created:</span> {fmtDate(selectedGroup.created_at)}
                </div>
                <div>
                  <span className="muted">ID:</span> <span style={{ fontSize: 11 }}>{selectedGroup.id}</span>
                </div>
              </div>
            </div>

            {/* IAM Policies Card */}
            <div className="card">
              <div className="card__head">
                <div>
                  <h3 className="card__title" style={{ fontSize: 15 }}>Attached IAM Policies ({policies.length})</h3>
                  <p style={{ fontSize: 12, color: "var(--text-muted)", margin: 0 }}>
                    Statements are evaluated top-down; explicit Deny blocks access immediately.
                  </p>
                </div>
                <button className="btn btn--secondary btn--sm" onClick={openCreatePolicy}>
                  <span className="material-symbols-outlined" style={{ fontSize: 16 }}>add</span>
                  Add Policy
                </button>
              </div>

              <div style={{ padding: 16 }}>
                {detailLoading ? (
                  <div className="empty">Loading policies…</div>
                ) : policies.length === 0 ? (
                  <div className="empty">
                    No IAM policies attached. Users in this group will be denied model access (implicit deny).
                  </div>
                ) : (
                  <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                    {policies.map((p) => {
                      const stmtCount = Array.isArray(p.policy?.Statement) ? p.policy.Statement.length : 0;
                      return (
                        <div
                          key={p.id}
                          style={{
                            border: "1px solid var(--border)",
                            borderRadius: 6,
                            padding: 12,
                            background: "var(--surface-high)",
                          }}
                        >
                          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
                            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                              <strong>{p.name}</strong>
                              <span className="mono" style={{ fontSize: 11, padding: "1px 6px", borderRadius: 3, background: "rgba(168, 85, 247, 0.15)", color: "var(--violet)" }}>
                                Priority {p.priority}
                              </span>
                              <span className="mono" style={{ fontSize: 11, color: "var(--text-muted)" }}>
                                ({stmtCount} statement{stmtCount !== 1 ? "s" : ""})
                              </span>
                            </div>
                            <div style={{ display: "flex", gap: 6 }}>
                              <button className="btn btn--ghost btn--sm" onClick={() => openEditPolicy(p)}>
                                Edit JSON
                              </button>
                              <button className="btn btn--ghost btn--sm" style={{ color: "var(--danger)" }} onClick={() => handleDeletePolicy(p.id)}>
                                Remove
                              </button>
                            </div>
                          </div>

                          <pre
                            className="mono"
                            style={{
                              fontSize: 11,
                              background: "rgba(0,0,0,0.3)",
                              padding: 8,
                              borderRadius: 4,
                              maxHeight: 140,
                              overflowY: "auto",
                              color: "var(--text-muted)",
                            }}
                          >
                            {typeof p.policy === "string" ? p.policy : JSON.stringify(p.policy, null, 2)}
                          </pre>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>

            {/* Group Members Card */}
            <div className="card">
              <div className="card__head">
                <h3 className="card__title" style={{ fontSize: 15 }}>Group Members ({members.length})</h3>
                <button
                  className="btn btn--secondary btn--sm"
                  onClick={() => setMemberModalOpen(true)}
                  disabled={availableUsersToAdd.length === 0}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 16 }}>person_add</span>
                  Add User
                </button>
              </div>

              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th>User</th>
                      <th>Role</th>
                      <th>Root</th>
                      <th style={{ textAlign: "right" }}>Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {members.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="empty">No users assigned to this group.</td>
                      </tr>
                    ) : (
                      members.map((m) => (
                        <tr key={m.id}>
                          <td>
                            <div><strong>{m.name}</strong></div>
                            <div className="mono muted" style={{ fontSize: 11 }}>{m.email}</div>
                          </td>
                          <td className="mono">{m.role}</td>
                          <td>
                            {m.is_root && (
                              <span className="mono" style={{ fontSize: 11, color: "var(--amber)", fontWeight: 700 }}>
                                ROOT
                              </span>
                            )}
                          </td>
                          <td style={{ textAlign: "right" }}>
                            <button
                              className="btn btn--ghost btn--sm"
                              style={{ color: "var(--danger)" }}
                              onClick={() => handleRemoveMember(m.id)}
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
          </div>
        )}
      </div>

      {/* Create Group Modal */}
      {createModalOpen && (
        <Modal title="Create New IAM Group" onClose={() => setCreateModalOpen(false)}>
          <form onSubmit={handleCreateGroup}>
            <div className="form-group">
              <label className="form-label">Group Name *</label>
              <input
                className="input"
                required
                placeholder="e.g. AI Engineering Team"
                value={groupName}
                onChange={(e) => setGroupName(e.target.value)}
              />
            </div>
            <div className="form-group">
              <label className="form-label">Description</label>
              <input
                className="input"
                placeholder="e.g. Core product ML engineers and developers"
                value={groupDesc}
                onChange={(e) => setGroupDesc(e.target.value)}
              />
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
              <div className="form-group">
                <label className="form-label">Monthly Budget Ceiling (USD)</label>
                <input
                  className="input"
                  type="number"
                  step="0.01"
                  placeholder="0 = unlimited"
                  value={groupBudget}
                  onChange={(e) => setGroupBudget(e.target.value)}
                />
              </div>
              <div className="form-group">
                <label className="form-label">Rate Limit RPM Override</label>
                <input
                  className="input"
                  type="number"
                  placeholder="0 = use key limit"
                  value={groupRPM}
                  onChange={(e) => setGroupRPM(e.target.value)}
                />
              </div>
            </div>

            <div className="modal-actions" style={{ marginTop: 20 }}>
              <button type="button" className="btn btn--ghost" onClick={() => setCreateModalOpen(false)}>
                Cancel
              </button>
              <button type="submit" className="btn btn--primary">
                Create Group
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* Policy Editor Modal */}
      {policyModalOpen && (
        <Modal
          title={editingPolicy ? `Edit Policy: ${editingPolicy.name}` : `Attach Policy to ${selectedGroup?.name}`}
          onClose={() => setPolicyModalOpen(false)}
        >
          <form onSubmit={handleSavePolicy}>
            {/* Template Selector */}
            <div style={{ marginBottom: 14 }}>
              <label className="form-label">Insert Policy Template:</label>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                {POLICY_TEMPLATES.map((tmpl) => (
                  <button
                    key={tmpl.name}
                    type="button"
                    className="btn btn--ghost btn--sm"
                    style={{ fontSize: 11 }}
                    onClick={() => loadTemplate(tmpl)}
                  >
                    {tmpl.name}
                  </button>
                ))}
              </div>
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 12 }}>
              <div className="form-group">
                <label className="form-label">Policy Name *</label>
                <input
                  className="input"
                  required
                  placeholder="e.g. AllowDeveloperModels"
                  value={policyName}
                  onChange={(e) => setPolicyName(e.target.value)}
                />
              </div>
              <div className="form-group">
                <label className="form-label">Priority (higher runs first)</label>
                <input
                  className="input"
                  type="number"
                  value={policyPriority}
                  onChange={(e) => setPolicyPriority(parseInt(e.target.value, 10) || 0)}
                />
              </div>
            </div>

            <div className="form-group">
              <label className="form-label">IAM JSON Document *</label>
              <textarea
                className="input mono"
                style={{ height: 260, fontSize: 12, lineHeight: 1.4 }}
                required
                value={policyJSON}
                onChange={(e) => setPolicyJSON(e.target.value)}
              />
            </div>

            {policyError && <div className="error-banner" style={{ marginBottom: 12 }}>{policyError}</div>}

            <div className="modal-actions">
              <button type="button" className="btn btn--ghost" onClick={() => setPolicyModalOpen(false)}>
                Cancel
              </button>
              <button type="submit" className="btn btn--primary">
                Save Policy
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* Add Member Modal */}
      {memberModalOpen && (
        <Modal title={`Add User to ${selectedGroup?.name}`} onClose={() => setMemberModalOpen(false)}>
          <form onSubmit={handleAddMember}>
            <div className="form-group">
              <label className="form-label">Select User to Assign:</label>
              <select
                className="input"
                required
                value={selectedUserId}
                onChange={(e) => setSelectedUserId(e.target.value)}
              >
                <option value="">-- Choose a user --</option>
                {availableUsersToAdd.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name} ({u.email}) - {u.role}
                  </option>
                ))}
              </select>
            </div>

            <div className="modal-actions" style={{ marginTop: 20 }}>
              <button type="button" className="btn btn--ghost" onClick={() => setMemberModalOpen(false)}>
                Cancel
              </button>
              <button type="submit" className="btn btn--primary" disabled={!selectedUserId}>
                Add to Group
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
