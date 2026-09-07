package permission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"llm-gateway/internal/db"

	"github.com/google/uuid"
)

// PermissionEngine evaluates IAM policies for users and API keys.
type PermissionEngine struct {
	pg *db.PostgresDB

	mu          sync.RWMutex
	policyCache map[uuid.UUID][]cachedPolicy // group_id -> parsed policies
	cacheExpiry time.Time
	cacheTTL    time.Duration
}

type cachedPolicy struct {
	Name     string
	Priority int
	Doc      *PolicyDocument
}

// NewPermissionEngine initializes the IAM permission engine.
func NewPermissionEngine(pg *db.PostgresDB) *PermissionEngine {
	return &PermissionEngine{
		pg:          pg,
		policyCache: make(map[uuid.UUID][]cachedPolicy),
		cacheTTL:    60 * time.Second,
	}
}

// Check evaluates whether a user has permission to perform action on resource.
// Returns (allowed bool, reason string, err error).
// Evaluation logic:
//   1. Root admin (is_root = true) bypasses all checks.
//   2. Explicit Deny in any assigned group policy wins over Allow.
//   3. Explicit Allow in any assigned group policy grants access.
//   4. Implicit Deny if no policy matches.
func (pe *PermissionEngine) Check(ctx context.Context, userID uuid.UUID, action, resource string) (bool, string, error) {
	if pe.pg == nil {
		return false, "database_unavailable", fmt.Errorf("database unavailable for permission evaluation")
	}

	// 1. Root user bypass
	user, err := pe.pg.GetUserByID(userID)
	if err != nil {
		return false, "user_not_found", fmt.Errorf("failed to load user %s: %w", userID, err)
	}
	if user.IsRoot {
		return true, "root:bypass", nil
	}

	// 2. Fetch user's active groups
	groups, err := pe.pg.GetUserGroups(userID)
	if err != nil {
		return false, "error_fetching_groups", err
	}
	if len(groups) == 0 {
		// If user has role="admin", treat as full access for backward compatibility
		if user.Role == "admin" {
			return true, "role:admin_fallback", nil
		}
		return false, "implicit_deny:no_groups", nil
	}

	// 3. Collect policies from all active groups
	hasAllow := false
	var matchedAllowPolicy string

	for _, g := range groups {
		if !g.IsActive {
			continue
		}

		policies, pErr := pe.getGroupPolicies(ctx, g.ID)
		if pErr != nil {
			return false, "error_fetching_policies", pErr
		}

		for _, cp := range policies {
			matched, isDeny := cp.Doc.Evaluate(action, resource)
			if matched {
				if isDeny {
					// Explicit Deny ALWAYS wins immediately
					return false, fmt.Sprintf("explicit_deny:policy_%s", cp.Name), nil
				}
				hasAllow = true
				matchedAllowPolicy = cp.Name
			}
		}
	}

	if hasAllow {
		return true, fmt.Sprintf("allow:policy_%s", matchedAllowPolicy), nil
	}

	// Implicit deny
	return false, "implicit_deny:no_matching_allow", nil
}

// CheckModelAccess is a convenience helper for gateway model authorization.
func (pe *PermissionEngine) CheckModelAccess(ctx context.Context, userID uuid.UUID, modelName string) (bool, error) {
	allowed, _, err := pe.Check(ctx, userID, "model:use", "model:"+modelName)
	return allowed, err
}

// CheckKeyModelAccess evaluates per-key model allowlist / denylist rules.
// If no rules exist for key, it returns true (fallback to user IAM policies).
func (pe *PermissionEngine) CheckKeyModelAccess(ctx context.Context, keyID uuid.UUID, modelName string) (bool, error) {
	if pe.pg == nil {
		return true, nil
	}

	policies, err := pe.pg.GetKeyModelPolicies(keyID)
	if err != nil {
		return false, fmt.Errorf("failed to load key model policies: %w", err)
	}

	if len(policies) == 0 {
		return true, nil // No per-key restriction configured
	}

	hasAllowlist := false
	matchedAllowlist := false

	for _, p := range policies {
		if p.IsAllowlist {
			hasAllowlist = true
			if MatchPattern(p.ModelPattern, modelName) {
				matchedAllowlist = true
			}
		} else {
			// Explicit key denylist
			if MatchPattern(p.ModelPattern, modelName) {
				return false, nil // Blocked by key denylist
			}
		}
	}

	// If allowlist rules exist, the model MUST match at least one
	if hasAllowlist && !matchedAllowlist {
		return false, nil
	}

	return true, nil
}

// InvalidateCache invalidates the in-memory policy cache.
func (pe *PermissionEngine) InvalidateCache() {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.policyCache = make(map[uuid.UUID][]cachedPolicy)
	pe.cacheExpiry = time.Time{}
}

// CheckGroupAdmin evaluates whether a user is an admin of the specified group or root.
func (pe *PermissionEngine) CheckGroupAdmin(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	if pe.pg == nil {
		return false, fmt.Errorf("database unavailable for permission evaluation")
	}
	user, err := pe.pg.GetUserByID(userID)
	if err == nil && user.IsRoot {
		return true, nil
	}
	return pe.pg.IsUserGroupAdmin(userID, groupID)
}

// CheckGroupMember evaluates whether a user belongs to the specified group.
func (pe *PermissionEngine) CheckGroupMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	if pe.pg == nil {
		return false, fmt.Errorf("database unavailable for permission evaluation")
	}
	_, err := pe.pg.GetGroupMemberRole(userID, groupID)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (pe *PermissionEngine) getGroupPolicies(ctx context.Context, groupID uuid.UUID) ([]cachedPolicy, error) {
	pe.mu.RLock()
	if time.Now().Before(pe.cacheExpiry) {
		if cached, ok := pe.policyCache[groupID]; ok {
			pe.mu.RUnlock()
			return cached, nil
		}
	}
	pe.mu.RUnlock()

	// Cache miss or expired: fetch from Postgres
	rawPolicies, err := pe.pg.GetGroupPolicies(groupID)
	if err != nil {
		return nil, err
	}

	parsed := make([]cachedPolicy, 0, len(rawPolicies))
	for _, rp := range rawPolicies {
		if !rp.IsActive {
			continue
		}
		doc, err := ParsePolicy(rp.Policy)
		if err != nil {
			continue
		}
		parsed = append(parsed, cachedPolicy{
			Name:     rp.Name,
			Priority: rp.Priority,
			Doc:      doc,
		})
	}

	pe.mu.Lock()
	pe.policyCache[groupID] = parsed
	if time.Now().After(pe.cacheExpiry) {
		pe.cacheExpiry = time.Now().Add(pe.cacheTTL)
	}
	pe.mu.Unlock()

	return parsed, nil
}
