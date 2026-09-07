import { FormEvent, useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { User } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";

interface UserForm {
  name: string;
  email: string;
  role: string;
  is_root: boolean;
}

const empty: UserForm = { name: "", email: "", role: "user", is_root: false };

export default function Users() {
  const navigate = useNavigate();
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<UserForm>(empty);

  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [editForm, setEditForm] = useState<UserForm>(empty);

  const [passwordTarget, setPasswordTarget] = useState<User | null>(null);
  const [newPassword, setNewPassword] = useState("");
  const [passwordSuccess, setPasswordSuccess] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      setUsers(await api.listUsers());
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
      await api.createUser(form);
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
              <th>Role</th>
              <th>Created</th>
              <th style={{ textAlign: "right" }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>{u.name || "—"}</td>
                <td className="mono" style={{ fontSize: 12 }}>
                  {u.email}
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
                      style={{ color: "#22d3ee" }}
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
                        setEditForm({ name: u.name, email: u.email, role: u.role, is_root: Boolean(u.is_root) });
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
            ))}
            {!users.length && (
              <tr>
                <td colSpan={5}>
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
          <UserFields form={form} setForm={setForm} />
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
          <UserFields form={editForm} setForm={setEditForm} emailDisabled />
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
    </div>
  );
}

function UserFields({
  form,
  setForm,
  emailDisabled,
}: {
  form: UserForm;
  setForm: (f: UserForm) => void;
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
        <label className="field__label">Role</label>
        <select
          className="select"
          value={form.role}
          onChange={(e) => setForm({ ...form, role: e.target.value })}
        >
          <option value="user">user</option>
          <option value="admin">admin</option>
        </select>
      </div>
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
