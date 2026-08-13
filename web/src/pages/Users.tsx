import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { User } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";

interface UserForm {
  name: string;
  email: string;
  role: string;
}

const empty: UserForm = { name: "", email: "", role: "user" };

export default function Users() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<UserForm>(empty);

  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [editForm, setEditForm] = useState<UserForm>(empty);

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

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">Users</h1>
          <p className="page__sub">
            {users.length} account(s). Deleting a user also revokes their API keys.
          </p>
        </div>
        <button className="btn btn--primary" onClick={() => setCreateOpen(true)}>
          + New user
        </button>
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
                  {u.role === "admin" ? (
                    <span className="badge badge--violet">admin</span>
                  ) : (
                    <span className="badge">user</span>
                  )}
                </td>
                <td className="muted">{fmtDate(u.created_at)}</td>
                <td>
                  <div className="table__actions">
                    <button
                      className="btn btn--sm"
                      onClick={() => {
                        setEditTarget(u);
                        setEditForm({ name: u.name, email: u.email, role: u.role });
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
    </>
  );
}
