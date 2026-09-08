import { useSearchParams } from "react-router-dom";
import Users from "./Users";
import Groups from "./Groups";
import Tenants from "./Tenants";
import MemberAnalytics from "./MemberAnalytics";
import { useLanguage } from "../i18n";

type TabKey = "users" | "groups" | "tenants" | "analytics";

interface TabConfig {
  key: TabKey;
  labelKey: string;
  icon: string;
  descKey: string;
}

const TAB_CONFIGS: TabConfig[] = [
  {
    key: "users",
    labelKey: "people.tab_users",
    icon: "person",
    descKey: "people.tab_users_desc",
  },
  {
    key: "groups",
    labelKey: "people.tab_groups",
    icon: "shield_person",
    descKey: "people.tab_groups_desc",
  },
  {
    key: "tenants",
    labelKey: "people.tab_tenants",
    icon: "domain",
    descKey: "people.tab_tenants_desc",
  },
  {
    key: "analytics",
    labelKey: "people.tab_analytics",
    icon: "insights",
    descKey: "people.tab_analytics_desc",
  },
];

export default function PeopleAccess() {
  const { t } = useLanguage();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentTab = (searchParams.get("tab") as TabKey) || "users";

  const handleTabChange = (tab: TabKey) => {
    setSearchParams({ tab });
  };

  const activeTabItem = TAB_CONFIGS.find((tItem) => tItem.key === currentTab) || TAB_CONFIGS[0];

  return (
    <div>
      {/* Top Header with Tab Switcher */}
      <div
        style={{
          borderBottom: "1px solid var(--border)",
          background: "var(--surface)",
          padding: "16px 24px 0 24px",
          marginBottom: 16,
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", flexWrap: "wrap", gap: 16 }}>
          <div>
            <div className="mono" style={{ fontSize: 11, color: "var(--primary)", textTransform: "uppercase", fontWeight: 700 }}>
              {t("people.badge", "ACCESS CONTROL & ORGANIZATION")}
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, margin: "2px 0 4px 0" }}>
              {t("people.title", "People & Access Management")}
            </h1>
            <p className="muted" style={{ fontSize: 13, margin: 0, paddingBottom: 12 }}>
              {t(activeTabItem.descKey)}
            </p>
          </div>

          {/* Segmented Pill Tabs */}
          <div
            style={{
              display: "flex",
              gap: 4,
              background: "var(--surface-high)",
              padding: 4,
              borderRadius: 8,
              border: "1px solid var(--border)",
              marginBottom: 12,
            }}
          >
            {TAB_CONFIGS.map((tab) => {
              const isActive = tab.key === currentTab;
              return (
                <button
                  key={tab.key}
                  type="button"
                  onClick={() => handleTabChange(tab.key)}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 6,
                    padding: "6px 14px",
                    borderRadius: 6,
                    fontSize: 13,
                    cursor: "pointer",
                    border: "none",
                    background: isActive ? "var(--primary)" : "transparent",
                    color: isActive ? "#06282d" : "var(--text-muted)",
                    fontWeight: isActive ? 700 : 400,
                    boxShadow: isActive ? "0 0 12px rgba(34, 211, 238, 0.35)" : "none",
                    transition: "all 0.15s ease",
                  }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                    {tab.icon}
                  </span>
                  <span>{t(tab.labelKey)}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>

      {/* Tab Content Panes */}
      <div style={{ padding: "0 8px" }}>
        {currentTab === "users" && <Users />}
        {currentTab === "groups" && <Groups />}
        {currentTab === "tenants" && <Tenants />}
        {currentTab === "analytics" && <MemberAnalytics />}
      </div>
    </div>
  );
}
