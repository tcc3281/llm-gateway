import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

const NAV = [
  { to: "/dashboard", label: "Overview", icon: "◉" },
  { to: "/users", label: "Users", icon: "◈" },
  { to: "/keys", label: "API Keys", icon: "⌗" },
  { to: "/providers", label: "Providers", icon: "⬡" },
  { to: "/models", label: "Models & Pricing", icon: "§" },
  { to: "/logs", label: "Usage Logs", icon: "≡" },
  { to: "/settings", label: "Settings", icon: "⚙" },
];

export default function Layout() {
  const { logout } = useAuth();
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
          <div>
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
              <span className="nav-item__icon">{n.icon}</span>
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar__foot">
          <span>v0.1.0</span>
          <button className="btn btn--ghost btn--sm" onClick={handleLogout}>
            Logout
          </button>
        </div>
      </aside>
      <div className="app__body">
        <header className="topbar">
          <span className="topbar__title">
            {current?.label ?? "Overview"}
          </span>
          <div className="topbar__spacer" />
          <span className="mono muted" style={{ fontSize: 11 }}>
            [ADMIN :8081]
          </span>
        </header>
        <main className="page">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
