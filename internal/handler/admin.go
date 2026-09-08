package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	"llm-gateway/internal/permission"
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
	permEngine  *permission.PermissionEngine
}

func NewAdminHandler(pg *db.PostgresDB, authService *auth.AuthService, keyStore *provider.KeyStore, costCalc *analytics.CostCalculator, redis *db.RedisClient, allowedIPsStr string, cfg *config.Config, gwService *service.GatewayService, permEngine *permission.PermissionEngine) *AdminHandler {
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
		permEngine:  permEngine,
	}
}

type AuthContext struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Name   string    `json:"name"`
	Role   string    `json:"role"`
	IsRoot bool      `json:"is_root"`
}

type authContextKey struct{}

func (h *AdminHandler) logAudit(r *http.Request, actorType, action, targetType, targetID string, details interface{}) {
	if h.pg == nil {
		return
	}
	clientIP := h.getClientIP(r)
	var rawDetails json.RawMessage
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			rawDetails = b
		}
	}
	var actorID *uuid.UUID
	ac := h.getAuthContext(r)
	if ac.UserID != uuid.Nil {
		actorID = &ac.UserID
	}
	if ac.IsRoot {
		actorType = "root"
	} else if ac.Email != "" {
		actorType = ac.Role
	}
	_ = h.pg.CreateAuditLog(&model.AuditLog{
		ActorID:    actorID,
		ActorType:  actorType,
		ActorIP:    clientIP,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    rawDetails,
	})
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

	// 2. Secret Key check (Break-glass Root Admin)
	adminKey := r.Header.Get("X-Admin-API-Key")
	if adminKey != "" && h.authService.ValidateAdminKey(adminKey) {
		ctx := context.WithValue(r.Context(), authContextKey{}, &AuthContext{
			Email:  "root",
			Name:   "Root Administrator",
			Role:   "admin",
			IsRoot: true,
		})
		*r = *r.WithContext(ctx)
		return true
	}

	// 3. Bearer Session Token check
	token := ""
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	} else if sessHeader := r.Header.Get("X-Session-Token"); sessHeader != "" {
		token = sessHeader
	}

	if token != "" {
		claims, err := h.authService.VerifySessionToken(token)
		if err == nil && claims != nil {
			ctx := context.WithValue(r.Context(), authContextKey{}, &AuthContext{
				UserID: claims.UserID,
				Email:  claims.Email,
				Name:   claims.Name,
				Role:   claims.Role,
				IsRoot: claims.IsRoot,
			})
			*r = *r.WithContext(ctx)
			return true
		}
	}

	http.Error(w, `{"error": "Unauthorized admin access"}`, http.StatusUnauthorized)
	return false
}

func (h *AdminHandler) getAuthContext(r *http.Request) *AuthContext {
	if val := r.Context().Value(authContextKey{}); val != nil {
		if ac, ok := val.(*AuthContext); ok {
			return ac
		}
	}
	return &AuthContext{Role: "user"}
}

