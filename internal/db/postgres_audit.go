package db

import (
	"fmt"
	"strings"
	"time"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ==========================================================================
// IAM: Audit Logging Repositories
// ==========================================================================

func (p *PostgresDB) CreateAuditLog(log *model.AuditLog) error {
	query := `
		INSERT INTO audit_logs (actor_id, actor_type, actor_ip, action, target_type, target_id, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	var actorIP interface{} = nil
	if log.ActorIP != "" {
		actorIP = log.ActorIP
	}
	return p.QueryRow(query, log.ActorID, log.ActorType, actorIP, log.Action, log.TargetType, log.TargetID, log.Details).Scan(&log.ID, &log.CreatedAt)
}

func (p *PostgresDB) ListAuditLogs(filter model.AuditLogFilter) ([]model.AuditLog, int64, error) {
	var where []string
	var args []interface{}
	idx := 1

	if filter.ActorType != "" {
		where = append(where, fmt.Sprintf("actor_type = $%d", idx))
		args = append(args, filter.ActorType)
		idx++
	}
	if filter.Action != "" {
		where = append(where, fmt.Sprintf("action ILIKE $%d", idx))
		args = append(args, "%"+filter.Action+"%")
		idx++
	}
	if filter.TargetType != "" {
		where = append(where, fmt.Sprintf("target_type = $%d", idx))
		args = append(args, filter.TargetType)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	if err := p.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, actor_id, actor_type, COALESCE(actor_ip::text, ''), action, COALESCE(target_type, ''), COALESCE(target_id, ''), details, created_at
		FROM audit_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, idx, idx+1)
	args = append(args, limit, offset)

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []model.AuditLog
	for rows.Next() {
		var l model.AuditLog
		if err := rows.Scan(&l.ID, &l.ActorID, &l.ActorType, &l.ActorIP, &l.Action, &l.TargetType, &l.TargetID, &l.Details, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}

	return logs, total, nil
}

// ==========================================================================
// IAM: Member & Team Usage Analytics ("ai dùng gì, dùng bao nhiêu")
// ==========================================================================

func (p *PostgresDB) GetMemberUsageAnalytics(from, to time.Time, groupIDFilter string, searchFilter string) (*model.AnalyticsMembersResponse, error) {
	resp := &model.AnalyticsMembersResponse{
		Members: make([]model.MemberUsageSummary, 0),
		Teams:   make([]model.TeamUsageSummary, 0),
	}

	// 1. Fetch Users matching criteria
	userQuery := `
		SELECT u.id, u.email, u.name, u.role, u.is_root, COALESCE(u.is_active, TRUE),
		       COALESCE(ARRAY_AGG(DISTINCT g.name) FILTER (WHERE g.name IS NOT NULL), '{}') AS groups
		FROM users u
		LEFT JOIN user_groups ug ON ug.user_id = u.id
		LEFT JOIN groups g ON g.id = ug.group_id
		WHERE ($1 = '' OR ug.group_id::text = $1)
		  AND ($2 = '' OR LOWER(u.email) LIKE LOWER('%' || $2 || '%') OR LOWER(u.name) LIKE LOWER('%' || $2 || '%'))
		GROUP BY u.id, u.email, u.name, u.role, u.is_root, u.is_active
		ORDER BY u.name ASC, u.email ASC`

	uRows, err := p.Query(userQuery, groupIDFilter, searchFilter)
	if err != nil {
		return nil, err
	}
	defer uRows.Close()

	type memberInfo struct {
		summary model.MemberUsageSummary
	}
	membersMap := make(map[uuid.UUID]*memberInfo)
	orderedUserIDs := make([]uuid.UUID, 0)

	for uRows.Next() {
		var m model.MemberUsageSummary
		if err := uRows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.IsRoot, &m.IsActive, pq.Array(&m.Groups)); err != nil {
			return nil, err
		}
		m.TopModels = make([]model.UserModelUsage, 0)
		membersMap[m.UserID] = &memberInfo{summary: m}
		orderedUserIDs = append(orderedUserIDs, m.UserID)
	}

	// 2. Fetch User API Key Budget limits
	budgetQuery := `SELECT user_id, COALESCE(MAX(budget_limit), 0) FROM api_keys GROUP BY user_id`
	bRows, err := p.Query(budgetQuery)
	if err == nil {
		defer bRows.Close()
		for bRows.Next() {
			var uid uuid.UUID
			var limit float64
			if err := bRows.Scan(&uid, &limit); err == nil {
				if info, exists := membersMap[uid]; exists {
					info.summary.BudgetLimit = limit
				}
			}
		}
	}

	// 3. Fetch User Usage Aggregates for [from, to]
	usageQuery := `
		SELECT user_id,
		       COUNT(*) AS total_requests,
		       COALESCE(SUM(cost), 0) AS total_cost,
		       COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
		       COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens
		FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY user_id`

	usgRows, err := p.Query(usageQuery, from, to)
	if err == nil {
		defer usgRows.Close()
		for usgRows.Next() {
			var uid uuid.UUID
			var reqs, pt, ct int64
			var cost float64
			if err := usgRows.Scan(&uid, &reqs, &cost, &pt, &ct); err == nil {
				if info, exists := membersMap[uid]; exists {
					info.summary.TotalRequests = reqs
					info.summary.TotalCost = cost
					info.summary.SumPromptTokens = pt
					info.summary.SumCompletionTokens = ct
					if info.summary.BudgetLimit > 0 {
						info.summary.BudgetUsedPercent = (cost / info.summary.BudgetLimit) * 100
					}
				}
			}
		}
	}

	// 4. Fetch Top Models per user (Top 3)
	topModelQuery := `
		SELECT user_id, model,
		       COUNT(*) AS reqs,
		       COALESCE(SUM(cost), 0) AS cost,
		       COALESCE(SUM(prompt_tokens), 0) AS pt,
		       COALESCE(SUM(completion_tokens), 0) AS ct
		FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY user_id, model
		ORDER BY user_id, reqs DESC`

	tmRows, err := p.Query(topModelQuery, from, to)
	if err == nil {
		defer tmRows.Close()
		for tmRows.Next() {
			var uid uuid.UUID
			var mu model.UserModelUsage
			if err := tmRows.Scan(&uid, &mu.Model, &mu.TotalRequests, &mu.TotalCost, &mu.SumPromptTokens, &mu.SumCompletionTokens); err == nil {
				if info, exists := membersMap[uid]; exists {
					if len(info.summary.TopModels) < 3 {
						info.summary.TopModels = append(info.summary.TopModels, mu)
					}
				}
			}
		}
	}

	// Assemble members list
	for _, uid := range orderedUserIDs {
		resp.Members = append(resp.Members, membersMap[uid].summary)
	}

	// 5. Team Aggregation
	teamQuery := `
		SELECT g.id, g.name, COALESCE(g.description, ''), COALESCE(g.budget_limit_monthly, 0),
		       COUNT(DISTINCT ug.user_id) AS member_count,
		       COUNT(l.id) AS total_requests,
		       COALESCE(SUM(l.cost), 0) AS total_cost
		FROM groups g
		LEFT JOIN user_groups ug ON ug.group_id = g.id
		LEFT JOIN usage_logs l ON l.user_id = ug.user_id AND l.created_at >= $1 AND l.created_at <= $2
		WHERE g.is_active = TRUE
		GROUP BY g.id, g.name, g.description, g.budget_limit_monthly
		ORDER BY total_cost DESC`

	tRows, err := p.Query(teamQuery, from, to)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var t model.TeamUsageSummary
			if err := tRows.Scan(&t.GroupID, &t.GroupName, &t.Description, &t.BudgetLimitMonthly, &t.MemberCount, &t.TotalRequests, &t.TotalCost); err == nil {
				if t.BudgetLimitMonthly > 0 {
					t.UtilizationPercent = (t.TotalCost / t.BudgetLimitMonthly) * 100
				}
				resp.Teams = append(resp.Teams, t)
				resp.Summary.BudgetLimit += t.BudgetLimitMonthly
			}
		}
	}

	// 6. Overall KPI Summary
	summaryQuery := `
		SELECT COUNT(DISTINCT user_id) AS active_members,
		       COALESCE(SUM(cost), 0) AS total_spend,
		       COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS total_tokens
		FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2`

	_ = p.QueryRow(summaryQuery, from, to).Scan(
		&resp.Summary.ActiveMembers,
		&resp.Summary.TotalSpend,
		&resp.Summary.TotalTokens,
	)

	// Top model overall
	topOverallQuery := `
		SELECT model FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY model ORDER BY COUNT(*) DESC LIMIT 1`
	_ = p.QueryRow(topOverallQuery, from, to).Scan(&resp.Summary.TopModel)
	if resp.Summary.TopModel == "" {
		resp.Summary.TopModel = "n/a"
	}

	return resp, nil
}

func (p *PostgresDB) GetMemberDetail(userID uuid.UUID, from, to time.Time) (*model.MemberDetailResponse, error) {
	resp := &model.MemberDetailResponse{
		ModelDetails: make([]model.UserModelUsage, 0),
		DailyTrends:  make([]model.UsageSeriesPoint, 0),
	}

	// 1. User base info
	u, err := p.GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	groups, _ := p.GetUserGroups(userID)
	groupNames := make([]string, 0, len(groups))
	for _, g := range groups {
		groupNames = append(groupNames, g.Name)
	}

	resp.Member.UserID = u.ID
	resp.Member.Email = u.Email
	resp.Member.Name = u.Name
	resp.Member.Role = u.Role
	resp.Member.IsRoot = u.IsRoot
	resp.Member.IsActive = u.IsActive
	resp.Member.Groups = groupNames
	resp.Member.TopModels = make([]model.UserModelUsage, 0)

	// 2. Budget Limit
	_ = p.QueryRow(`SELECT COALESCE(MAX(budget_limit), 0) FROM api_keys WHERE user_id = $1`, userID).Scan(&resp.Member.BudgetLimit)

	// 3. User usage totals
	totalsQuery := `
		SELECT COUNT(*), COALESCE(SUM(cost), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0)
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3`
	_ = p.QueryRow(totalsQuery, userID, from, to).Scan(
		&resp.Member.TotalRequests,
		&resp.Member.TotalCost,
		&resp.Member.SumPromptTokens,
		&resp.Member.SumCompletionTokens,
	)
	if resp.Member.BudgetLimit > 0 {
		resp.Member.BudgetUsedPercent = (resp.Member.TotalCost / resp.Member.BudgetLimit) * 100
	}

	// 4. Model details breakdown
	modelQuery := `
		SELECT model,
		       COUNT(*) AS reqs,
		       COALESCE(SUM(cost), 0) AS cost,
		       COALESCE(SUM(prompt_tokens), 0) AS pt,
		       COALESCE(SUM(completion_tokens), 0) AS ct
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3
		GROUP BY model
		ORDER BY cost DESC`

	mRows, err := p.Query(modelQuery, userID, from, to)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var mu model.UserModelUsage
			if err := mRows.Scan(&mu.Model, &mu.TotalRequests, &mu.TotalCost, &mu.SumPromptTokens, &mu.SumCompletionTokens); err == nil {
				resp.ModelDetails = append(resp.ModelDetails, mu)
				if len(resp.Member.TopModels) < 3 {
					resp.Member.TopModels = append(resp.Member.TopModels, mu)
				}
			}
		}
	}

	// 5. Daily trends
	trendQuery := `
		SELECT date_trunc('day', created_at) AS bucket,
		       COUNT(*) AS total_requests,
		       COALESCE(SUM(cost), 0) AS total_cost,
		       COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
		       COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens,
		       0 AS sum_cached_tokens,
		       0 AS sum_reasoning_tokens,
		       COALESCE(COUNT(*) FILTER (WHERE status_code >= 400), 0) AS error_count,
		       0 AS avg_ttft_ms,
		       COALESCE(ROUND(AVG(total_latency_ms), 2), 0) AS avg_latency_ms
		FROM usage_logs
		WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3
		GROUP BY bucket
		ORDER BY bucket ASC`

	trRows, err := p.Query(trendQuery, userID, from, to)
	if err == nil {
		defer trRows.Close()
		for trRows.Next() {
			var pt model.UsageSeriesPoint
			if err := trRows.Scan(&pt.TimeBucket, &pt.TotalRequests, &pt.TotalCost, &pt.SumPromptTokens, &pt.SumCompletionTokens, &pt.SumCachedTokens, &pt.SumReasoningTokens, &pt.ErrorCount, &pt.AvgTTFTMs, &pt.AvgLatencyMs); err == nil {
				resp.DailyTrends = append(resp.DailyTrends, pt)
			}
		}
	}

	return resp, nil
}
