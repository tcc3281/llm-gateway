package builtin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"llm-gateway/internal/plugin"
)

type BudgetControlPlugin struct{}

func NewBudgetControlPlugin() *BudgetControlPlugin {
	return &BudgetControlPlugin{}
}

func (p *BudgetControlPlugin) Name() string {
	return "budget-control"
}

func (p *BudgetControlPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostAuth}
}

func (p *BudgetControlPlugin) ExecPostAuth(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.APIKey == nil && pctx.Tenant == nil {
		return nil
	}

	limit := 0.0
	if v, ok := cfg["budget_limit"].(float64); ok && v > 0 {
		limit = v
	} else if pctx.APIKey != nil && pctx.APIKey.BudgetLimit > 0 {
		limit = pctx.APIKey.BudgetLimit
	} else if pctx.Tenant != nil && pctx.Tenant.BudgetLimitMonthly > 0 {
		limit = pctx.Tenant.BudgetLimitMonthly
	}

	if limit <= 0 {
		return nil
	}

	action := "block"
	if v, ok := cfg["action"].(string); ok && v != "" {
		action = v
	}

	// Calculate spent
	var spent float64
	var err error
	gotFromCache := false

	keyID := ""
	if pctx.APIKey != nil {
		keyID = pctx.APIKey.ID.String()
	}

	if pctx.Redis != nil && keyID != "" {
		if cachedSpent, cErr := pctx.Redis.GetCachedBudgetSpent(pctx.Ctx, keyID); cErr == nil {
			spent = cachedSpent
			gotFromCache = true
		}
	}

	if !gotFromCache && pctx.Postgres != nil && pctx.APIKey != nil {
		spent, err = pctx.Postgres.GetSpentCostByKeyID(pctx.APIKey.ID)
		if err == nil && pctx.Redis != nil && keyID != "" {
			_ = pctx.Redis.SetCachedBudgetSpent(pctx.Ctx, keyID, spent, 60*time.Second)
		}
	}

	if err == nil && spent >= limit {
		if action == "block" {
			pctx.Stop = true
			pctx.StopStatusCode = http.StatusPaymentRequired
			errResp := map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("Budget limit of $%.2f has been reached (Current spent: $%.2f).", limit, spent),
					"type":    "insufficient_quota",
					"code":    "budget_limit_exceeded",
				},
			}
			pctx.StopErrorBody, _ = json.Marshal(errResp)
			return nil
		}

		if pctx.ExtraHeaders == nil {
			pctx.ExtraHeaders = make(map[string]string)
		}
		pctx.ExtraHeaders["X-Budget-Warning"] = fmt.Sprintf("Limit $%.2f exceeded (Spent: $%.2f)", limit, spent)
	}

	return nil
}
