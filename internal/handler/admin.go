package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/auth"
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/service"

	"github.com/google/uuid"
)

type AdminHandler struct {
	pg          *db.PostgresDB
	authService *auth.AuthService
	keyStore    *provider.KeyStore
	costCalc    *analytics.CostCalculator
	redis       *db.RedisClient
	allowedIPs  []string
	cfg         *config.Config
	gwService   *service.GatewayService // model discovery (optional — may be nil)
}

func NewAdminHandler(pg *db.PostgresDB, authService *auth.AuthService, keyStore *provider.KeyStore, costCalc *analytics.CostCalculator, redis *db.RedisClient, allowedIPsStr string, cfg *config.Config, gwService *service.GatewayService) *AdminHandler {
	var allowedIPs []string
	if allowedIPsStr != "" {
		parts := strings.Split(allowedIPsStr, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				allowedIPs = append(allowedIPs, trimmed)
			}
		}
	}

	return &AdminHandler{
		pg:          pg,
		authService: authService,
		keyStore:    keyStore,
		costCalc:    costCalc,
		redis:       redis,
		allowedIPs:  allowedIPs,
		cfg:         cfg,
		gwService:   gwService,
	}
}

func (h *AdminHandler) getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *AdminHandler) isIPAllowed(ipStr string) bool {
	if len(h.allowedIPs) == 0 {
		return true // Allow all if not configured
	}
	clientIP := net.ParseIP(ipStr)
	if clientIP == nil {
		return false
	}
	for _, allowed := range h.allowedIPs {
		if _, ipNet, err := net.ParseCIDR(allowed); err == nil {
			if ipNet.Contains(clientIP) {
				return true
			}
		} else {
			if allowedIP := net.ParseIP(allowed); allowedIP != nil {
				if allowedIP.Equal(clientIP) {
					return true
				}
			}
		}
	}
	return false
}

func (h *AdminHandler) checkAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	// 1. IP Whitelist check
	clientIP := h.getClientIP(r)
	if !h.isIPAllowed(clientIP) {
		http.Error(w, `{"error": "Forbidden: Client IP `+clientIP+` is not allowed"}`, http.StatusForbidden)
		return false
	}

	// 2. Secret Key check
	adminKey := r.Header.Get("X-Admin-API-Key")
	if !h.authService.ValidateAdminKey(adminKey) {
		http.Error(w, `{"error": "Unauthorized admin access"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

// POST /admin/users -> Create a new user
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	user := &model.User{
		Email: req.Email,
		Name:  req.Name,
		Role:  req.Role,
	}

	if err := h.pg.CreateUser(user); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// POST /admin/keys -> Create a new API Key for user
func (h *AdminHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		UserID       string  `json:"user_id"`
		Name         string  `json:"name"`
		BudgetLimit  float64 `json:"budget_limit"`
		RateLimitRPM int     `json:"rate_limit_rpm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, `{"error": "Invalid user_id UUID"}`, http.StatusBadRequest)
		return
	}

	if req.RateLimitRPM <= 0 {
		req.RateLimitRPM = 60
	}

	plainKey, apiKey, err := h.authService.CreateAPIKey(r.Context(), userID, req.Name, req.BudgetLimit, req.RateLimitRPM)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"api_key":  plainKey, // Secret returned only ONCE upon creation!
		"key_info": apiKey,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// orEmpty guards against Go's nil-slice-to-JSON-null encoding: a slice with no
// rows must serialize as [] so JSON consumers never see null where they expect
// an array. Apply at the JSON boundary of every list endpoint.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// GET /admin/users -> List all users
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	users, err := h.pg.ListUsers()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(users))
}

// GET /admin/keys -> List all API keys (key_hash never serialized).
// Optional ?user_id=<uuid> narrows the listing to one user.
func (h *AdminHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var keys []model.APIKey
	var err error
	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		userID, parseErr := uuid.Parse(userIDStr)
		if parseErr != nil {
			http.Error(w, `{"error": "Invalid user_id UUID"}`, http.StatusBadRequest)
			return
		}
		keys, err = h.pg.ListAPIKeysByUser(userID)
	} else {
		keys, err = h.pg.ListAPIKeys()
	}
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(keys))
}