// POST /admin/users -> Create a new user
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Email      string  `json:"email"`
		Name       string  `json:"name"`
		Role       string  `json:"role"`
		IsRoot     bool    `json:"is_root"`
		TenantID   *string `json:"tenant_id"`
		TenantRole string  `json:"tenant_role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	var tid *uuid.UUID
	if req.TenantID != nil && strings.TrimSpace(*req.TenantID) != "" {
		if parsed, err := uuid.Parse(strings.TrimSpace(*req.TenantID)); err == nil {
			tid = &parsed
		}
	}

	user := &model.User{
		Email:      req.Email,
		Name:       req.Name,
		Role:       req.Role,
		IsRoot:     req.IsRoot,
		TenantID:   tid,
		TenantRole: req.TenantRole,
	}

	if err := h.pg.CreateUser(user); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "user.create", "user", user.ID.String(), map[string]interface{}{
		"email":       user.Email,
		"name":        user.Name,
		"role":        user.Role,
		"is_root":     user.IsRoot,
		"tenant_id":   user.TenantID,
		"tenant_role": user.TenantRole,
	})

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
		UserID          string  `json:"user_id"`
		TenantID        *string `json:"tenant_id"`
		Name            string  `json:"name"`
		BudgetLimit     float64 `json:"budget_limit"`
		RateLimitRPM    int     `json:"rate_limit_rpm"`
		CacheTTLSeconds *int    `json:"cache_ttl_seconds"`
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

	var tid *uuid.UUID
	if req.TenantID != nil && strings.TrimSpace(*req.TenantID) != "" {
		if parsed, err := uuid.Parse(strings.TrimSpace(*req.TenantID)); err == nil {
			tid = &parsed
		}
	}

	if req.RateLimitRPM <= 0 {
		req.RateLimitRPM = 60
	}

	cacheTTL := 0
	if req.CacheTTLSeconds != nil {
		cacheTTL = *req.CacheTTLSeconds
	}

	plainKey, apiKey, err := h.authService.CreateAPIKey(r.Context(), userID, req.Name, req.BudgetLimit, req.RateLimitRPM, cacheTTL, tid)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"api_key":  plainKey, // Secret returned only ONCE upon creation!
		"key_info": apiKey,
	}

	h.logAudit(r, "admin", "key.create", "key", apiKey.ID.String(), map[string]interface{}{
		"name":           apiKey.Name,
		"user_id":        apiKey.UserID,
		"budget_limit":   apiKey.BudgetLimit,
		"rate_limit_rpm": apiKey.RateLimitRPM,
	})

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

	h.logAudit(r, "admin", "key.revoke", "key", id.String(), nil)

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
		Name       *string `json:"name"`
		Role       *string `json:"role"`
		IsRoot     *bool   `json:"is_root"`
		TenantID   *string `json:"tenant_id"`
		TenantRole *string `json:"tenant_role"`
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
	name, role, isRoot := current.Name, current.Role, current.IsRoot
	if req.Name != nil {
		name = *req.Name
	}
	if req.Role != nil {
		role = *req.Role
	}
	if req.IsRoot != nil {
		isRoot = *req.IsRoot
	}

	if err := h.pg.UpdateUser(id, name, role, isRoot); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	tenantID := current.TenantID
	tenantRole := current.TenantRole
	if req.TenantID != nil {
		trimmed := strings.TrimSpace(*req.TenantID)
		if trimmed == "" {
			tenantID = nil
		} else if parsed, err := uuid.Parse(trimmed); err == nil {
			tenantID = &parsed
		}
	}
	if req.TenantRole != nil {
		tenantRole = *req.TenantRole
	}
	if req.TenantID != nil || req.TenantRole != nil {
		_ = h.pg.UpdateUserTenant(id, tenantID, tenantRole)
	}

	h.logAudit(r, "admin", "user.update", "user", id.String(), req)

	updated := *current
	updated.Name = name
	updated.Role = role
	updated.IsRoot = isRoot
	updated.TenantID = tenantID
	updated.TenantRole = tenantRole
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

	h.logAudit(r, "admin", "user.delete", "user", id.String(), nil)

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
		Name            *string  `json:"name"`
		BudgetLimit     *float64 `json:"budget_limit"`
		RateLimitRPM    *int     `json:"rate_limit_rpm"`
		CacheTTLSeconds *int     `json:"cache_ttl_seconds"`
		ExpiresAt       *string  `json:"expires_at"`
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
	cacheTTL := current.CacheTTLSeconds
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
	if req.CacheTTLSeconds != nil {
		cacheTTL = *req.CacheTTLSeconds
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

	if err := h.authService.UpdateAPIKey(r.Context(), id, name, budget, rpm, cacheTTL, expiresAt); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "key.update", "key", id.String(), req)

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

	h.logAudit(r, "admin", "key.reactivate", "key", id.String(), nil)

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
	if v := q.Get("key_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.KeyID = &id
		}
	}
	f.Provider = q.Get("provider")
	f.Model = q.Get("model")
	f.Status = q.Get("status")
	f.InboundFormat = q.Get("inbound")
	f.Modality = q.Get("modality")
	f.StructureHash = q.Get("structure_hash")
	if v := q.Get("has_images"); v != "" {
		b := v == "true" || v == "1"
		f.HasImages = &b
	}
	if v := q.Get("has_audio"); v != "" {
		b := v == "true" || v == "1"
		f.HasAudio = &b
	}

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

// GET /admin/logs/{id} -> Single usage log detail with user & key metadata
func (h *AdminHandler) GetUsageLogDetail(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	if idStr == "" {
		http.Error(w, `{"error": "Missing log ID"}`, http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error": "Invalid log ID"}`, http.StatusBadRequest)
		return
	}

	logDetail, err := h.pg.GetUsageLogByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error": "Log entry not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logDetail)
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

	var filter model.DashboardFilter
	if idStr := query.Get("user_id"); idStr != "" {
		if parsed, err := uuid.Parse(idStr); err == nil {
			filter.UserID = &parsed
		}
	}
	if keyStr := query.Get("key_id"); keyStr != "" {
		if parsed, err := uuid.Parse(keyStr); err == nil {
			filter.KeyID = &parsed
		}
	}
	filter.Provider = query.Get("provider")
	filter.Model = query.Get("model")

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

	totals, err := h.pg.GetUsageTotals(filter, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	series, err := h.pg.GetUsageTimeSeries(filter, interval, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	byModel, err := h.pg.GetUsageByModel(filter, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	byUser, err := h.pg.GetUsageByUser(filter, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	recent, err := h.pg.GetRecentUsageLogs(filter, limit, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	statusBreakdown, err := h.pg.GetStatusCodeBreakdown(filter, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	modalityBreakdown, err := h.pg.GetModalityBreakdown(filter, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"from":               from.Format(time.RFC3339),
		"to":                 to.Format(time.RFC3339),
		"interval":           interval,
		"totals":             totals,
		"time_series":        orEmpty(series),
		"by_model":           orEmpty(byModel),
		"by_user":            orEmpty(byUser),
		"recent":             orEmpty(recent),
		"status_breakdown":   statusBreakdown,
		"modality_breakdown": modalityBreakdown,
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
		Provider             string   `json:"provider"`
		ModelName            string   `json:"model_name"`
		PromptPricePer1M     float64  `json:"prompt_price_per_1m"`
		CompletionPricePer1M float64  `json:"completion_price_per_1m"`
		CacheReadPricePer1M  *float64 `json:"cache_read_price_per_1m,omitempty"`
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
		CacheReadPricePer1M:  req.CacheReadPricePer1M,
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
		Provider         string `json:"provider"`
		APIKey           string `json:"api_key"`
		BaseURL          string `json:"base_url"`
		Protocol         string `json:"protocol"`
		FallbackPriority *int   `json:"fallback_priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		http.Error(w, `{"error": "provider is required"}`, http.StatusBadRequest)
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

	if err := h.keyStore.SetKey(r.Context(), req.Provider, req.APIKey, baseURL, protocol, req.FallbackPriority); err != nil {
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

	h.logAudit(r, "admin", "provider.set", "provider", req.Provider, map[string]interface{}{
		"provider": req.Provider,
		"base_url": baseURL,
		"protocol": protocol,
	})

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

	h.logAudit(r, "admin", "provider.delete", "provider", providerName, nil)

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

// ---------------------------------------------------------------------------
// Member Authentication & User Profile
// ---------------------------------------------------------------------------

// POST /admin/auth/login
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	clientIP := h.getClientIP(r)
	if !h.isIPAllowed(clientIP) {
		http.Error(w, `{"error": "Forbidden: Client IP `+clientIP+` is not allowed"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Password = strings.TrimSpace(req.Password)
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error": "Email and password are required"}`, http.StatusBadRequest)
		return
	}

	var user *model.User
	var err error

	// Check if this is an emergency break-glass login using the admin key as password or master admin email
	isEmergencyRoot := (req.Email == "root" || req.Email == "admin" || req.Email == "admin@llm-gateway.internal" || req.Email == "admin@llm-gateway.local") && h.authService.ValidateAdminKey(req.Password)

	if isEmergencyRoot {
		user, err = h.pg.GetUserByEmail(req.Email)
		if err != nil {
			user = &model.User{
				Email:    req.Email,
				Name:     "Root Administrator",
				Role:     "admin",
				IsRoot:   true,
				IsActive: true,
			}
		}
	} else {
		user, err = h.pg.GetUserByEmail(req.Email)
		if err != nil {
			http.Error(w, `{"error": "Invalid email or password"}`, http.StatusUnauthorized)
			return
		}

		if !user.IsActive {
			http.Error(w, `{"error": "Account is inactive"}`, http.StatusForbidden)
			return
		}

		valid := false
		if user.PasswordHash == "" {
			// If password hash not yet set, allow master admin key or default 'admin' if user is admin
			if h.authService.ValidateAdminKey(req.Password) || (user.Role == "admin" && req.Password == "admin") {
				valid = true
			}
		} else {
			valid = h.authService.VerifyPassword(req.Password, user.PasswordHash)
		}

		if !valid {
			http.Error(w, `{"error": "Invalid email or password"}`, http.StatusUnauthorized)
			return
		}
	}

	token, err := h.authService.CreateSessionToken(user, 24*time.Hour)
	if err != nil {
		http.Error(w, `{"error": "Failed to create session token"}`, http.StatusInternalServerError)
		return
	}

	groups, _ := h.pg.GetUserGroups(user.ID)
	groupNames := make([]string, 0, len(groups))
	for _, g := range groups {
		groupNames = append(groupNames, g.Name)
	}

	h.logAudit(r, "user", "auth.login", "user", user.ID.String(), map[string]interface{}{
		"email": user.Email,
		"role":  user.Role,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":      user.ID,
			"email":   user.Email,
			"name":    user.Name,
			"role":    user.Role,
			"is_root": user.IsRoot,
			"groups":  groupNames,
		},
	})
}

// GET /admin/auth/me
func (h *AdminHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	ac := h.getAuthContext(r)
	groups, _ := h.pg.GetUserGroups(ac.UserID)
	groupNames := make([]string, 0, len(groups))
	for _, g := range groups {
		groupNames = append(groupNames, g.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      ac.UserID,
		"email":   ac.Email,
		"name":    ac.Name,
		"role":    ac.Role,
		"is_root": ac.IsRoot,
		"groups":  groupNames,
	})
}

// POST /admin/auth/password
func (h *AdminHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		http.Error(w, `{"error": "Password must be at least 6 characters"}`, http.StatusBadRequest)
		return
	}

	ac := h.getAuthContext(r)
	targetID := ac.UserID
	if req.UserID != "" {
		parsed, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, `{"error": "Invalid user_id"}`, http.StatusBadRequest)
			return
		}
		// Non-root or non-admin can only change their own password
		if parsed != ac.UserID && !ac.IsRoot && ac.Role != "admin" {
			http.Error(w, `{"error": "Permission denied: cannot change another user's password"}`, http.StatusForbidden)
			return
		}
		targetID = parsed
	}

	hash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error": "Failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	if err := h.pg.SetUserPassword(targetID, hash); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "user", "auth.password_update", "user", targetID.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}

// ---------------------------------------------------------------------------
// Member & Team Usage Analytics ("ai dùng gì, dùng bao nhiêu")
// ---------------------------------------------------------------------------

// GET /admin/analytics/members
func (h *AdminHandler) GetMemberUsageAnalytics(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	query := r.URL.Query()
	now := time.Now()
	from := now.Add(-7 * 24 * time.Hour)
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

	groupID := query.Get("group_id")
	search := query.Get("search")

	analyticsResp, err := h.pg.GetMemberUsageAnalytics(from, to, groupID, search)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analyticsResp)
}

// GET /admin/analytics/members/{id}
func (h *AdminHandler) GetMemberDetail(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	uid, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid member id"}`, http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	now := time.Now()
	from := now.Add(-7 * 24 * time.Hour)
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

	detail, err := h.pg.GetMemberDetail(uid, from, to)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// GET /admin/iam/actions -> Return catalog of available IAM actions for policy builders
func (h *AdminHandler) GetIAMActions(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(permission.AllActions)
}

