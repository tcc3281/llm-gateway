import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [key, setKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await login(key.trim());
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login">
      <form className="login__card" onSubmit={submit}>
        <div className="login__brand">
          <div className="sidebar__logo">GW</div>
          <h1 className="login__title">LLM Gateway Admin</h1>
        </div>
        <p className="login__sub">
          Sign in with the admin API key to monitor and manage the gateway.
        </p>
        {error && <div className="error-banner">{error}</div>}
        <div className="field">
          <label className="field__label" htmlFor="admin-key">
            Admin API key
          </label>
          <input
            id="admin-key"
            className="input"
            type="password"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="sk-admin-..."
            autoFocus
            autoComplete="off"
          />
        </div>
        <button
          className="btn btn--primary"
          type="submit"
          disabled={busy || !key.trim()}
          style={{ width: "100%", justifyContent: "center" }}
        >
          {busy ? "Verifying…" : "Sign in"}
        </button>
        <p className="muted" style={{ fontSize: 12, marginTop: 14, marginBottom: 0 }}>
          Key is stored in your browser and sent as{" "}
          <code>X-Admin-API-Key</code>.
        </p>
      </form>
    </div>
  );
}