// DELETE /admin/keys?id=<uuid> -> Revoke (deactivate) an API key
func (h *AdminHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, `{"error": "Missing key id parameter"}`, http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid UUID format"}`, http.StatusBadRequest)
		return
	}

	if err := h.authService.RevokeAPIKey(r.Context(), id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "API key revoked successfully"})
}

// PATCH /admin/users?id=<uuid> -> Update a user's name/role. Unset fields are
// merged from the current record so a partial body updates only what's given.
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	id, err := parseIDParam(w, r)
	if err != nil {
		return
	}

	var req struct {
		Name *string `json:"name"`
		Role *string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	current, err := h.pg.GetUserByID(id)
	if err != nil {
		http.Error(w, `{"error": "user not found"}`, http.StatusNotFound)
		return
	}
	name, role := current.Name, current.Role
	if req.Name != nil {
		name = *req.Name
	}
	if req.Role != nil {
		role = *req.Role
	}

	if err := h.pg.UpdateUser(id, name, role); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	updated := *current
	updated.Name = name
	updated.Role = role
	updated.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DELETE /admin/users?id=<uuid> -> Delete a user. Their API keys are removed by
// the ON DELETE CASCADE foreign key; usage_logs are kept (no FK) for audit.
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	id, err := parseIDParam(w, r)
	if err != nil {
		return
	}

	if err := h.pg.DeleteUser(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
}

// PATCH /admin/keys?id=<uuid> -> Edit a key's name/budget/rate-limit/expiry.
// Unset fields keep their current values. expires_at: "" clears the expiry,
// an RFC3339 value sets it.
func (h *AdminHandler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	id, err := parseIDParam(w, r)
	if err != nil {
		return
	}

	var req struct {
		Name         *string  `json:"name"`
		BudgetLimit  *float64 `json:"budget_limit"`
		RateLimitRPM *int     `json:"rate_limit_rpm"`
		ExpiresAt    *string  `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	current, err := h.pg.GetAPIKeyByID(id)
	if err != nil {
		http.Error(w, `{"error": "API key not found"}`, http.StatusNotFound)
		return
	}
	name := current.Name
	budget := current.BudgetLimit
	rpm := current.RateLimitRPM
	expiresAt := current.ExpiresAt
	if req.Name != nil {
		name = *req.Name
	}
	if req.BudgetLimit != nil {
		budget = *req.BudgetLimit
	}
	if req.RateLimitRPM != nil {
		rpm = *req.RateLimitRPM
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			expiresAt = nil
		} else {
			t, parseErr := time.Parse(time.RFC3339, *req.ExpiresAt)
			if parseErr != nil {
				http.Error(w, `{"error": "expires_at must be RFC3339 or empty"}`, http.StatusBadRequest)
				return
			}
			expiresAt = &t
		}
	}

	if err := h.authService.UpdateAPIKey(r.Context(), id, name, budget, rpm, expiresAt); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "API key updated successfully"})
}

// POST /admin/keys/reactivate?id=<uuid> -> Re-enable a revoked API key.
func (h *AdminHandler) ReactivateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	id, err := parseIDParam(w, r)
	if err != nil {
		return
	}

	if err := h.authService.ReactivateAPIKey(r.Context(), id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "API key reactivated successfully"})
}

// GET /admin/logs -> Paginated, filterable usage logs.
// Query: from,to (RFC3339), user_id, provider, model, status ("200"|"2xx"),
// inbound ("openai"|"anthropic"), limit (≤200, default 50), offset (default 0).
func (h *AdminHandler) GetUsageLogs(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	q := r.URL.Query()

	var f model.UsageLogFilter
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = &t
		}
	}
	if v := q.Get("user_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.UserID = &id
		}
	}
	f.Provider = q.Get("provider")
	f.Model = q.Get("model")
	f.Status = q.Get("status")
	f.InboundFormat = q.Get("inbound")

	limit := 50
	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}

	logs, err := h.pg.ListUsageLogs(f, limit, offset)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	total, err := h.pg.CountUsageLogs(f)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":   orEmpty(logs),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GET /admin/system/info -> Redacted runtime configuration for the Settings
// page. Secrets (DB password, admin key, provider master key) are never sent.
func (h *AdminHandler) GetSystemInfo(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	cfg := h.cfg
	if cfg == nil {
		http.Error(w, `{"error": "config unavailable"}`, http.StatusInternalServerError)
		return
	}

	allowedIPs := cfg.AdminAllowedIPs
	if allowedIPs == "" {
		allowedIPs = "ALL"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"env":                         cfg.Env,
		"port":                        cfg.Port,
		"admin_port":                  cfg.AdminPort,
		"db_host":                     cfg.DBHost,
		"db_port":                     cfg.DBPort,
		"db_name":                     cfg.DBName,
		"db_user":                     cfg.DBUser,
		"db_sslmode":                  cfg.DBSSLMode,
		"redis_host":                  cfg.RedisHost,
		"admin_api_key_set":           cfg.AdminAPIKey != "",
		"provider_master_key_enabled": cfg.ProviderMasterKey != "",
		"admin_allowed_ips":           allowedIPs,
		"version":                     "dev",
	})
}

// parseIDParam reads and validates the shared ?id=<uuid> query parameter.
func parseIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, error) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, `{"error": "Missing id parameter"}`, http.StatusBadRequest)
		return uuid.Nil, fmt.Errorf("missing id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid UUID format"}`, http.StatusBadRequest)
		return uuid.Nil, err
	}
	return id, nil
}

