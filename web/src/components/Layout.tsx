import { useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { useLanguage } from "../i18n";
import LanguageSwitcher from "./LanguageSwitcher";

const NAV_ITEMS = [
  { to: "/dashboard", key: "nav.overview", defaultLabel: "Overview", icon: "dashboard" },
  { to: "/people", key: "nav.people", defaultLabel: "People & Access", icon: "shield_person" },
  { to: "/keys", key: "nav.keys", defaultLabel: "API Keys", icon: "key" },
  { to: "/providers", key: "nav.providers", defaultLabel: "Providers", icon: "hub" },
  { to: "/models", key: "nav.models", defaultLabel: "Models & Pricing", icon: "payments" },
  { to: "/virtual-models", key: "nav.virtual_models", defaultLabel: "Virtual Models", icon: "alt_route" },
  { to: "/plugins", key: "nav.plugins", defaultLabel: "Plugins", icon: "extension" },
  { to: "/alerts", key: "nav.alerts", defaultLabel: "Alerts & Hooks", icon: "notifications_active" },
  { to: "/logs", key: "nav.logs", defaultLabel: "Usage Logs", icon: "list_alt" },
  { to: "/audit-logs", key: "nav.audit", defaultLabel: "Audit Trail", icon: "history" },
  { to: "/settings", key: "nav.settings", defaultLabel: "Settings", icon: "settings" },
];

export default function Layout() {
  const { user, logout } = useAuth();
  const { t } = useLanguage();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const current = NAV_ITEMS.find((n) => pathname.startsWith(n.to));
  const [copiedUrl, setCopiedUrl] = useState(false);

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
          {NAV_ITEMS.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.to === "/dashboard"}
              className={({ isActive }) =>
                `nav-item${isActive ? " active" : ""}`
              }
            >
              <span className="material-symbols-outlined nav-item__icon">{n.icon}</span>
              <span className="nav-item__text">{t(n.key, n.defaultLabel)}</span>
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
            <span className="mono muted" style={{ fontSize: 11 }}>v1.0.0</span>
            <button className="btn btn--ghost btn--sm" onClick={handleLogout}>
              {t("common.logout", "Logout")}
            </button>
          </div>
        </div>
      </aside>
      <div className="app__body">
        <header className="topbar">
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--text-muted)" }}>
            <span>{t("common.system", "System")}</span>
            <span className="material-symbols-outlined" style={{ fontSize: 14 }}>chevron_right</span>
            <span style={{ color: "var(--text)", fontWeight: 600 }}>
              {current ? t(current.key, current.defaultLabel) : t("nav.overview", "Overview")}
            </span>
          </div>

          <div className="topbar__spacer" />

          {/* Gateway Live Endpoint Pill */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              background: "rgba(34, 211, 238, 0.08)",
              border: "1px solid rgba(34, 211, 238, 0.25)",
              borderRadius: 6,
              padding: "3px 10px",
              fontSize: 12,
              fontFamily: "var(--font-mono, monospace)",
              color: "#22d3ee",
            }}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 14 }}>link</span>
            <span>https://vin-ai-t205.io.vn</span>
            <button
              type="button"
              onClick={() => {
                navigator.clipboard.writeText("https://vin-ai-t205.io.vn");
                setCopiedUrl(true);
                setTimeout(() => setCopiedUrl(false), 2000);
              }}
              style={{
                background: "transparent",
                border: "none",
                cursor: "pointer",
                color: copiedUrl ? "#10b981" : "#22d3ee",
                display: "inline-flex",
                alignItems: "center",
                padding: "0 2px",
              }}
              title="Copy Gateway Endpoint URL"
            >
              <span className="material-symbols-outlined" style={{ fontSize: 14 }}>
                {copiedUrl ? "check" : "content_copy"}
              </span>
            </button>
          </div>

          {/* Instant Language Switcher */}
          <LanguageSwitcher />

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
