import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { useLanguage } from "../i18n";
import LanguageSwitcher from "../components/LanguageSwitcher";

export default function Login() {
  const { t } = useLanguage();
  const { login, loginWithPassword } = useAuth();
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<"member" | "root">("member");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [rootKey, setRootKey] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleMemberSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await loginWithPassword(email.trim(), password);
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setBusy(false);
    }
  };

  const handleRootSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await login(rootKey.trim());
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Root authentication failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login" style={{ minHeight: "100vh", display: "flex", flexDirection: "column", justifyContent: "center", alignItems: "center", padding: "16px" }}>
      <div style={{ marginBottom: "16px", display: "flex", alignItems: "center", gap: "10px", flexWrap: "wrap", justifyContent: "center" }}>
        <span style={{ color: "#22d3ee", fontWeight: "bold", fontSize: "20px" }}>▮▮</span>
        <span style={{ fontFamily: "Inter, sans-serif", fontWeight: 700, fontSize: "22px", letterSpacing: "-0.02em", color: "#dde4e5", textTransform: "uppercase" }}>
          LLM Gateway
        </span>
        <span style={{ fontSize: "11px", fontFamily: "JetBrains Mono, monospace", color: "#10b981", border: "1px solid rgba(16,185,129,0.3)", backgroundColor: "rgba(16,185,129,0.1)", padding: "2px 6px", borderRadius: "2px" }}>
          {t("login.env_secure", "[ENV: PRODUCTION / SECURE]")}
        </span>
        <LanguageSwitcher />
      </div>

      <div
        className="login__card"
        style={{
          width: "100%",
          maxWidth: "460px",
          backgroundColor: "#181a20",
          border: "1px solid #2d3139",
          borderRadius: "6px",
          boxShadow: "0 20px 40px rgba(0,0,0,0.6)",
          overflow: "hidden",
        }}
      >
        {/* Header Section */}
        <div style={{ padding: "24px", borderBottom: "1px solid #2d3139", textAlign: "center" }}>
          <h2 style={{ fontFamily: "Inter, sans-serif", fontSize: "20px", fontWeight: 600, color: "#ffffff", margin: "0 0 6px 0" }}>
            {t("login.title", "Access Control Plane")}
          </h2>
          <p style={{ fontSize: "13px", color: "#94a3b8", margin: 0 }}>
            {t("login.subtitle", "Sign in with team credentials or emergency root token")}
          </p>
        </div>

        {/* Tab Switcher */}
        <div style={{ display: "flex", borderBottom: "1px solid #2d3139", backgroundColor: "#14171d", fontFamily: "JetBrains Mono, monospace", fontSize: "13px" }}>
          <button
            type="button"
            onClick={() => { setActiveTab("member"); setError(null); }}
            style={{
              flex: 1,
              padding: "12px 16px",
              background: "none",
              border: "none",
              borderBottom: activeTab === "member" ? "2px solid #22d3ee" : "2px solid transparent",
              color: activeTab === "member" ? "#22d3ee" : "#859397",
              cursor: "pointer",
              fontWeight: 600,
              transition: "all 0.15s ease",
            }}
          >
            {t("login.tab_member", "Member Login")}
          </button>
          <button
            type="button"
            onClick={() => { setActiveTab("root"); setError(null); }}
            style={{
              flex: 1,
              padding: "12px 16px",
              background: "none",
              border: "none",
              borderBottom: activeTab === "root" ? "2px solid #f59e0b" : "2px solid transparent",
              color: activeTab === "root" ? "#f59e0b" : "#859397",
              cursor: "pointer",
              fontWeight: 600,
              transition: "all 0.15s ease",
            }}
          >
            {t("login.tab_root", "Emergency Root Key")}
          </button>
        </div>

        <div style={{ padding: "24px" }}>
          {error && <div className="error-banner" style={{ marginBottom: "16px" }}>{error}</div>}

          {activeTab === "member" ? (
            <form onSubmit={handleMemberSubmit} style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
              <div className="field">
                <label className="field__label" htmlFor="member-email" style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", textTransform: "uppercase" }}>
                  {t("login.email", "Email Address")}
                </label>
                <input
                  id="member-email"
                  className="input"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="admin@enterprise.com"
                  autoFocus
                  required
                  style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px" }}
                />
              </div>

              <div className="field">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <label className="field__label" htmlFor="member-pass" style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", textTransform: "uppercase" }}>
                    {t("login.password", "Password")}
                  </label>
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    style={{ background: "none", border: "none", color: "#859397", fontSize: "11px", cursor: "pointer" }}
                  >
                    {showPassword ? t("login.hide", "Hide") : t("login.show", "Show")}
                  </button>
                </div>
                <input
                  id="member-pass"
                  className="input"
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••••••••••"
                  required
                  style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px" }}
                />
              </div>

              <button
                className="btn btn--primary"
                type="submit"
                disabled={busy || !email.trim() || !password}
                style={{ width: "100%", justifyContent: "center", marginTop: "8px", backgroundColor: "#10b981", color: "#000000", fontWeight: 700 }}
              >
                {busy ? t("login.btn_authenticating", "Authenticating…") : t("login.btn_login", "Sign In")}
              </button>

              <div style={{ textAlign: "center", marginTop: "8px" }}>
                <span style={{ fontSize: "11px", color: "#64748b", fontFamily: "JetBrains Mono, monospace" }}>
                  {t("login.default_admin_hint", "Default Admin: admin@llm-gateway.internal / admin")}
                </span>
              </div>
            </form>
          ) : (
            <form onSubmit={handleRootSubmit} style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
              <div style={{ backgroundColor: "rgba(245, 158, 11, 0.08)", border: "1px solid rgba(245, 158, 11, 0.3)", borderRadius: "4px", padding: "12px" }}>
                <span style={{ color: "#f59e0b", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", fontWeight: "bold", display: "block", marginBottom: "4px" }}>
                  {t("login.break_glass", "[BREAK-GLASS ACCESS]")}
                </span>
                <p style={{ margin: 0, fontSize: "12px", color: "#94a3b8", lineHeight: 1.4 }}>
                  {t("login.root_notice", "Use of root tokens is logged permanently in the audit trail and alerts global administrators.")}
                </p>
              </div>

              <div className="field">
                <label className="field__label" htmlFor="admin-key" style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", textTransform: "uppercase" }}>
                  {t("login.root_key_label", "X-Admin-API-Key")}
                </label>
                <input
                  id="admin-key"
                  className="input"
                  type="password"
                  value={rootKey}
                  onChange={(e) => setRootKey(e.target.value)}
                  placeholder="sk-admin-... or master key"
                  autoFocus
                  required
                  style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px" }}
                />
              </div>

              <button
                className="btn btn--primary"
                type="submit"
                disabled={busy || !rootKey.trim()}
                style={{ width: "100%", justifyContent: "center", marginTop: "8px", backgroundColor: "#22d3ee", color: "#000000", fontWeight: 700 }}
              >
                {busy ? t("login.verifying", "Verifying…") : t("login.auth_master", "Authenticate Master")}
              </button>

              <p className="muted" style={{ fontSize: 11, margin: "6px 0 0 0", textAlign: "center", fontFamily: "JetBrains Mono, monospace" }}>
                {t("login.keys_stored_hint", "Keys are stored locally and sent as")} <code>X-Admin-API-Key</code>.
              </p>
            </form>
          )}
        </div>
      </div>

      <div style={{ marginTop: "24px", display: "flex", alignItems: "center", gap: "16px", fontSize: "11px", fontFamily: "JetBrains Mono, monospace", color: "#64748b" }}>
        <span>v1.0.0</span>
        <span>•</span>
        <span style={{ color: "#10b981", display: "flex", alignItems: "center", gap: "6px" }}>
          <span style={{ width: "6px", height: "6px", borderRadius: "50%", backgroundColor: "#10b981", display: "inline-block" }}></span>
          {t("login.tls_active", "TLS 1.3 ACTIVE")}
        </span>
      </div>
    </div>
  );
}