// GET /admin/analytics/stats -> Timeseries Usage Analytics Query
func (h *AdminHandler) GetUsageStats(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	query := r.URL.Query()
	interval := query.Get("interval") // "hour" or "day"
	if interval == "" {
		interval = "hour"
	}

	userIDStr := query.Get("user_id")
	var userID *uuid.UUID
	if userIDStr != "" {
		parsed, err := uuid.Parse(userIDStr)
		if err == nil {
			userID = &parsed
		}
	}

	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now

	if fromStr := query.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := query.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	stats, err := h.pg.GetUsageStats(userID, interval, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"interval": interval,
		"data":     orEmpty(stats),
	})
}

// GET /admin/analytics/dashboard -> Consolidated JSON payload for a custom
// dashboard: window totals, time series, top models, per-user usage and the
// most recent requests. Data is read live from usage_logs.
func (h *AdminHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	query := r.URL.Query()
	interval := query.Get("interval") // "hour" or "day"
	if interval == "" {
		interval = "hour"
	}

	var userID *uuid.UUID
	if idStr := query.Get("user_id"); idStr != "" {
		if parsed, err := uuid.Parse(idStr); err == nil {
			userID = &parsed
		}
	}

	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now
	if fromStr := query.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr := query.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	limit := 20 // recent requests
	if lStr := query.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	totals, err := h.pg.GetUsageTotals(userID, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	series, err := h.pg.GetUsageTimeSeries(userID, interval, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	byModel, err := h.pg.GetUsageByModel(userID, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	byUser, err := h.pg.GetUsageByUser(from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	recent, err := h.pg.GetRecentUsageLogs(limit, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"from":        from.Format(time.RFC3339),
		"to":          to.Format(time.RFC3339),
		"interval":    interval,
		"totals":      totals,
		"time_series": orEmpty(series),
		"by_model":    orEmpty(byModel),
		"by_user":     orEmpty(byUser),
		"recent":      orEmpty(recent),
	})
}

// GET /admin/models -> Get list of all model pricings
func (h *AdminHandler) GetModels(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	models, err := h.pg.GetAllModelsList()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(models))
}

// GET /admin/models/catalog -> Registered model_pricings + discovered
// provider_models in one payload. Registered rows carry explicit prices for cost
// calculation; discovered rows are the routing source of truth. The CLI merges
// them so an operator sees the full routable catalog and where each row lives.
func (h *AdminHandler) GetModelCatalog(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	registered, err := h.pg.GetAllModelsList()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	discovered, err := h.pg.ListCatalogModels()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"registered": orEmpty(registered),
		"discovered": orEmpty(discovered),
	})
}

