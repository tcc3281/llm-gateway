package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/auth"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/provider"

	"github.com/google/uuid"
)

type AdminHandler struct {
	pg          *db.PostgresDB
	authService *auth.AuthService
	keyStore    *provider.KeyStore
	costCalc    *analytics.CostCalculator
	redis       *db.RedisClient
}

func NewAdminHandler(pg *db.PostgresDB, authService *auth.AuthService, keyStore *provider.KeyStore, costCalc *analytics.CostCalculator, redis *db.RedisClient) *AdminHandler {
	return &AdminHandler{
		pg:          pg,
		authService: authService,
		keyStore:    keyStore,
		costCalc:    costCalc,
		redis:       redis,
	}
}

func (h *AdminHandler) checkAdminAuth(w http.ResponseWriter, r *http.Request) bool {
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
		"api_key":   plainKey, // Secret returned only ONCE upon creation!
		"key_info":  apiKey,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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
	json.NewEncoder(w).Encode(users)
}

// GET /admin/keys -> List all API keys (key_hash never serialized)
func (h *AdminHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	keys, err := h.pg.ListAPIKeys()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
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
		"from":      from.Format(time.RFC3339),
		"to":        to.Format(time.RFC3339),
		"interval":  interval,
		"data":      stats,
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
	json.NewEncoder(w).Encode(models)
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
	json.NewEncoder(w).Encode(keys)
}

// POST /admin/providers -> Create/Update a provider API key (stored statefully)
func (h *AdminHandler) SetProviderKey(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.APIKey == "" {
		http.Error(w, `{"error": "provider and api_key are required"}`, http.StatusBadRequest)
		return
	}

	if err := h.keyStore.SetKey(r.Context(), req.Provider, req.APIKey); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	h.invalidateModelsCache(r.Context())

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

