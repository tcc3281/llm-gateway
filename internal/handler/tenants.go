package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Multi-Tenant B2B Management Endpoints
// ---------------------------------------------------------------------------

// GET /admin/tenants -> List all tenants
func (h *AdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	tenants, err := h.pg.ListTenants()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(tenants))
}

// POST /admin/tenants -> Create a new Tenant
func (h *AdminHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Name               string   `json:"name"`
		Type               string   `json:"type"` // "internal" | "external"
		BillingMode        string   `json:"billing_mode"` // "prepaid" | "postpaid"
		BudgetLimitMonthly float64  `json:"budget_limit_monthly"`
		AllowedModels      []string `json:"allowed_models"`
		IsActive           *bool    `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, `{"error": "Tenant name is required"}`, http.StatusBadRequest)
		return
	}

	tenantType := strings.TrimSpace(req.Type)
	if tenantType == "" {
		tenantType = "external"
	} else if tenantType != "internal" && tenantType != "external" {
		http.Error(w, `{"error": "Type must be 'internal' or 'external'"}`, http.StatusBadRequest)
		return
	}

	billingMode := strings.TrimSpace(req.BillingMode)
	if billingMode == "" {
		billingMode = "prepaid"
	} else if billingMode != "prepaid" && billingMode != "postpaid" {
		http.Error(w, `{"error": "BillingMode must be 'prepaid' or 'postpaid'"}`, http.StatusBadRequest)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	allowedModels := req.AllowedModels
	if allowedModels == nil {
		allowedModels = []string{}
	}

	tenant := model.Tenant{
		ID:                 uuid.New(),
		Name:               name,
		Type:               tenantType,
		BillingMode:        billingMode,
		CreditBalance:      0.0,
		BudgetLimitMonthly: req.BudgetLimitMonthly,
		AllowedModels:      allowedModels,
		IsActive:           isActive,
	}

	if err := h.pg.CreateTenant(&tenant); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "create_tenant", "tenant", tenant.ID.String(), map[string]interface{}{
		"name":         tenant.Name,
		"type":         tenant.Type,
		"billing_mode": tenant.BillingMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// GET /admin/tenants/{id} -> Get tenant details
func (h *AdminHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid tenant ID"}`, http.StatusBadRequest)
		return
	}

	tenant, err := h.pg.GetTenantByID(id)
	if err != nil {
		http.Error(w, `{"error": "Tenant not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// PATCH /admin/tenants/{id} -> Update tenant properties
func (h *AdminHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid tenant ID"}`, http.StatusBadRequest)
		return
	}

	var patch model.TenantPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if patch.Type != nil {
		t := strings.TrimSpace(*patch.Type)
		if t != "internal" && t != "external" {
			http.Error(w, `{"error": "Type must be 'internal' or 'external'"}`, http.StatusBadRequest)
			return
		}
		patch.Type = &t
	}

	if patch.BillingMode != nil {
		bm := strings.TrimSpace(*patch.BillingMode)
		if bm != "prepaid" && bm != "postpaid" {
			http.Error(w, `{"error": "BillingMode must be 'prepaid' or 'postpaid'"}`, http.StatusBadRequest)
			return
		}
		patch.BillingMode = &bm
	}

	if err := h.pg.UpdateTenant(id, &patch); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	updated, err := h.pg.GetTenantByID(id)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch updated tenant"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "update_tenant", "tenant", id.String(), patch)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DELETE /admin/tenants/{id} -> Soft or hard delete tenant
func (h *AdminHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid tenant ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteTenant(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "delete_tenant", "tenant", id.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "deleted"}`))
}

// POST /admin/tenants/{id}/topup -> Top up prepaid credit via contract or adjustment
func (h *AdminHandler) TopUpCredit(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid tenant ID"}`, http.StatusBadRequest)
		return
	}

	var req model.TopUpCreditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, `{"error": "Top-up amount must be greater than 0"}`, http.StatusBadRequest)
		return
	}

	var actorID *uuid.UUID
	ac := h.getAuthContext(r)
	if ac.UserID != uuid.Nil {
		actorID = &ac.UserID
	}

	txType := "contract_deposit"
	if strings.TrimSpace(req.ContractRef) == "" {
		txType = "admin_adjustment"
	}

	tx, err := h.pg.TopUpTenantCredit(id, req.Amount, txType, req.ContractRef, req.Notes, actorID)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "topup_tenant_credit", "tenant", id.String(), map[string]interface{}{
		"amount":        req.Amount,
		"contract_ref":  req.ContractRef,
		"notes":         req.Notes,
		"balance_after": tx.BalanceAfter,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tx)
}

// GET /admin/tenants/{id}/transactions -> List credit transactions for a tenant
func (h *AdminHandler) ListTenantTransactions(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid tenant ID"}`, http.StatusBadRequest)
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	txs, total, err := h.pg.ListTenantTransactions(id, limit, offset)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":   orEmpty(txs),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