// POST /admin/models -> Create or Update model pricing
func (h *AdminHandler) UpsertModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Provider             string  `json:"provider"`
		ModelName            string  `json:"model_name"`
		PromptPricePer1M     float64 `json:"prompt_price_per_1m"`
		CompletionPricePer1M float64 `json:"completion_price_per_1m"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Provider == "" || req.ModelName == "" {
		http.Error(w, `{"error": "provider and model_name are required"}`, http.StatusBadRequest)
		return
	}

	modelPricing := &model.ModelPricing{
		Provider:             req.Provider,
		ModelName:            req.ModelName,
		PromptPricePer1M:     req.PromptPricePer1M,
		CompletionPricePer1M: req.CompletionPricePer1M,
	}

	if err := h.pg.UpsertModelPricing(modelPricing); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.costCalc != nil {
		_ = h.costCalc.RefreshPricing() // pick up new price immediately
	}
	h.invalidateModelsCache(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(modelPricing)
}

// DELETE /admin/models/{id} -> Delete model pricing
func (h *AdminHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, `{"error": "Missing model id parameter"}`, http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid UUID format"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteModelPricing(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.costCalc != nil {
		_ = h.costCalc.RefreshPricing()
	}
	h.invalidateModelsCache(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Model pricing deleted successfully"})
}

// POST /admin/models/sync -> Pull pricing dynamically from LiteLLM and sync with db
func (h *AdminHandler) SyncModelsFromCatalog(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	providerParam := r.URL.Query().Get("provider")
	if providerParam == "" {
		providerParam = "all"
	}

	var providers []string
	if providerParam == "all" {
		providers = []string{"openai", "anthropic", "gemini"}
		if h.keyStore != nil {
			if active, err := h.keyStore.ListActiveProviders(); err == nil {
				for _, ap := range active {
					found := false
					for _, existing := range providers {
						if existing == ap.Provider {
							found = true
							break
						}
					}
					if !found {
						providers = append(providers, ap.Provider)
					}
				}
			}
		}
	} else {
		providers = []string{providerParam}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	totalSynced := 0

	for _, p := range providers {
		url := fmt.Sprintf("https://api.litellm.ai/model_catalog?provider=%s", p)
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error fetching catalog for %s: %v", p, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Non-OK status from LiteLLM for %s: %d", p, resp.StatusCode)
			continue
		}

		var catalog struct {
			Data []struct {
				ID                 string   `json:"id"`
				Provider           string   `json:"provider"`
				Mode               string   `json:"mode"`
				InputCostPerToken  *float64 `json:"input_cost_per_token"`
				OutputCostPerToken *float64 `json:"output_cost_per_token"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
			log.Printf("Error decoding LiteLLM response for %s: %v", p, err)
			continue
		}

		for _, item := range catalog.Data {
			if item.ID == "" {
				continue
			}

			// Clean provider prefix
			modelName := item.ID
			prefix := p + "/"
			if strings.HasPrefix(modelName, prefix) {
				modelName = strings.TrimPrefix(modelName, prefix)
			}

			var promptPrice, completionPrice float64
			if item.InputCostPerToken != nil {
				promptPrice = (*item.InputCostPerToken) * 1_000_000
			} else {
				promptPrice = 0.0 // Default fallback so user can edit manually
			}

			if item.OutputCostPerToken != nil {
				completionPrice = (*item.OutputCostPerToken) * 1_000_000
			} else {
				completionPrice = 0.0
			}

			modelPricing := &model.ModelPricing{
				Provider:             p,
				ModelName:            modelName,
				PromptPricePer1M:     promptPrice,
				CompletionPricePer1M: completionPrice,
			}

			if err := h.pg.UpsertModelPricing(modelPricing); err != nil {
				log.Printf("Error upserting model pricing for %s/%s: %v", p, modelName, err)
				continue
			}
			totalSynced++
		}
	}

	if h.costCalc != nil {
		_ = h.costCalc.RefreshPricing()
	}
	h.invalidateModelsCache(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"synced_models": totalSynced,
	})
}

