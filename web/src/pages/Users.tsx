import { FormEvent, useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Tenant, User } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";
import UserDetailModal from "../components/UserDetailModal";

interface UserForm {
  name: string;
  email: string;
  role: string;
  is_root: boolean;
  tenant_id: string;
  tenant_role: string;
}

const empty: UserForm = { name: "", email: "", role: "user", is_root: false, tenant_id: "", tenant_role: "member" };

export default function Users() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [users, setUsers] = useState<User[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<UserForm>(empty);

  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [editForm, setEditForm] = useState<UserForm>(empty);

  const [passwordTarget, setPasswordTarget] = useState<User | null>(null);
  const [newPassword, setNewPassword] = useState("");
  const [passwordSuccess, setPasswordSuccess] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);

  const userIdParam = searchParams.get("user_id");

  useEffect(() => {
    if (userIdParam && users.length > 0) {
      const match = users.find((u) => u.id === userIdParam);
      if (match) setSelectedUser(match);
    }
  }, [userIdParam, users]);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [uList, tList] = await Promise.all([
        api.listUsers(),
        api.listTenants().catch(() => []),
      ]);
      setUsers(uList);
      setTenants(tList);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load users");
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim() || !form.email.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.createUser({
        name: form.name,
        email: form.email,
        role: form.role,
        is_root: form.is_root,
        tenant_id: form.tenant_id ? form.tenant_id : null,
        tenant_role: form.tenant_role || "member",
      });
      setCreateOpen(false);
      setForm(empty);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Create failed");
    } finally {
      setBusy(false);
    }
  };

  const submitEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editTarget) return;
    setBusy(true);
    setError(null);
    try {
      await api.updateUser(editTarget.id, {
        name: editForm.name,
        role: editForm.role,
        is_root: editForm.is_root,
        tenant_id: editForm.tenant_id ? editForm.tenant_id : null,
        tenant_role: editForm.tenant_role || "member",
      });
      setEditTarget(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setError(null);
    try {
      await api.deleteUser(deleteTarget.id);
      setDeleteTarget(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setBusy(false);
    }
  };

  const submitPassword = async (e: FormEvent) => {
    e.preventDefault();
    if (!passwordTarget || !newPassword.trim()) return;
    setBusy(true);
    setError(null);
    setPasswordSuccess(null);
    try {
      await api.setPassword(passwordTarget.id, newPassword.trim());
      setPasswordSuccess(`Password updated for ${passwordTarget.email}`);
      setNewPassword("");
      setTimeout(() => {
        setPasswordTarget(null);
        setPasswordSuccess(null);
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update password");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">Users &amp; Members</h1>
          <p className="page__sub">
            {users.length} account(s). Manage team credentials, passwords, and roles.
          </p>
        </div>
        <div style={{ display: "flex", gap: "10px" }}>
          <button
            className="btn"
            style={{ backgroundColor: "#1a2122", border: "1px solid #2d3139", color: "#22d3ee", fontFamily: "JetBrains Mono, monospace", fontSize: "12px", display: "flex", alignItems: "center", gap: "6px" }}
            onClick={() => navigate("/analytics/members")}
          >
            <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>insights</span>
            Member Analytics
          </button>
          <button className="btn btn--primary" onClick={() => setCreateOpen(true)}>
            + New user
          </button>
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Tenant</th>
              <th>Role</th>
              <th>Created</th>
              <th style={{ textAlign: "right" }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => {
              const matchedTenant = tenants.find((t) => t.id === u.tenant_id);
              return (
                <tr key={u.id}>
                  <td>
                    <button
                      type="button"
                      className="btn btn--ghost"
                      style={{
                        padding: 0,
                        minHeight: "unset",
                        fontWeight: 600,
                        color: "var(--text)",
                        cursor: "pointer",
                        textAlign: "left",
                        fontSize: 13,
                      }}
                      onClick={() => setSelectedUser(u)}
                      title="View user details & analytics"
                    >
                      {u.name || "—"}
                    </button>
                  </td>
                  <td className="mono" style={{ fontSize: 12 }}>
                    <button
                      type="button"
                      className="btn btn--ghost mono"
                      style={{
                        padding: 0,
                        minHeight: "unset",
                        color: "var(--primary)",
                        cursor: "pointer",
                        fontSize: 12,
                      }}
                      onClick={() => setSelectedUser(u)}
                      title="View user details & analytics"
                    >
                      {u.email}
                    </button>
                  </td>
                  <td>
                    {matchedTenant ? (
                      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                        <span
                          style={{
                            display: "inline-block",
                            padding: "2px 8px",
                            borderRadius: 4,
                            fontSize: 11,
                            fontWeight: 600,
                            background: matchedTenant.type === "internal" ? "rgba(99, 102, 241, 0.15)" : "rgba(16, 185, 129, 0.15)",
                            color: matchedTenant.type === "internal" ? "#818cf8" : "#34d399",
                          }}
                        >
                          {matchedTenant.name}
                        </span>
                        <span className="muted mono" style={{ fontSize: 10, textTransform: "uppercase" }}>
                          ({u.tenant_role || "member"})
                        </span>
                      </div>
                    ) : (
                      <span className="muted" style={{ fontSize: 12 }}>Global</span>
                    )}
                  </td>
                  <td>
                    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                      {u.role === "admin" ? (
                        <span className="badge badge--violet">admin</span>
                      ) : (
                        <span className="badge">user</span>
                      )}
                      {u.is_root && (
                        <span
                          className="mono"
                          style={{
                            fontSize: 10,
                            fontWeight: 700,
                            padding: "1px 5px",
                            borderRadius: 3,
                            background: "rgba(245, 158, 11, 0.15)",
                            color: "var(--amber)",
                            border: "1px solid rgba(245, 158, 11, 0.3)",
                          }}
                        >
                          ROOT
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="muted">{fmtDate(u.created_at)}</td>
                  <td>
                    <div className="table__actions">
                      <button
                        className="btn btn--sm"
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 4,
                          color: "#22d3ee",
                          background: "rgba(34, 211, 238, 0.1)",
                          border: "1px solid rgba(34, 211, 238, 0.3)",
                          fontWeight: 600,
                        }}
                        onClick={() => setSelectedUser(u)}
                        title="View user dashboard, keys, and logs"
                      >
                        <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                          insights
                        </span>
                        Details
                      </button>
                      <button
                        className="btn btn--sm"
                        onClick={() => {
                          setPasswordTarget(u);
                          setNewPassword("");
                          setPasswordSuccess(null);
                        }}
                      >
                        Password
                      </button>
                      <button
                        className="btn btn--sm"
                        onClick={() => {
                          setEditTarget(u);
                          setEditForm({
                            name: u.name,
                            email: u.email,
                            role: u.role,
                            is_root: Boolean(u.is_root),
                            tenant_id: u.tenant_id || "",
                            tenant_role: u.tenant_role || "member",
                          });
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className="btn btn--sm btn--danger"
                        onClick={() => setDeleteTarget(u)}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {!users.length && (
              <tr>
                <td colSpan={6}>
                  <div className="empty">No users yet</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {createOpen && (
        <Modal
          title="New user"
          onClose={() => setCreateOpen(false)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setCreateOpen(false)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={submitCreate}
                disabled={busy || !form.name.trim() || !form.email.trim()}
              >
                {busy ? "Creating…" : "Create"}
              </button>
            </>
          }
        >
          <UserFields form={form} setForm={setForm} tenants={tenants} />
        </Modal>
      )}

      {editTarget && (
        <Modal
          title="Edit user"
          onClose={() => setEditTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setEditTarget(null)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={submitEdit}
                disabled={busy || !editForm.name.trim()}
              >
                {busy ? "Saving…" : "Save"}
              </button>
            </>
          }
        >
          <UserFields form={editForm} setForm={setEditForm} tenants={tenants} emailDisabled />
        </Modal>
      )}

      {passwordTarget && (
        <Modal
          title={`Set Password: ${passwordTarget.email}`}
          onClose={() => setPasswordTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setPasswordTarget(null)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={submitPassword}
                disabled={busy || newPassword.length < 6}
              >
                {busy ? "Updating…" : "Update Password"}
              </button>
            </>
          }
        >
          {passwordSuccess && (
            <div style={{ padding: "8px 12px", backgroundColor: "rgba(16, 185, 129, 0.15)", border: "1px solid rgba(16, 185, 129, 0.4)", borderRadius: 4, color: "#10b981", fontSize: 13, marginBottom: 12 }}>
              {passwordSuccess}
            </div>
          )}
          <div className="field">
            <label className="field__label">New Password (min 6 characters)</label>
            <input
              className="input"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="••••••••••••"
              autoFocus
            />
          </div>
          <p className="muted" style={{ fontSize: 12, marginTop: 8 }}>
            Setting a password allows this member to authenticate directly via Member Login.
          </p>
        </Modal>
      )}

      {deleteTarget && (
        <Modal
          title="Delete user"
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
            Delete <strong>{deleteTarget.email}</strong>? Their API keys are
            revoked (cascade). Usage logs are kept.
          </p>
        </Modal>
      )}

      {selectedUser && (
        <UserDetailModal
          user={selectedUser}
          tenant={tenants.find((t) => t.id === selectedUser.tenant_id)}
          onClose={() => {
            setSelectedUser(null);
            if (searchParams.has("user_id")) {
              searchParams.delete("user_id");
              setSearchParams(searchParams);
            }
          }}
          onUserUpdated={load}
        />
      )}
    </div>
  );
}

function UserFields({
  form,
  setForm,
  tenants,
  emailDisabled,
}: {
  form: UserForm;
  setForm: (f: UserForm) => void;
  tenants: Tenant[];
  emailDisabled?: boolean;
}) {
  return (
    <>
      <div className="field">
        <label className="field__label">Name</label>
        <input
          className="input"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="Dev 1"
        />
      </div>
      <div className="field">
        <label className="field__label">Email</label>
        <input
          className="input"
          type="email"
          value={form.email}
          disabled={emailDisabled}
          onChange={(e) => setForm({ ...form, email: e.target.value })}
          placeholder="dev@abc.com"
        />
      </div>
      <div className="field">
        <label className="field__label">System Role</label>
        <select
          className="select"
          value={form.role}
          onChange={(e) => setForm({ ...form, role: e.target.value })}
        >
          <option value="user">user</option>
          <option value="admin">admin</option>
        </select>
      </div>

      <div className="field">
        <label className="field__label">Tenant Organization</label>
        <select
          className="select"
          value={form.tenant_id}
          onChange={(e) => setForm({ ...form, tenant_id: e.target.value })}
        >
          <option value="">None (Global Access)</option>
          {tenants.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name} ({t.type === "internal" ? "Internal" : "B2B Client"})
            </option>
          ))}
        </select>
      </div>

      {form.tenant_id && (
        <div className="field">
          <label className="field__label">Tenant Role</label>
          <select
            className="select"
            value={form.tenant_role}
            onChange={(e) => setForm({ ...form, tenant_role: e.target.value })}
          >
            <option value="member">Member</option>
            <option value="tenant_admin">Tenant Admin (Manager)</option>
          </select>
        </div>
      )}

      <div className="field" style={{ marginTop: 12 }}>
        <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={form.is_root}
            onChange={(e) => setForm({ ...form, is_root: e.target.checked })}
          />
          <span style={{ fontSize: 13, fontWeight: 600, color: "var(--text)" }}>
            Root Administrator (IAM Bypass)
          </span>
        </label>
        <div style={{ fontSize: 11, color: "var(--text-muted)", marginTop: 4, marginLeft: 22 }}>
          Root users bypass all IAM group and model permission checks.
        </div>
      </div>
    </>
  );
}
