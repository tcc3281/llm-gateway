package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"llm-gateway/internal/model"
	"llm-gateway/internal/permission"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Groups Management Endpoints
// ---------------------------------------------------------------------------

// GET /admin/groups -> List all groups with member & policy counts
func (h *AdminHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	groups, err := h.pg.ListGroups()
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(groups))
}

// POST /admin/groups -> Create a new IAM Group
func (h *AdminHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	var req struct {
		Name               string  `json:"name"`
		Description        string  `json:"description"`
		BudgetLimitMonthly float64 `json:"budget_limit_monthly"`
		RateLimitRPM       int     `json:"rate_limit_rpm"`
		IsActive           *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, `{"error": "Group name is required"}`, http.StatusBadRequest)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	group := &model.Group{
		Name:               name,
		Description:        req.Description,
		BudgetLimitMonthly: req.BudgetLimitMonthly,
		RateLimitRPM:       req.RateLimitRPM,
		IsActive:           isActive,
	}

	if err := h.pg.CreateGroup(group); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "group.create", "group", group.ID.String(), group)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

// GET /admin/groups/{id} -> Get group detail by ID
func (h *AdminHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	group, err := h.pg.GetGroupByID(id)
	if err != nil {
		http.Error(w, `{"error": "Group not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// PATCH /admin/groups/{id} -> Update group fields
func (h *AdminHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	current, err := h.pg.GetGroupByID(id)
	if err != nil {
		http.Error(w, `{"error": "Group not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Name               *string  `json:"name"`
		Description        *string  `json:"description"`
		BudgetLimitMonthly *float64 `json:"budget_limit_monthly"`
		RateLimitRPM       *int     `json:"rate_limit_rpm"`
		IsActive           *bool    `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.BudgetLimitMonthly != nil {
		current.BudgetLimitMonthly = *req.BudgetLimitMonthly
	}
	if req.RateLimitRPM != nil {
		current.RateLimitRPM = *req.RateLimitRPM
	}
	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	if err := h.pg.UpdateGroup(current); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "group.update", "group", current.ID.String(), req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(current)
}

// DELETE /admin/groups/{id} -> Delete a group
func (h *AdminHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteGroup(id); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "group.delete", "group", id.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Group deleted successfully"})
}

// ---------------------------------------------------------------------------
// Group Policies Endpoints
// ---------------------------------------------------------------------------

// GET /admin/groups/{id}/policies -> List policies attached to a group
func (h *AdminHandler) ListGroupPolicies(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	policies, err := h.pg.GetGroupPolicies(id)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(policies))
}

// POST /admin/groups/{id}/policies -> Add an IAM policy to a group
func (h *AdminHandler) CreateGroupPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	groupID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Name     string          `json:"name"`
		Policy   json.RawMessage `json:"policy"`
		Priority int             `json:"priority"`
		IsActive *bool           `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, `{"error": "Policy name is required"}`, http.StatusBadRequest)
		return
	}

	// Validate JSON policy schema
	if _, err := permission.ParsePolicy(req.Policy); err != nil {
		http.Error(w, `{"error": "Invalid IAM policy document: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	gp := &model.GroupPolicy{
		GroupID:  groupID,
		Name:     strings.TrimSpace(req.Name),
		Policy:   req.Policy,
		Priority: req.Priority,
		IsActive: isActive,
	}

	if err := h.pg.CreateGroupPolicy(gp); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "policy.create", "policy", gp.ID.String(), gp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(gp)
}

// PATCH /admin/groups/{id}/policies/{pid} -> Update an IAM policy
func (h *AdminHandler) UpdateGroupPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	pidStr := r.PathValue("pid")
	if pidStr == "" {
		pidStr = r.URL.Query().Get("pid")
	}
	pid, err := uuid.Parse(pidStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid policy ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Name     string          `json:"name"`
		Policy   json.RawMessage `json:"policy"`
		Priority int             `json:"priority"`
		IsActive bool            `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if _, err := permission.ParsePolicy(req.Policy); err != nil {
		http.Error(w, `{"error": "Invalid IAM policy document: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	gp := &model.GroupPolicy{
		ID:       pid,
		Name:     req.Name,
		Policy:   req.Policy,
		Priority: req.Priority,
		IsActive: req.IsActive,
	}

	if err := h.pg.UpdateGroupPolicy(gp); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "policy.update", "policy", pid.String(), req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Policy updated successfully"})
}

// DELETE /admin/groups/{id}/policies/{pid} -> Delete an IAM policy
func (h *AdminHandler) DeleteGroupPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	pidStr := r.PathValue("pid")
	if pidStr == "" {
		pidStr = r.URL.Query().Get("pid")
	}
	pid, err := uuid.Parse(pidStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid policy ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteGroupPolicy(pid); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "policy.delete", "policy", pid.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Policy deleted successfully"})
}

// ---------------------------------------------------------------------------
// Group Members Endpoints
// ---------------------------------------------------------------------------

// GET /admin/groups/{id}/members -> List all user members of a group
func (h *AdminHandler) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	members, err := h.pg.GetGroupMembers(id)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(members))
}

// POST /admin/groups/{id}/members -> Add a user to a group
func (h *AdminHandler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	groupID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, `{"error": "Invalid user ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.AddUserToGroup(userID, groupID); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "group.add_member", "group", groupID.String(), map[string]string{
		"user_id":  userID.String(),
		"group_id": groupID.String(),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "User added to group successfully"})
}

// DELETE /admin/groups/{id}/members/{uid} -> Remove a user from a group
func (h *AdminHandler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	groupID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid group ID"}`, http.StatusBadRequest)
		return
	}

	uidStr := r.PathValue("uid")
	if uidStr == "" {
		uidStr = r.URL.Query().Get("uid")
	}
	userID, err := uuid.Parse(uidStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid user ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.RemoveUserFromGroup(userID, groupID); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if h.permEngine != nil {
		h.permEngine.InvalidateCache()
	}

	h.logAudit(r, "admin", "group.remove_member", "group", groupID.String(), map[string]string{
		"user_id":  userID.String(),
		"group_id": groupID.String(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "User removed from group successfully"})
}

// ---------------------------------------------------------------------------
// Key Model Restrictions Endpoints
// ---------------------------------------------------------------------------

// GET /admin/keys/{id}/models -> List model restriction policies for a key
func (h *AdminHandler) ListKeyModelPolicies(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	keyID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid key ID"}`, http.StatusBadRequest)
		return
	}

	policies, err := h.pg.GetKeyModelPolicies(keyID)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orEmpty(policies))
}

// POST /admin/keys/{id}/models -> Add a model restriction rule to a key
func (h *AdminHandler) CreateKeyModelPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	keyID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid key ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		ModelPattern string `json:"model_pattern"`
		IsAllowlist  *bool  `json:"is_allowlist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	pattern := strings.TrimSpace(req.ModelPattern)
	if pattern == "" {
		http.Error(w, `{"error": "model_pattern is required"}`, http.StatusBadRequest)
		return
	}

	isAllowlist := true
	if req.IsAllowlist != nil {
		isAllowlist = *req.IsAllowlist
	}

	kmp := &model.KeyModelPolicy{
		KeyID:        keyID,
		ModelPattern: pattern,
		IsAllowlist:  isAllowlist,
	}

	if err := h.pg.CreateKeyModelPolicy(kmp); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "key.add_model_policy", "key", keyID.String(), kmp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(kmp)
}

// DELETE /admin/keys/{id}/models/{pid} -> Delete a key model restriction rule
func (h *AdminHandler) DeleteKeyModelPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	pidStr := r.PathValue("pid")
	if pidStr == "" {
		pidStr = r.URL.Query().Get("pid")
	}
	pid, err := uuid.Parse(pidStr)
	if err != nil {
		http.Error(w, `{"error": "Invalid policy ID"}`, http.StatusBadRequest)
		return
	}

	if err := h.pg.DeleteKeyModelPolicy(pid); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	h.logAudit(r, "admin", "key.delete_model_policy", "key_model_policy", pid.String(), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Key model policy removed successfully"})
}

// ---------------------------------------------------------------------------
// Audit Logs Query Endpoint
// ---------------------------------------------------------------------------

// GET /admin/audit-logs -> Query audit trail with pagination and filters
func (h *AdminHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(w, r) {
		return
	}

	q := r.URL.Query()
	f := model.AuditLogFilter{
		ActorType:  q.Get("actor_type"),
		Action:     q.Get("action"),
		TargetType: q.Get("target_type"),
		Limit:      50,
		Offset:     0,
	}

	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 200 {
			f.Limit = l
		}
	}
	if v := q.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			f.Offset = o
		}
	}

	logs, total, err := h.pg.ListAuditLogs(f)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":   orEmpty(logs),
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}