// POST /admin/models/discover -> Fetch each configured provider's /models API and
// persist the result into the provider_models catalog (routing source of truth).
// Per-provider failures are reported but do not fail the whole request.
func (h *AdminHandler) DiscoverModels(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if h.gwService == nil {
		http.Error(w, `{"error": "model discovery is not available"}`, http.StatusServiceUnavailable)
		return
	}

	total, errs := h.gwService.DiscoverAllModels(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"synced_models": total,
		"errors":        errs,
	})
}

// GET /admin/providers -> List provider credentials (secret redacted)
func (h *AdminHandler) ListProviderKeys(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	keys, err := h.keyStore.ListKeys()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(keys))
}

// builtinProviders have a hardcoded upstream URL in the provider client; they
// must not carry a custom base_url.
var builtinProviders = map[string]bool{
	"openai":    true,
	"anthropic": true,
	"gemini":    true,
}

// providerNameRe constrains provider names to the routing prefix grammar:
// a lowercase letter/digit, followed by [a-z0-9_-]. '/' is excluded because it
// is the model-prefix separator.
var providerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// POST /admin/providers -> Create/Update a provider API key (stored statefully)
func (h *AdminHandler) SetProviderKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		BaseURL  string `json:"base_url"`
		Protocol string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.APIKey == "" {
		http.Error(w, `{"error": "provider and api_key are required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Provider) > 50 || !providerNameRe.MatchString(req.Provider) {
		http.Error(w, `{"error": "provider must be <=50 chars: start with a lowercase letter/digit, then [a-z0-9_-], no '/'"}`, http.StatusBadRequest)
		return
	}

	// Built-in providers keep their hardcoded upstream URL; only custom
	// (OpenAI/Anthropic-compatible) providers may set a base_url + protocol.
	isBuiltin := builtinProviders[req.Provider]
	baseURL := strings.TrimSpace(req.BaseURL)
	if isBuiltin && baseURL != "" {
		http.Error(w, `{"error": "built-in provider "`+req.Provider+`" has a fixed upstream URL; base_url is only for custom providers"}`, http.StatusBadRequest)
		return
	}
	if !isBuiltin && baseURL == "" {
		http.Error(w, `{"error": "custom providers require a non-empty base_url"}`, http.StatusBadRequest)
		return
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = "openai"
	}
	if protocol != "openai" && protocol != "anthropic" {
		http.Error(w, `{"error": "protocol must be \"openai\" or \"anthropic\""}`, http.StatusBadRequest)
		return
	}

	if err := h.keyStore.SetKey(r.Context(), req.Provider, req.APIKey, baseURL, protocol); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	h.invalidateModelsCache(r.Context())

	// Kick off model discovery so the new provider's models land in the catalog
	// (routing source of truth) without an admin having to trigger it manually.
	if h.gwService != nil {
		go func() {
			dctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := h.gwService.DiscoverProvider(dctx, req.Provider); err != nil {
				log.Printf("Warning: model discovery for %s failed: %v", req.Provider, err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Provider API key saved successfully", "provider": req.Provider})
}

// DELETE /admin/providers?provider=openai -> Delete a provider key
func (h *AdminHandler) DeleteProviderKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, `{"error": "Missing provider parameter"}`, http.StatusBadRequest)
		return
	}

	if err := h.keyStore.DeleteKey(r.Context(), providerName); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	h.invalidateModelsCache(r.Context())

	// Deleting a provider cascades its provider_models rows (FK); refresh the
	// routing cache so those models stop resolving to the removed provider.
	if h.costCalc != nil {
		_ = h.costCalc.RefreshCatalog()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Provider key deleted successfully", "provider": providerName})
}

// invalidateModelsCache drops the cached public model list so the next
// GET /v1/models re-fetches live from providers with the updated keys/pricing.
func (h *AdminHandler) invalidateModelsCache(ctx context.Context) {
	if h.redis != nil {
		_ = h.redis.InvalidateModelsCache(ctx)
	}
}
