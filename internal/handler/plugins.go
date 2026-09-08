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
// Plugin Catalog & Scope Bindings Handlers (v0.3.0)
// ---------------------------------------------------------------------------

// GET /admin/plugins -> List all plugins in the catalog
func (h *AdminHandler) ListPlugins(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginRead) {
		return
	}

	plugins, err := h.pg.ListPlugins()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(plugins))
}

// GET /admin/plugins/{name} -> Get plugin details by name
func (h *AdminHandler) GetPlugin(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginRead) {
		return
	}

	name := r.PathValue("name")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		http.Error(w, `{"error": "Plugin name is required"}`, http.StatusBadRequest)
		return
	}

	p, err := h.pg.GetPluginByName(name)
	if err != nil {
		http.Error(w, `{"error": "Plugin not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// GET /admin/plugin-bindings -> List active plugin bindings (optionally filtered)
func (h *AdminHandler) ListPluginBindings(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginRead) {
		return
	}

	var tenantID *uuid.UUID
	tenantQuery := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if tenantQuery != "" {
		if tid, err := uuid.Parse(tenantQuery); err == nil {
			tenantID = &tid
		}
	}

	pluginName := strings.TrimSpace(r.URL.Query().Get("plugin_name"))

	bindings, err := h.pg.ListPluginBindings(tenantID, pluginName)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(bindings))
}

// GET /admin/plugin-bindings/{id} -> Get a single plugin binding
func (h *AdminHandler) GetPluginBinding(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginRead) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid binding ID"}`, http.StatusBadRequest)
		return
	}

	binding, err := h.pg.GetPluginBindingByID(id)
	if err != nil {
		http.Error(w, `{"error": "Plugin binding not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(binding)
}

// POST /admin/plugin-bindings -> Bind a plugin to a scope
func (h *AdminHandler) CreatePluginBinding(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginCreate) {
		return
	}

	var req model.CreatePluginBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	pluginName := strings.TrimSpace(req.PluginName)
	if pluginName == "" {
		http.Error(w, `{"error": "Plugin name is required"}`, http.StatusBadRequest)
		return
	}

	// Verify plugin exists
	if _, err := h.pg.GetPluginByName(pluginName); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Plugin %q does not exist in catalog"}`, pluginName), http.StatusBadRequest)
		return
	}

	scopeLevel := strings.ToLower(strings.TrimSpace(req.ScopeLevel))
	validScopes := map[string]bool{
		"global":   true,
		"provider": true,
		"model":    true,
		"tenant":   true,
		"role":     true,
		"user":     true,
		"api_key":  true,
	}
	if !validScopes[scopeLevel] {
		http.Error(w, `{"error": "Invalid scope_level. Must be one of: global, provider, model, tenant, role, user, api_key"}`, http.StatusBadRequest)
		return
	}

	// Validate required scope targets
	switch scopeLevel {
	case "provider":
		if req.ScopeProvider == nil || strings.TrimSpace(*req.ScopeProvider) == "" {
			http.Error(w, `{"error": "scope_provider is required for provider scope"}`, http.StatusBadRequest)
			return
		}
	case "model":
		if req.ScopeModel == nil || strings.TrimSpace(*req.ScopeModel) == "" {
			http.Error(w, `{"error": "scope_model is required for model scope"}`, http.StatusBadRequest)
			return
		}
	case "tenant":
		if req.ScopeTenantID == nil {
			http.Error(w, `{"error": "scope_tenant_id is required for tenant scope"}`, http.StatusBadRequest)
			return
		}
	case "role":
		if req.ScopeRole == nil || strings.TrimSpace(*req.ScopeRole) == "" {
			http.Error(w, `{"error": "scope_role is required for role scope"}`, http.StatusBadRequest)
			return
		}
	case "user":
		if req.ScopeUserID == nil {
			http.Error(w, `{"error": "scope_user_id is required for user scope"}`, http.StatusBadRequest)
			return
		}
	case "api_key":
		if req.ScopeKeyID == nil {
			http.Error(w, `{"error": "scope_key_id is required for api_key scope"}`, http.StatusBadRequest)
			return
		}
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	priority := 100
	if req.Priority != nil {
		priority = *req.Priority
	}

	configJSON := req.Config
	if len(configJSON) == 0 {
		configJSON = json.RawMessage("{}")
	}

	ac := h.getAuthContext(r)
	var createdBy *uuid.UUID
	if ac.UserID != uuid.Nil {
		createdBy = &ac.UserID
	}

	b := model.PluginBinding{
		PluginName:    pluginName,
		ScopeLevel:    scopeLevel,
		ScopeProvider: req.ScopeProvider,
		ScopeModel:    req.ScopeModel,
		ScopeTenantID: req.ScopeTenantID,
		ScopeRole:     req.ScopeRole,
		ScopeUserID:   req.ScopeUserID,
		ScopeKeyID:    req.ScopeKeyID,
		Config:        configJSON,
		IsActive:      isActive,
		Priority:      priority,
		CreatedBy:     createdBy,
	}

	if err := h.pg.CreatePluginBinding(&b); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "create_plugin_binding", "plugin_binding", b.ID.String(), map[string]interface{}{
		"plugin_name": b.PluginName,
		"scope_level": b.ScopeLevel,
		"priority":    b.Priority,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(b)
}

// PUT /admin/plugin-bindings/{id} -> Update plugin binding config/status/priority
func (h *AdminHandler) UpdatePluginBinding(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginUpdate) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid binding ID"}`, http.StatusBadRequest)
		return
	}

	var req model.UpdatePluginBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.UpdatePluginBinding(id, req.Config, req.IsActive, req.Priority); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	updated, err := h.pg.GetPluginBindingByID(id)
	if err != nil {
		http.Error(w, `{"error": "Updated but failed to load binding"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "update_plugin_binding", "plugin_binding", id.String(), map[string]interface{}{
		"is_active": updated.IsActive,
		"priority":  updated.Priority,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DELETE /admin/plugin-bindings/{id} -> Delete a plugin binding
func (h *AdminHandler) DeletePluginBinding(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}
	if !h.checkIAMAction(w, r, permission.ActionPluginDelete) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid binding ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeletePluginBinding(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "delete_plugin_binding", "plugin_binding", id.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
