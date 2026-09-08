package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"llm-gateway/internal/model"
	"llm-gateway/internal/permission"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Virtual Models & Dynamic Routing Endpoints (v0.3.0)
// ---------------------------------------------------------------------------

// checkIAMAction verifies that the authenticated user has permission to perform action.
func (h *AdminHandler) checkIAMAction(w http.ResponseWriter, r *http.Request, action string) bool {
	ac := h.getAuthContext(r)
	if ac.IsRoot {
		return true
	}
	if h.permEngine == nil {
		return true
	}
	allowed, reason, err := h.permEngine.Check(r.Context(), ac.UserID, action, "*")
	if err != nil || !allowed {
		http.Error(w, fmt.Sprintf(`{"error": "Forbidden: missing required IAM permission %s (%s)"}`, action, reason), http.StatusForbidden)
		return false
	}
	return true
}

// GET /admin/virtual-models -> List all virtual models (optionally filtered by tenant_id)
func (h *AdminHandler) ListVirtualModels(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionVirtualModelRead) {
		return
	}

	var tenantID *uuid.UUID
	tenantQuery := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if tenantQuery != "" {
		if tid, err := uuid.Parse(tenantQuery); err == nil {
			tenantID = &tid
		}
	}

	vms, err := h.pg.ListVirtualModels(tenantID)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(vms))
}

// GET /admin/virtual-models/{id} -> Get virtual model details
func (h *AdminHandler) GetVirtualModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionVirtualModelRead) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid virtual model ID"}`, http.StatusBadRequest)
		return
	}

	vm, err := h.pg.GetVirtualModelByID(id)
	if err != nil {
		http.Error(w, `{"error": "Virtual model not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vm)
}

// POST /admin/virtual-models -> Create a new Virtual Model
func (h *AdminHandler) CreateVirtualModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionVirtualModelCreate) {
		return
	}

	var req model.CreateVirtualModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, `{"error": "Virtual model name is required"}`, http.StatusBadRequest)
		return
	}

	// Collision check: Prevent accidental shadowing of real models in the catalog
	if !req.AllowOverride && h.pg != nil {
		if exists, prov, _ := h.pg.CheckModelNameInCatalog(name); exists {
			http.Error(w, fmt.Sprintf(`{"error": "Model alias %q conflicts with an existing model from provider %q. Set 'allow_override': true if you intentionally want to shadow this provider model."}`, name, prov), http.StatusConflict)
			return
		}
	}

	strategy := strings.TrimSpace(req.RoutingStrategy)
	if strategy == "" {
		strategy = "weighted"
	} else if strategy != "weighted" && strategy != "fallback" && strategy != "round_robin" {
		http.Error(w, `{"error": "RoutingStrategy must be 'weighted', 'fallback', or 'round_robin'"}`, http.StatusBadRequest)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var targets []model.VirtualModelTarget
	for _, t := range req.Targets {
		providerName := strings.TrimSpace(t.Provider)
		targetModel := strings.TrimSpace(t.ModelName)
		if providerName == "" || targetModel == "" {
			continue
		}
		weight := t.Weight
		if weight <= 0 {
			weight = 100
		}
		priority := t.Priority
		if priority <= 0 {
			priority = 1
		}
		targetActive := true
		if t.IsActive != nil {
			targetActive = *t.IsActive
		}
		targets = append(targets, model.VirtualModelTarget{
			Provider:  providerName,
			ModelName: targetModel,
			Weight:    weight,
			Priority:  priority,
			IsActive:  targetActive,
		})
	}

	if len(targets) == 0 {
		http.Error(w, `{"error": "At least one valid target provider/model is required"}`, http.StatusBadRequest)
		return
	}

	vm := model.VirtualModel{
		ID:              uuid.New(),
		Name:            name,
		Description:     strings.TrimSpace(req.Description),
		TenantID:        req.TenantID,
		RoutingStrategy: strategy,
		IsActive:        isActive,
		Targets:         targets,
	}

	if err := h.pg.CreateVirtualModel(&vm); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "create_virtual_model", "virtual_model", vm.ID.String(), map[string]interface{}{
		"name":             vm.Name,
		"routing_strategy": vm.RoutingStrategy,
		"target_count":     len(vm.Targets),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(vm)
}

// PUT /admin/virtual-models/{id} -> Update an existing Virtual Model
func (h *AdminHandler) UpdateVirtualModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionVirtualModelUpdate) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid virtual model ID"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.pg.GetVirtualModelByID(id)
	if err != nil {
		http.Error(w, `{"error": "Virtual model not found"}`, http.StatusNotFound)
		return
	}

	var req model.UpdateVirtualModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		newName := strings.TrimSpace(*req.Name)
		if newName != existing.Name {
			allowOverride := req.AllowOverride != nil && *req.AllowOverride
			if !allowOverride && h.pg != nil {
				if exists, prov, _ := h.pg.CheckModelNameInCatalog(newName); exists {
					http.Error(w, fmt.Sprintf(`{"error": "Model alias %q conflicts with an existing model from provider %q. Set 'allow_override': true if you intentionally want to shadow this provider model."}`, newName, prov), http.StatusConflict)
					return
				}
			}
			existing.Name = newName
		}
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.TenantID != nil {
		existing.TenantID = req.TenantID
	}
	if req.RoutingStrategy != nil && strings.TrimSpace(*req.RoutingStrategy) != "" {
		strategy := strings.TrimSpace(*req.RoutingStrategy)
		if strategy == "weighted" || strategy == "fallback" || strategy == "round_robin" {
			existing.RoutingStrategy = strategy
		}
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if req.Targets != nil {
		var targets []model.VirtualModelTarget
		for _, t := range req.Targets {
			providerName := strings.TrimSpace(t.Provider)
			targetModel := strings.TrimSpace(t.ModelName)
			if providerName == "" || targetModel == "" {
				continue
			}
			weight := t.Weight
			if weight <= 0 {
				weight = 100
			}
			priority := t.Priority
			if priority <= 0 {
				priority = 1
			}
			targetActive := true
			if t.IsActive != nil {
				targetActive = *t.IsActive
			}
			targets = append(targets, model.VirtualModelTarget{
				VirtualModelID: existing.ID,
				Provider:       providerName,
				ModelName:      targetModel,
				Weight:         weight,
				Priority:       priority,
				IsActive:       targetActive,
			})
		}
		if len(targets) > 0 {
			existing.Targets = targets
		}
	}

	if err := h.pg.UpdateVirtualModel(existing); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "update_virtual_model", "virtual_model", id.String(), map[string]interface{}{
		"name":             existing.Name,
		"routing_strategy": existing.RoutingStrategy,
		"target_count":     len(existing.Targets),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

// DELETE /admin/virtual-models/{id} -> Delete a virtual model
func (h *AdminHandler) DeleteVirtualModel(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionVirtualModelDelete) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid virtual model ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteVirtualModel(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "delete_virtual_model", "virtual_model", id.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "deleted"}`))
}
