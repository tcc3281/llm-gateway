import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type {
  AnalyticsMembersResponse,
  MemberDetailResponse,
  MemberUsageSummary,
} from "../api/types";

export default function MemberAnalytics() {
  const [range, setRange] = useState<"24h" | "7d" | "30d">("7d");
  const [selectedGroup, setSelectedGroup] = useState<string>("");
  const [search, setSearch] = useState<string>("");
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<AnalyticsMembersResponse | null>(null);
  const [selectedMember, setSelectedMember] = useState<MemberUsageSummary | null>(null);
  const [memberDetail, setMemberDetail] = useState<MemberDetailResponse | null>(null);
  const [loadingDetail, setLoadingDetail] = useState<boolean>(false);

  const getTimeRange = (r: "24h" | "7d" | "30d") => {
    const now = new Date();
    const to = now.toISOString();
    const fromDate = new Date();
    if (r === "24h") fromDate.setHours(now.getHours() - 24);
    else if (r === "7d") fromDate.setDate(now.getDate() - 7);
    else fromDate.setDate(now.getDate() - 30);
    return { from: fromDate.toISOString(), to };
  };

  const loadAnalytics = useCallback(async () => {
    setLoading(true);
    try {
      const { from, to } = getTimeRange(range);
      const res = await api.getMemberAnalytics({
        from,
        to,
        group_id: selectedGroup || undefined,
        search: search.trim() || undefined,
      });
      setData(res);
      if (res.members.length > 0 && !selectedMember) {
        setSelectedMember(res.members[0]);
      }
    } catch (err) {
      console.error("Failed to load member analytics", err);
    } finally {
      setLoading(false);
    }
  }, [range, selectedGroup, search, selectedMember]);

  useEffect(() => {
    loadAnalytics();
  }, [range, selectedGroup]);

  // Handle search with debounce
  useEffect(() => {
    const timer = setTimeout(() => {
      loadAnalytics();
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  // Load drilldown detail when member selection changes
  useEffect(() => {
    if (!selectedMember) {
      setMemberDetail(null);
      return;
    }
    setLoadingDetail(true);
    const { from, to } = getTimeRange(range);
    api
      .getMemberDetail(selectedMember.user_id, { from, to })
      .then((res) => setMemberDetail(res))
      .catch((err) => console.error("Failed to load member detail", err))
      .finally(() => setLoadingDetail(false));
  }, [selectedMember, range]);

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
    if (n >= 1_000) return (n / 1_000).toFixed(0) + "K";
    return n.toString();
  };

  const exportCSV = () => {
    if (!data?.members) return;
    const headers = [
      "Name",
      "Email",
      "Role",
      "Teams",
      "Total Requests",
      "Prompt Tokens",
      "Completion Tokens",
      "Total Cost (USD)",
      "Budget Limit",
      "Budget Used %",
    ];
    const rows = data.members.map((m) => [
      `"${m.name || "N/A"}"`,
      `"${m.email}"`,
      m.role,
      `"${m.groups.join(", ")}"`,
      m.requests,
      m.prompt_tokens,
      m.completion_tokens,
      m.cost_usd.toFixed(4),
      m.budget_limit,
      m.budget_used_percent.toFixed(1) + "%",
    ]);
    const csvContent = "data:text/csv;charset=utf-8," + [headers.join(","), ...rows.map((r) => r.join(","))].join("\n");
    const encodedUri = encodeURI(csvContent);
    const link = document.createElement("a");
    link.setAttribute("href", encodedUri);
    link.setAttribute("download", `llm-gateway-member-usage-${range}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "24px" }}>
      {/* Page Title & Breadcrumb */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end" }}>
        <div>
          <h1 style={{ fontFamily: "Inter, sans-serif", fontSize: "22px", fontWeight: 700, margin: "0 0 4px 0", color: "#dde4e5" }}>
            Member &amp; Team Usage Analytics
          </h1>
          <p style={{ margin: 0, fontSize: "13px", color: "#859397" }}>
            Telemetry, model breakdown, and budget quota tracking per team member
          </p>
        </div>
        <div style={{ display: "flex", gap: "8px" }}>
          <button
            onClick={exportCSV}
            className="btn"
            style={{
              display: "flex",
              alignItems: "center",
              gap: "6px",
              backgroundColor: "#1a2122",
              border: "1px solid #2d3139",
              color: "#dde4e5",
              fontFamily: "JetBrains Mono, monospace",
              fontSize: "12px",
            }}
          >
            Export CSV
          </button>
        </div>
      </div>

      {/* TOP KPI STRIP */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
          gap: "16px",
        }}
      >
        {/* Card 1: Active Members */}
        <div
          style={{
            backgroundColor: "#1a2122",
            border: "1px solid #2d3139",
            borderRadius: "4px",
            padding: "16px",
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", fontWeight: 700, color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>
            ACTIVE MEMBERS
          </span>
          <div style={{ margin: "12px 0 0 0" }}>
            <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "28px", fontWeight: 700, color: "#22d3ee" }}>
              {data?.summary.active_members ?? 0}
            </span>
          </div>
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397" }}>
            Making calls in past {range}
          </span>
        </div>

        {/* Card 2: Total Team Spend & Quota */}
        <div
          style={{
            backgroundColor: "#1a2122",
            border: "1px solid #2d3139",
            borderRadius: "4px",
            padding: "16px",
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", fontWeight: 700, color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>
            TOTAL SPEND / BUDGET
          </span>
          <div style={{ margin: "8px 0 0 0" }}>
            <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "22px", fontWeight: 700, color: "#10b981" }}>
              ${data?.summary.total_spend.toFixed(2) ?? "0.00"}
            </span>
            <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px", color: "#859397" }}>
              {" "}
              / ${data?.summary.budget_limit.toFixed(0) || "unlimited"}
            </span>
          </div>
          {data && data.summary.budget_limit > 0 && (
            <div style={{ marginTop: "8px" }}>
              <div style={{ width: "100%", height: "6px", backgroundColor: "#242b2d", borderRadius: "2px", overflow: "hidden" }}>
                <div
                  style={{
                    height: "100%",
                    width: `${Math.min(100, (data.summary.total_spend / data.summary.budget_limit) * 100)}%`,
                    backgroundColor: (data.summary.total_spend / data.summary.budget_limit) > 0.9 ? "#ef4444" : (data.summary.total_spend / data.summary.budget_limit) > 0.7 ? "#f59e0b" : "#10b981",
                  }}
                />
              </div>
              <div style={{ textAlign: "right", fontSize: "10px", fontFamily: "JetBrains Mono, monospace", color: "#859397", marginTop: "4px" }}>
                {((data.summary.total_spend / data.summary.budget_limit) * 100).toFixed(1)}% QUOTA
              </div>
            </div>
          )}
        </div>

        {/* Card 3: Top Model */}
        <div
          style={{
            backgroundColor: "#1a2122",
            border: "1px solid #2d3139",
            borderRadius: "4px",
            padding: "16px",
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", fontWeight: 700, color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>
            MOST POPULAR MODEL
          </span>
          <div style={{ margin: "12px 0 0 0" }}>
            <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px", color: "#a855f7", backgroundColor: "rgba(168, 85, 247, 0.1)", border: "1px solid rgba(168, 85, 247, 0.3)", padding: "4px 8px", borderRadius: "3px" }}>
              [{data?.summary.top_model || "none"}]
            </span>
          </div>
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397" }}>
            Highest request volume
          </span>
        </div>

        {/* Card 4: Total Tokens */}
        <div
          style={{
            backgroundColor: "#1a2122",
            border: "1px solid #2d3139",
            borderRadius: "4px",
            padding: "16px",
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", fontWeight: 700, color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>
            TOTAL TOKENS PROCESSED
          </span>
          <div style={{ margin: "12px 0 0 0" }}>
            <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "28px", fontWeight: 700, color: "#22d3ee" }}>
              {formatTokens(data?.summary.total_tokens ?? 0)}
            </span>
          </div>
          <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397" }}>
            Prompt + Completion combined
          </span>
        </div>
      </div>

      {/* FILTER BAR */}
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: "12px",
          padding: "12px 16px",
          backgroundColor: "#1a2122",
          border: "1px solid #2d3139",
          borderRadius: "4px",
        }}
      >
        {/* Range Buttons */}
        <div style={{ display: "flex", border: "1px solid #3c494c", borderRadius: "3px", overflow: "hidden" }}>
          {(["24h", "7d", "30d"] as const).map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              style={{
                padding: "6px 14px",
                fontFamily: "JetBrains Mono, monospace",
                fontSize: "12px",
                border: "none",
                cursor: "pointer",
                backgroundColor: range === r ? "rgba(34, 211, 238, 0.15)" : "#242b2d",
                color: range === r ? "#22d3ee" : "#dde4e5",
                borderRight: r !== "30d" ? "1px solid #3c494c" : "none",
              }}
            >
              {r}
            </button>
          ))}
        </div>

        {/* Team Dropdown */}
        <select
          value={selectedGroup}
          onChange={(e) => setSelectedGroup(e.target.value)}
          style={{
            backgroundColor: "#242b2d",
            border: "1px solid #3c494c",
            color: "#dde4e5",
            fontFamily: "JetBrains Mono, monospace",
            fontSize: "12px",
            padding: "6px 12px",
            borderRadius: "3px",
            outline: "none",
          }}
        >
          <option value="">All Teams &amp; Groups</option>
          {data?.teams.map((t) => (
            <option key={t.group_id} value={t.group_id}>
              {t.group_name} ({t.member_count} members)
            </option>
          ))}
        </select>

        {/* Search Input */}
        <div style={{ flex: 1, minWidth: "220px" }}>
          <input
            type="text"
            placeholder="Search member by name or email..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              width: "100%",
              backgroundColor: "#242b2d",
              border: "1px solid #3c494c",
              color: "#dde4e5",
              fontFamily: "JetBrains Mono, monospace",
              fontSize: "12px",
              padding: "6px 12px",
              borderRadius: "3px",
              outline: "none",
            }}
          />
        </div>
      </div>

      {/* MAIN SECTION: Matrix Table + Side Drill-down Drawer */}
      <div style={{ display: "flex", gap: "20px", alignItems: "flex-start" }}>
        {/* Table View */}
        <div
          style={{
            flex: 1,
            backgroundColor: "#1a2122",
            border: "1px solid #2d3139",
            borderRadius: "4px",
            overflow: "hidden",
          }}
        >
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", textAlign: "left" }}>
              <thead>
                <tr style={{ borderBottom: "1px solid #2d3139", backgroundColor: "rgba(47, 54, 56, 0.4)" }}>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>Member</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>Assigned Team</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase" }}>Top Models Used</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", textAlign: "right" }}>Total Reqs</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", textAlign: "right" }}>Prompt Tok</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", textAlign: "right" }}>Comp Tok</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", textAlign: "right" }}>Total Cost</th>
                  <th style={{ padding: "12px 16px", fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", width: "140px" }}>Budget Quota</th>
                </tr>
              </thead>
              <tbody style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "13px" }}>
                {loading && (
                  <tr>
                    <td colSpan={8} style={{ padding: "32px", textAlign: "center", color: "#859397" }}>
                      Loading member usage analytics...
                    </td>
                  </tr>
                )}
                {!loading && data?.members.length === 0 && (
                  <tr>
                    <td colSpan={8} style={{ padding: "32px", textAlign: "center", color: "#859397" }}>
                      No member activity found matching filters.
                    </td>
                  </tr>
                )}
                {!loading &&
                  data?.members.map((m) => {
                    const isSelected = selectedMember?.user_id === m.user_id;
                    const quotaColor =
                      m.budget_used_percent > 90 ? "#ef4444" : m.budget_used_percent > 70 ? "#f59e0b" : "#10b981";
                    return (
                      <tr
                        key={m.user_id}
                        onClick={() => setSelectedMember(m)}
                        style={{
                          borderBottom: "1px solid #1f2229",
                          backgroundColor: isSelected ? "rgba(34, 211, 238, 0.08)" : "transparent",
                          cursor: "pointer",
                          transition: "background-color 0.15s ease",
                        }}
                      >
                        <td style={{ padding: "12px 16px" }}>
                          <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                            <div
                              style={{
                                width: "32px",
                                height: "32px",
                                borderRadius: "4px",
                                backgroundColor: isSelected ? "#22d3ee" : "#242b2d",
                                color: isSelected ? "#000" : "#22d3ee",
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                                fontWeight: 700,
                                fontSize: "12px",
                              }}
                            >
                              {(m.name || m.email).substring(0, 2).toUpperCase()}
                            </div>
                            <div>
                              <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
                                <span style={{ fontWeight: 600, color: "#ffffff" }}>{m.name || "Unnamed"}</span>
                                {m.is_root ? (
                                  <span style={{ fontSize: "9px", padding: "1px 4px", borderRadius: "2px", border: "1px solid rgba(34, 211, 238, 0.4)", backgroundColor: "rgba(34, 211, 238, 0.1)", color: "#22d3ee", fontWeight: 700 }}>
                                    [ROOT]
                                  </span>
                                ) : m.role === "admin" ? (
                                  <span style={{ fontSize: "9px", padding: "1px 4px", borderRadius: "2px", border: "1px solid rgba(16, 185, 129, 0.4)", backgroundColor: "rgba(16, 185, 129, 0.1)", color: "#10b981", fontWeight: 700 }}>
                                    [LEAD]
                                  </span>
                                ) : (
                                  <span style={{ fontSize: "9px", padding: "1px 4px", borderRadius: "2px", border: "1px solid #3c494c", backgroundColor: "#242b2d", color: "#859397", fontWeight: 700 }}>
                                    [MEMBER]
                                  </span>
                                )}
                              </div>
                              <div style={{ fontSize: "11px", color: "#859397" }}>{m.email}</div>
                            </div>
                          </div>
                        </td>

                        <td style={{ padding: "12px 16px", color: "#dde4e5" }}>
                          {m.groups.length > 0 ? (
                            <span style={{ color: "#22d3ee" }}>{m.groups.join(", ")}</span>
                          ) : (
                            <span style={{ color: "#64748b" }}>Unassigned</span>
                          )}
                        </td>

                        <td style={{ padding: "12px 16px" }}>
                          <div style={{ display: "flex", flexWrap: "wrap", gap: "4px" }}>
                            {m.top_models.length > 0 ? (
                              m.top_models.map((tm) => (
                                <span
                                  key={tm.model}
                                  style={{
                                    fontSize: "11px",
                                    backgroundColor: "#242b2d",
                                    border: "1px solid #3c494c",
                                    borderRadius: "2px",
                                    padding: "2px 6px",
                                    color: "#dde4e5",
                                  }}
                                >
                                  [{tm.model}]
                                </span>
                              ))
                            ) : (
                              <span style={{ fontSize: "11px", color: "#64748b" }}>none</span>
                            )}
                          </div>
                        </td>

                        <td style={{ padding: "12px 16px", textAlign: "right", color: "#ffffff", fontWeight: 600 }}>
                          {m.requests.toLocaleString()}
                        </td>

                        <td style={{ padding: "12px 16px", textAlign: "right", color: "#859397" }}>
                          {formatTokens(m.prompt_tokens)}
                        </td>

                        <td style={{ padding: "12px 16px", textAlign: "right", color: "#859397" }}>
                          {formatTokens(m.completion_tokens)}
                        </td>

                        <td style={{ padding: "12px 16px", textAlign: "right", color: "#10b981", fontWeight: 700 }}>
                          ${m.cost_usd.toFixed(2)}
                        </td>

                        <td style={{ padding: "12px 16px" }}>
                          {m.budget_limit > 0 ? (
                            <div>
                              <div style={{ width: "100%", height: "4px", backgroundColor: "#242b2d", borderRadius: "2px", overflow: "hidden", marginBottom: "4px" }}>
                                <div style={{ height: "100%", width: `${Math.min(100, m.budget_used_percent)}%`, backgroundColor: quotaColor }} />
                              </div>
                              <div style={{ fontSize: "10px", color: quotaColor, textAlign: "right" }}>
                                {m.budget_used_percent.toFixed(0)}% of ${m.budget_limit.toFixed(0)}
                              </div>
                            </div>
                          ) : (
                            <span style={{ fontSize: "11px", color: "#64748b" }}>Unlimited</span>
                          )}
                        </td>
                      </tr>
                    );
                  })}
              </tbody>
            </table>
          </div>
        </div>

        {/* SIDE DRILL-DOWN PANEL */}
        {selectedMember && (
          <aside
            style={{
              width: "360px",
              backgroundColor: "#1a2122",
              border: "1px solid #2d3139",
              borderRadius: "4px",
              display: "flex",
              flexDirection: "column",
              flexShrink: 0,
            }}
          >
            <div style={{ padding: "16px", borderBottom: "1px solid #2d3139", display: "flex", justifyContent: "space-between", alignItems: "flex-start", backgroundColor: "rgba(47, 54, 56, 0.2)" }}>
              <div>
                <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#22d3ee", letterSpacing: "0.05em", textTransform: "uppercase", fontWeight: 700 }}>
                  MEMBER TELEMETRY
                </span>
                <h3 style={{ fontFamily: "Inter, sans-serif", fontSize: "16px", fontWeight: 600, color: "#ffffff", margin: "4px 0 2px 0" }}>
                  {selectedMember.name || "Member Details"}
                </h3>
                <span style={{ fontSize: "12px", color: "#859397", fontFamily: "JetBrains Mono, monospace" }}>
                  {selectedMember.email}
                </span>
              </div>
              <button
                onClick={() => setSelectedMember(null)}
                style={{ background: "none", border: "none", color: "#859397", cursor: "pointer", fontSize: "16px" }}
              >
                ✕
              </button>
            </div>

            <div style={{ padding: "16px", display: "flex", flexDirection: "column", gap: "20px" }}>
              {/* Member Status Badge */}
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "8px 12px", backgroundColor: "#242b2d", border: "1px solid #3c494c", borderRadius: "3px" }}>
                <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397" }}>STATUS:</span>
                <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#10b981", backgroundColor: "rgba(16, 185, 129, 0.1)", border: "1px solid rgba(16, 185, 129, 0.3)", padding: "2px 6px", borderRadius: "2px", fontWeight: 700 }}>
                  [ACTIVE_LOGGING]
                </span>
              </div>

              {/* Models Breakdown */}
              <div>
                <h4 style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", margin: "0 0 10px 0" }}>
                  Model Distribution ({range})
                </h4>
                {loadingDetail ? (
                  <div style={{ fontSize: "12px", color: "#859397", fontFamily: "JetBrains Mono, monospace" }}>Loading breakdown...</div>
                ) : memberDetail?.model_details.length === 0 ? (
                  <div style={{ fontSize: "12px", color: "#64748b", fontFamily: "JetBrains Mono, monospace" }}>No model calls recorded.</div>
                ) : (
                  <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                    {memberDetail?.model_details.map((md, idx) => {
                      const share = selectedMember.cost_usd > 0 ? (md.cost_usd / selectedMember.cost_usd) * 100 : 0;
                      const accentColor = idx === 0 ? "#22d3ee" : idx === 1 ? "#a855f7" : "#10b981";
                      return (
                        <div
                          key={md.model}
                          style={{
                            padding: "8px 12px",
                            backgroundColor: "#242b2d",
                            border: "1px solid #3c494c",
                            borderLeft: `3px solid ${accentColor}`,
                            borderRadius: "2px",
                            fontFamily: "JetBrains Mono, monospace",
                          }}
                        >
                          <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12px", marginBottom: "4px" }}>
                            <span style={{ color: "#ffffff", fontWeight: 600 }}>[{md.model}]</span>
                            <span style={{ color: "#10b981", fontWeight: 700 }}>${md.cost_usd.toFixed(2)}</span>
                          </div>
                          <div style={{ display: "flex", justifyContent: "space-between", fontSize: "11px", color: "#859397" }}>
                            <span>{md.requests.toLocaleString()} reqs</span>
                            <span>{share.toFixed(1)}% spend</span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Daily Trend Volume Sparkline Bars */}
              <div>
                <h4 style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", margin: "0 0 10px 0" }}>
                  Daily Request Trend
                </h4>
                {memberDetail?.daily_trends && memberDetail.daily_trends.length > 0 ? (
                  <div
                    style={{
                      height: "80px",
                      backgroundColor: "#242b2d",
                      border: "1px solid #3c494c",
                      borderRadius: "3px",
                      display: "flex",
                      alignItems: "flex-end",
                      padding: "8px 6px",
                      gap: "4px",
                    }}
                  >
                    {(() => {
                      const max = Math.max(...memberDetail.daily_trends.map((t) => t.requests), 1);
                      return memberDetail.daily_trends.map((t) => {
                        const heightPct = Math.max(8, (t.requests / max) * 100);
                        return (
                          <div
                            key={t.time_bucket}
                            title={`${t.time_bucket}: ${t.requests} reqs ($${t.cost_usd.toFixed(2)})`}
                            style={{
                              flex: 1,
                              height: `${heightPct}%`,
                              backgroundColor: "rgba(34, 211, 238, 0.5)",
                              borderRadius: "2px 2px 0 0",
                              transition: "background-color 0.15s ease",
                              cursor: "pointer",
                            }}
                            onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = "#22d3ee")}
                            onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = "rgba(34, 211, 238, 0.5)")}
                          />
                        );
                      });
                    })()}
                  </div>
                ) : (
                  <div style={{ fontSize: "11px", color: "#64748b", fontFamily: "JetBrains Mono, monospace" }}>
                    No daily points recorded in window.
                  </div>
                )}
              </div>

              {/* Action Button */}
              <button
                onClick={() => window.location.href = "/audit-logs"}
                className="btn"
                style={{
                  width: "100%",
                  justifyContent: "center",
                  backgroundColor: "#242b2d",
                  border: "1px solid #3c494c",
                  color: "#dde4e5",
                  fontFamily: "JetBrains Mono, monospace",
                  fontSize: "11px",
                  marginTop: "8px",
                }}
              >
                VIEW FULL AUDIT LOG
              </button>
            </div>
          </aside>
        )}
      </div>

      {/* MAIN SECTION 3: TEAM AGGREGATION CARDS */}
      {data && data.teams.length > 0 && (
        <div style={{ marginTop: "8px" }}>
          <h3 style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "11px", color: "#859397", letterSpacing: "0.05em", textTransform: "uppercase", margin: "0 0 12px 0" }}>
            TEAM AGGREGATION ({range})
          </h3>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: "16px" }}>
            {data.teams.map((t) => {
              const utilColor =
                t.utilization_percent > 90 ? "#ef4444" : t.utilization_percent > 70 ? "#f59e0b" : "#10b981";
              return (
                <div
                  key={t.group_id}
                  style={{
                    backgroundColor: "#1a2122",
                    border: "1px solid #2d3139",
                    borderRadius: "4px",
                    padding: "16px",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: "12px" }}>
                    <div>
                      <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "14px", fontWeight: 700, color: "#ffffff" }}>
                        {t.group_name}
                      </span>
                      <div style={{ fontSize: "11px", color: "#859397", fontFamily: "JetBrains Mono, monospace" }}>
                        {t.member_count} active members · {t.requests.toLocaleString()} reqs
                      </div>
                    </div>
                    <span style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "14px", fontWeight: 700, color: "#10b981" }}>
                      ${t.cost_usd.toFixed(2)}
                    </span>
                  </div>

                  <div style={{ width: "100%", height: "6px", backgroundColor: "#242b2d", borderRadius: "2px", overflow: "hidden", marginBottom: "6px" }}>
                    <div
                      style={{
                        height: "100%",
                        width: `${Math.min(100, t.utilization_percent)}%`,
                        backgroundColor: utilColor,
                      }}
                    />
                  </div>

                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: "10px", fontFamily: "JetBrains Mono, monospace", color: "#859397" }}>
                    <span>UTILIZATION</span>
                    <span style={{ color: utilColor }}>
                      {t.utilization_percent.toFixed(1)}% / ${t.budget_limit_monthly.toFixed(0) || "unlimited"}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
