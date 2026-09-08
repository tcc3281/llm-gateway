import { useSearchParams } from "react-router-dom";
import Users from "./Users";
import Groups from "./Groups";
import Tenants from "./Tenants";
import MemberAnalytics from "./MemberAnalytics";

type TabKey = "users" | "groups" | "tenants" | "analytics";

interface TabItem {
  key: TabKey;
  label: string;
  icon: string;
  desc: string;
}

const TABS: TabItem[] = [
  {
    key: "users",
    label: "Users & Accounts",
    icon: "person",
    desc: "Manage user accounts, roles, and administrative access",
  },
  {
    key: "groups",
    label: "IAM Groups & Policies",
    icon: "shield_person",
    desc: "Organize teams, assign monthly budgets, and configure IAM permission policies",
  },
  {
    key: "tenants",
    label: "Tenants & B2B Billing",
    icon: "domain",
    desc: "Manage multi-tenant SaaS clients, system boundary models, and prepaid credit escrows",
  },
  {
    key: "analytics",
    label: "Member Analytics",
    icon: "insights",
    desc: "Track team and member consumption, spending ceilings, and model breakdown",
  },
];

export default function PeopleAccess() {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentTab = (searchParams.get("tab") as TabKey) || "users";

  const handleTabChange = (tab: TabKey) => {
    setSearchParams({ tab });
  };

  const activeTabItem = TABS.find((t) => t.key === currentTab) || TABS[0];

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
              Access Control & Organization
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, margin: "2px 0 4px 0" }}>People & Access</h1>
            <p className="muted" style={{ fontSize: 13, margin: 0, paddingBottom: 12 }}>
              {activeTabItem.desc}
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
            {TABS.map((tab) => {
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
                  <span>{tab.label}</span>
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
