import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

const NAV = [
  { to: "/dashboard", label: "Overview", icon: "dashboard" },
  { to: "/people", label: "People & Access", icon: "shield_person" },
  { to: "/keys", label: "API Keys", icon: "key" },
  { to: "/providers", label: "Providers", icon: "hub" },
  { to: "/models", label: "Models & Pricing", icon: "payments" },
  { to: "/alerts", label: "Alerts & Hooks", icon: "notifications_active" },
  { to: "/logs", label: "Usage Logs", icon: "list_alt" },
  { to: "/audit-logs", label: "Audit Trail", icon: "history" },
  { to: "/settings", label: "Settings", icon: "settings" },
];

export default function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const current = NAV.find((n) => pathname.startsWith(n.to));

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar__brand">
          <div className="sidebar__logo">GW</div>
          <div className="sidebar__brand-text">
            <div className="sidebar__title">LLM Gateway</div>
            <div className="sidebar__subtitle">Admin Ops</div>
          </div>
        </div>
        <nav className="sidebar__nav">
          {NAV.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.to === "/dashboard"}
              className={({ isActive }) =>
                `nav-item${isActive ? " active" : ""}`
              }
            >
              <span className="material-symbols-outlined nav-item__icon">{n.icon}</span>
              <span className="nav-item__text">{n.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="sidebar__foot" style={{ flexDirection: "column", alignItems: "stretch", gap: "8px" }}>
          {user && (
            <div style={{ fontSize: "11px", fontFamily: "JetBrains Mono, monospace", color: "#859397", borderTop: "1px solid #2d3139", paddingTop: "8px" }}>
              <div style={{ color: user.is_root ? "#22d3ee" : "#10b981", fontWeight: 700 }}>
                {user.name || user.email}
              </div>
              <div style={{ fontSize: "10px", color: "#64748b" }}>
                {user.is_root ? "[ROOT ADMIN]" : `[${user.role.toUpperCase()}]`}
              </div>
            </div>
          )}
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <span>v1.0.0</span>
            <button className="btn btn--ghost btn--sm" onClick={handleLogout}>
              Logout
            </button>
          </div>
        </div>
      </aside>
      <div className="app__body">
        <header className="topbar">
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--text-muted)" }}>
            <span>System</span>
            <span className="material-symbols-outlined" style={{ fontSize: 14 }}>chevron_right</span>
            <span style={{ color: "var(--text)", fontWeight: 600 }}>
              {current?.label ?? "Overview"}
            </span>
          </div>
          <div className="topbar__spacer" />
          {user && (
            <div
              className={`terminal-badge ${user.is_root ? "terminal-badge--root glow-amber" : user.role === "admin" ? "terminal-badge--admin" : "terminal-badge--user"}`}
              style={{ fontSize: 12, padding: "3px 8px" }}
            >
              <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                {user.is_root ? "admin_panel_settings" : "account_circle"}
              </span>
              <span>
                {user.is_root ? "[ROOT ADMIN]" : `[${user.role.toUpperCase()}]`} {user.email}
              </span>
            </div>
          )}
          <span className="mono muted" style={{ fontSize: 11, border: "1px solid var(--border)", padding: "2px 6px", borderRadius: 4 }}>
            :8081
          </span>
        </header>
        <main className="page">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
