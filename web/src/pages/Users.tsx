import { FormEvent, useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Tenant, User } from "../api/types";
import { fmtDate } from "../lib/format";
import Modal from "../components/Modal";
import UserDetailModal from "../components/UserDetailModal";
import { useToast } from "../components/Toast";
import { useLanguage } from "../i18n";

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
  const { t } = useLanguage();
  const navigate = useNavigate();
  const toast = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const [users, setUsers] = useState<User[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Search and filter state
  const [searchQuery, setSearchQuery] = useState("");
  const [filterTenant, setFilterTenant] = useState("all");
  const [filterRole, setFilterRole] = useState("all");

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
      toast.success(`User ${form.email} created successfully`, "[200 OK]");
      setCreateOpen(false);
      setForm(empty);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Create failed";
      setError(msg);
      toast.error(msg, "[CREATE FAILED]");
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
      toast.success(`User ${editForm.email || editTarget.email} updated`, "[200 OK]");
      setEditTarget(null);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Update failed";
      setError(msg);
      toast.error(msg, "[UPDATE FAILED]");
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
      toast.info(`User ${deleteTarget.email} deleted`, "[200 OK]");
      setDeleteTarget(null);
      await load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed";
      setError(msg);
      toast.error(msg, "[DELETE FAILED]");
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
      const msg = `Password updated for ${passwordTarget.email}`;
      setPasswordSuccess(msg);
      toast.success(msg, "[200 OK]");
      setNewPassword("");
      setTimeout(() => {
        setPasswordTarget(null);
        setPasswordSuccess(null);
      }, 1500);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to update password";
      setError(msg);
      toast.error(msg, "[AUTH ERROR]");
    } finally {
      setBusy(false);
    }
  };

  const filteredUsers = users.filter((u) => {
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim();
      const matchName = (u.name || "").toLowerCase().includes(q);
      const matchEmail = u.email.toLowerCase().includes(q);
      const matchId = u.id.toLowerCase().includes(q);
      if (!matchName && !matchEmail && !matchId) return false;
    }
    if (filterTenant !== "all") {
      if (filterTenant === "none" && u.tenant_id) return false;
      if (filterTenant !== "none" && u.tenant_id !== filterTenant) return false;
    }
    if (filterRole !== "all") {
      if (filterRole === "root" && !u.is_root) return false;
      if (filterRole === "admin" && (u.role !== "admin" || u.is_root)) return false;
      if (filterRole === "user" && (u.role !== "user" || u.is_root)) return false;
    }
    return true;
  });

  return (
    <div>
      <div className="page__header">
        <div>
          <h1 className="page__title">{t("people.tab_users", "Users & Accounts")}</h1>
          <p className="page__sub">
            {filteredUsers.length} / {users.length} {t("people.tab_users_desc")}
          </p>
        </div>
        <div style={{ display: "flex", gap: "10px" }}>
          <button
            className="btn"
            style={{ backgroundColor: "#1a2122", border: "1px solid #2d3139", color: "#22d3ee", fontFamily: "JetBrains Mono, monospace", fontSize: "12px", display: "flex", alignItems: "center", gap: "6px" }}
            onClick={() => navigate("/people?tab=analytics")}
          >
            <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>insights</span>
            {t("people.tab_analytics", "Member Analytics")}
          </button>
          <button className="btn btn--primary" onClick={() => setCreateOpen(true)}>
            {t("users.create_btn", "+ Add User")}
          </button>
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      {/* Toolbar Filter */}
      <div className="filter-bar filter-bar--inline">
        <div className="search-box">
          <span className="material-symbols-outlined">search</span>
          <input
            type="text"
            className="input"
            style={{ fontFamily: "var(--font-mono)", fontSize: 13 }}
            placeholder={t("users.search_placeholder", "Search members, emails, or IDs...")}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
        <select
          className="select"
          style={{ width: "auto", minWidth: 170, fontFamily: "var(--font-mono)", fontSize: 12 }}
          value={filterTenant}
          onChange={(e) => setFilterTenant(e.target.value)}
        >
          <option value="all">{t("users.filter_all_tenants", "All Tenants")}</option>
          <option value="none">{t("users.filter_global", "Global (No Tenant)")}</option>
          {tenants.map((tItem) => (
            <option key={tItem.id} value={tItem.id}>
              {tItem.name} ({tItem.type.toUpperCase()})
            </option>
          ))}
        </select>
        <select
          className="select"
          style={{ width: "auto", minWidth: 140, fontFamily: "var(--font-mono)", fontSize: 12 }}
          value={filterRole}
          onChange={(e) => setFilterRole(e.target.value)}
        >
          <option value="all">{t("users.filter_all_roles", "All Roles")}</option>
          <option value="root">{t("users.filter_root", "Root Admin")}</option>
          <option value="admin">{t("users.filter_admin", "Tenant Admin")}</option>
          <option value="user">{t("users.filter_user", "Standard User")}</option>
        </select>
      </div>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>{t("users.col_name", "MEMBER")}</th>
              <th>{t("users.col_tenant", "TENANT / ORG")}</th>
              <th>{t("users.col_role", "SYSTEM ROLE")}</th>
              <th>{t("users.col_created", "CREATED")}</th>
              <th style={{ textAlign: "right" }}>{t("users.col_actions", "ACTIONS")}</th>
            </tr>
          </thead>
          <tbody>
            {filteredUsers.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ textAlign: "center", padding: "32px", color: "var(--text-muted)" }}>
                  {t("users.no_members", "No members match the selected search or filters.")}
                </td>
              </tr>
            ) : (
              filteredUsers.map((u) => {
                const matchedTenant = tenants.find((item) => item.id === u.tenant_id);
                return (
                  <tr key={u.id}>
                    <td>
                      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <div
                          style={{
                            width: 32,
                            height: 32,
                            borderRadius: "var(--radius-sm)",
                            background: u.is_root
                              ? "rgba(245, 158, 11, 0.15)"
                              : u.role === "admin"
                              ? "rgba(16, 185, 129, 0.15)"
                              : "rgba(34, 211, 238, 0.15)",
                            border: u.is_root
                              ? "1px solid rgba(245, 158, 11, 0.4)"
                              : u.role === "admin"
                              ? "1px solid rgba(16, 185, 129, 0.4)"
                              : "1px solid rgba(34, 211, 238, 0.4)",
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            fontWeight: 700,
                            fontSize: 13,
                            fontFamily: "var(--font-mono)",
                            color: u.is_root ? "var(--amber)" : u.role === "admin" ? "var(--emerald)" : "var(--primary)",
                          }}
                        >
                          {(u.name || u.email).charAt(0).toUpperCase()}
                        </div>
                        <div>
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
                            title="View member deep-dive"
                          >
                            {u.name || "—"}
                          </button>
                          <div className="mono muted" style={{ fontSize: 11 }}>
                            {u.email}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td>
                      {matchedTenant ? (
                        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                            <span
                              className={`terminal-badge ${matchedTenant.type === "internal" ? "terminal-badge--internal" : "terminal-badge--external"}`}
                            >
                              [{matchedTenant.type.toUpperCase()}]
                            </span>
                            <span style={{ fontWeight: 600, fontSize: 12 }}>{matchedTenant.name}</span>
                          </div>
                          <span className="muted mono" style={{ fontSize: 10 }}>
                            ({u.tenant_role || "member"})
                          </span>
                        </div>
                      ) : (
                        <span className="terminal-badge" style={{ color: "var(--text-muted)", borderColor: "var(--border)" }}>
                          [GLOBAL]
                        </span>
                      )}
                    </td>
                    <td>
                      {u.is_root ? (
                        <span className="terminal-badge terminal-badge--root glow-amber">
                          [ROOT]
                        </span>
                      ) : u.role === "admin" ? (
                        <span className="terminal-badge terminal-badge--admin">
                          [ADMIN]
                        </span>
                      ) : (
                        <span className="terminal-badge terminal-badge--user">
                          [USER]
                        </span>
                      )}
                    </td>
                    <td className="muted mono" style={{ fontSize: 11 }}>{fmtDate(u.created_at)}</td>
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
                        {t("users.details_btn", "Details")}
                      </button>
                      <button
                        className="btn btn--sm"
                        onClick={() => {
                          setPasswordTarget(u);
                          setNewPassword("");
                          setPasswordSuccess(null);
                        }}
                      >
                        {t("users.password_btn", "Password")}
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
                        {t("users.edit_btn", "Edit")}
                      </button>
                      <button
                        className="btn btn--sm btn--danger"
                        onClick={() => setDeleteTarget(u)}
                      >
                        {t("users.delete_btn", "Delete")}
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

      {createOpen && (
        <Modal
          title={t("users.create_title", "New user")}
          onClose={() => setCreateOpen(false)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setCreateOpen(false)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={submitCreate}
                disabled={busy || !form.name.trim() || !form.email.trim()}
              >
                {busy ? t("common.creating", "Creating…") : t("common.create", "Create")}
              </button>
            </>
          }
        >
          <UserFields form={form} setForm={setForm} tenants={tenants} />
        </Modal>
      )}

      {editTarget && (
        <Modal
          title={t("users.edit_title", "Edit user")}
          onClose={() => setEditTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setEditTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={submitEdit}
                disabled={busy || !editForm.name.trim()}
              >
                {busy ? t("common.saving", "Saving…") : t("common.save", "Save")}
              </button>
            </>
          }
        >
          <UserFields form={editForm} setForm={setEditForm} tenants={tenants} emailDisabled />
        </Modal>
      )}

      {passwordTarget && (
        <Modal
          title={`${t("users.pwd_modal_title", "Set Password for")}: ${passwordTarget.email}`}
          onClose={() => setPasswordTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setPasswordTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button
                className="btn btn--primary"
                onClick={submitPassword}
                disabled={busy || newPassword.length < 6}
              >
                {busy ? t("common.saving", "Updating…") : t("users.reset_pwd", "Update Password")}
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
            <label className="field__label">{t("users.pwd_field_label", "New Password (min 6 characters)")}</label>
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
            {t("users.pwd_field_hint", "Setting a password allows this member to authenticate directly via Member Login.")}
          </p>
        </Modal>
      )}

      {deleteTarget && (
        <Modal
          title={t("users.delete_title", "Delete user")}
          onClose={() => setDeleteTarget(null)}
          footer={
            <>
              <button className="btn btn--ghost" onClick={() => setDeleteTarget(null)}>
                {t("common.cancel", "Cancel")}
              </button>
              <button className="btn btn--danger" onClick={confirmDelete} disabled={busy}>
                {busy ? t("common.deleting", "Deleting…") : t("common.delete", "Delete")}
              </button>
            </>
          }
        >
          <p>
            {t("users.delete_confirm", "Delete")} <strong>{deleteTarget.email}</strong>? {t("users.delete_confirm_desc", "Their API keys will be revoked (cascade). Usage logs are kept.")}
          </p>
        </Modal>
      )}

      {selectedUser && (
        <UserDetailModal
          user={selectedUser}
          tenant={tenants.find((item) => item.id === selectedUser.tenant_id)}
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
  const { t } = useLanguage();
  return (
    <>
      <div className="field">
        <label className="field__label">{t("users.field_name", "Name")}</label>
        <input
          className="input"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="Dev 1"
        />
      </div>
      <div className="field">
        <label className="field__label">{t("users.field_email", "Email")}</label>
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
        <label className="field__label">{t("users.field_role", "System Role")}</label>
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
        <label className="field__label">{t("users.field_tenant", "Tenant Organization")}</label>
        <select
          className="select"
          value={form.tenant_id}
          onChange={(e) => setForm({ ...form, tenant_id: e.target.value })}
        >
          <option value="">{t("users.global_access", "None (Global Access)")}</option>
          {tenants.map((tItem) => (
            <option key={tItem.id} value={tItem.id}>
              {tItem.name} ({tItem.type === "internal" ? "Internal" : "B2B Client"})
            </option>
          ))}
        </select>
      </div>

      {form.tenant_id && (
        <div className="field">
          <label className="field__label">{t("users.field_tenant_role", "Tenant Role")}</label>
          <select
            className="select"
            value={form.tenant_role}
            onChange={(e) => setForm({ ...form, tenant_role: e.target.value })}
          >
            <option value="member">{t("users.role_member", "Member")}</option>
            <option value="tenant_admin">{t("users.role_manager", "Tenant Admin (Manager)")}</option>
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
            {t("users.field_root", "Root Administrator (IAM Bypass)")}
          </span>
        </label>
        <div style={{ fontSize: 11, color: "var(--text-muted)", marginTop: 4, marginLeft: 22 }}>
          {t("users.field_root_hint", "Root users bypass all IAM group and model permission checks.")}
        </div>
      </div>
    </>
  );
}
