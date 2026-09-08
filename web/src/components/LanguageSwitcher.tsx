import { useLanguage } from "../i18n";

export default function LanguageSwitcher() {
  const { lang, setLang } = useLanguage();

  return (
    <div
      className="mono"
      style={{
        display: "inline-flex",
        alignItems: "center",
        background: "var(--surface-high)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-sm)",
        padding: 2,
        gap: 2,
        fontSize: 11,
        userSelect: "none",
      }}
      title="Chuyển đổi ngôn ngữ tức thì / Switch Language"
    >
      <button
        type="button"
        onClick={() => setLang("vi")}
        style={{
          border: "none",
          cursor: "pointer",
          borderRadius: 3,
          padding: "3px 7px",
          fontWeight: 700,
          background: lang === "vi" ? "#22d3ee" : "transparent",
          color: lang === "vi" ? "#001f25" : "var(--text-muted)",
          transition: "all 0.15s ease",
        }}
      >
        🇻🇳 VI
      </button>
      <button
        type="button"
        onClick={() => setLang("en")}
        style={{
          border: "none",
          cursor: "pointer",
          borderRadius: 3,
          padding: "3px 7px",
          fontWeight: 700,
          background: lang === "en" ? "#22d3ee" : "transparent",
          color: lang === "en" ? "#001f25" : "var(--text-muted)",
          transition: "all 0.15s ease",
        }}
      >
        🇬🇧 EN
      </button>
    </div>
  );
}
