package db

import (
	"fmt"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// IAM: Groups Management
// ---------------------------------------------------------------------------

func (p *PostgresDB) CreateGroup(g *model.Group) error {
	g.ID = uuid.New()
	query := `
		INSERT INTO groups (id, name, description, budget_limit_monthly, rate_limit_rpm, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`
	return p.QueryRow(query, g.ID, g.Name, g.Description, g.BudgetLimitMonthly, g.RateLimitRPM, g.IsActive).Scan(&g.CreatedAt, &g.UpdatedAt)
}

func (p *PostgresDB) GetGroupByID(id uuid.UUID) (*model.Group, error) {
	g := &model.Group{}
	query := `
		SELECT g.id, g.name, COALESCE(g.description, ''), g.budget_limit_monthly, g.rate_limit_rpm, g.is_active, g.created_at, g.updated_at,
		       COUNT(DISTINCT ug.user_id) AS member_count,
		       COUNT(DISTINCT gp.id) AS policy_count
		FROM groups g
		LEFT JOIN user_groups ug ON ug.group_id = g.id
		LEFT JOIN group_policies gp ON gp.group_id = g.id AND gp.is_active = TRUE
		WHERE g.id = $1
		GROUP BY g.id`
	err := p.QueryRow(query, id).Scan(&g.ID, &g.Name, &g.Description, &g.BudgetLimitMonthly, &g.RateLimitRPM, &g.IsActive, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount, &g.PolicyCount)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (p *PostgresDB) ListGroups() ([]model.Group, error) {
	query := `
		SELECT g.id, g.name, COALESCE(g.description, ''), g.budget_limit_monthly, g.rate_limit_rpm, g.is_active, g.created_at, g.updated_at,
		       COUNT(DISTINCT ug.user_id) AS member_count,
		       COUNT(DISTINCT gp.id) AS policy_count
		FROM groups g
		LEFT JOIN user_groups ug ON ug.group_id = g.id
		LEFT JOIN group_policies gp ON gp.group_id = g.id AND gp.is_active = TRUE
		GROUP BY g.id
		ORDER BY g.created_at ASC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.BudgetLimitMonthly, &g.RateLimitRPM, &g.IsActive, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount, &g.PolicyCount); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (p *PostgresDB) UpdateGroup(g *model.Group) error {
	query := `
		UPDATE groups
		SET name = $2, description = $3, budget_limit_monthly = $4, rate_limit_rpm = $5, is_active = $6, updated_at = NOW()
		WHERE id = $1`
	_, err := p.Exec(query, g.ID, g.Name, g.Description, g.BudgetLimitMonthly, g.RateLimitRPM, g.IsActive)
	return err
}

func (p *PostgresDB) DeleteGroup(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM groups WHERE id = $1`, id)
	return err
}

// ---------------------------------------------------------------------------
// IAM: Group Policies Management
// ---------------------------------------------------------------------------

func (p *PostgresDB) CreateGroupPolicy(gp *model.GroupPolicy) error {
	gp.ID = uuid.New()
	query := `
		INSERT INTO group_policies (id, group_id, name, policy, priority, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`
	return p.QueryRow(query, gp.ID, gp.GroupID, gp.Name, gp.Policy, gp.Priority, gp.IsActive).Scan(&gp.CreatedAt)
}

func (p *PostgresDB) GetGroupPolicies(groupID uuid.UUID) ([]model.GroupPolicy, error) {
	query := `
		SELECT id, group_id, name, policy, priority, is_active, created_at
		FROM group_policies
		WHERE group_id = $1
		ORDER BY priority DESC, created_at ASC`
	rows, err := p.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []model.GroupPolicy
	for rows.Next() {
		var gp model.GroupPolicy
		if err := rows.Scan(&gp.ID, &gp.GroupID, &gp.Name, &gp.Policy, &gp.Priority, &gp.IsActive, &gp.CreatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, gp)
	}
	return policies, nil
}

func (p *PostgresDB) UpdateGroupPolicy(gp *model.GroupPolicy) error {
	query := `
		UPDATE group_policies
		SET name = $2, policy = $3, priority = $4, is_active = $5
		WHERE id = $1`
	_, err := p.Exec(query, gp.ID, gp.Name, gp.Policy, gp.Priority, gp.IsActive)
	return err
}

func (p *PostgresDB) DeleteGroupPolicy(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM group_policies WHERE id = $1`, id)
	return err
}

// ---------------------------------------------------------------------------
// IAM: User-Group Mapping
// ---------------------------------------------------------------------------

func (p *PostgresDB) AddUserToGroup(userID, groupID uuid.UUID) error {
	return p.AddUserToGroupWithRole(userID, groupID, "member")
}

func (p *PostgresDB) AddUserToGroupWithRole(userID, groupID uuid.UUID, role string) error {
	if role == "" {
		role = "member"
	}
	query := `
		INSERT INTO user_groups (user_id, group_id, group_role)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, group_id) DO UPDATE SET group_role = EXCLUDED.group_role`
	_, err := p.Exec(query, userID, groupID, role)
	return err
}

func (p *PostgresDB) SetGroupMemberRole(userID, groupID uuid.UUID, role string) error {
	if role != "admin" && role != "member" {
		return fmt.Errorf("invalid group role: %s (must be 'admin' or 'member')", role)
	}
	query := `UPDATE user_groups SET group_role = $1 WHERE user_id = $2 AND group_id = $3`
	res, err := p.Exec(query, role, userID, groupID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user is not a member of this group")
	}
	return nil
}

func (p *PostgresDB) GetGroupMemberRole(userID, groupID uuid.UUID) (string, error) {
	var role string
	err := p.QueryRow(`SELECT COALESCE(group_role, 'member') FROM user_groups WHERE user_id = $1 AND group_id = $2`, userID, groupID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

func (p *PostgresDB) IsUserGroupAdmin(userID, groupID uuid.UUID) (bool, error) {
	role, err := p.GetGroupMemberRole(userID, groupID)
	if err != nil {
		return false, err
	}
	return role == "admin", nil
}

func (p *PostgresDB) GetUserAdminGroups(userID uuid.UUID) ([]model.Group, error) {
	query := `
		SELECT g.id, g.name, COALESCE(g.description, ''), g.budget_limit_monthly, g.rate_limit_rpm, g.is_active, g.created_at, g.updated_at
		FROM groups g
		JOIN user_groups ug ON ug.group_id = g.id
		WHERE ug.user_id = $1 AND ug.group_role = 'admin' AND g.is_active = TRUE
		ORDER BY g.name ASC`
	rows, err := p.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.BudgetLimitMonthly, &g.RateLimitRPM, &g.IsActive, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (p *PostgresDB) RemoveUserFromGroup(userID, groupID uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM user_groups WHERE user_id = $1 AND group_id = $2`, userID, groupID)
	return err
}

func (p *PostgresDB) GetUserGroups(userID uuid.UUID) ([]model.Group, error) {
	query := `
		SELECT g.id, g.name, COALESCE(g.description, ''), g.budget_limit_monthly, g.rate_limit_rpm, g.is_active, g.created_at, g.updated_at
		FROM groups g
		JOIN user_groups ug ON ug.group_id = g.id
		WHERE ug.user_id = $1 AND g.is_active = TRUE
		ORDER BY g.name ASC`
	rows, err := p.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.BudgetLimitMonthly, &g.RateLimitRPM, &g.IsActive, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (p *PostgresDB) GetGroupMembers(groupID uuid.UUID) ([]model.User, error) {
	query := `
		SELECT u.id, u.email, u.name, u.role, u.is_root, COALESCE(u.is_active, TRUE), COALESCE(u.password_hash, ''), COALESCE(ug.group_role, 'member'), u.created_at, u.updated_at
		FROM users u
		JOIN user_groups ug ON ug.user_id = u.id
		WHERE ug.group_id = $1
		ORDER BY (ug.group_role = 'admin') DESC, u.created_at DESC`
	rows, err := p.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsRoot, &u.IsActive, &u.PasswordHash, &u.GroupRole, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ---------------------------------------------------------------------------
// IAM: Key Model Restrictions (Allowlist / Denylist)
// ---------------------------------------------------------------------------

func (p *PostgresDB) CreateKeyModelPolicy(kmp *model.KeyModelPolicy) error {
	kmp.ID = uuid.New()
	query := `
		INSERT INTO key_model_policies (id, key_id, model_pattern, is_allowlist)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`
	return p.QueryRow(query, kmp.ID, kmp.KeyID, kmp.ModelPattern, kmp.IsAllowlist).Scan(&kmp.CreatedAt)
}

func (p *PostgresDB) GetKeyModelPolicies(keyID uuid.UUID) ([]model.KeyModelPolicy, error) {
	query := `
		SELECT id, key_id, model_pattern, is_allowlist, created_at
		FROM key_model_policies
		WHERE key_id = $1
		ORDER BY created_at ASC`
	rows, err := p.Query(query, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []model.KeyModelPolicy
	for rows.Next() {
		var kmp model.KeyModelPolicy
		if err := rows.Scan(&kmp.ID, &kmp.KeyID, &kmp.ModelPattern, &kmp.IsAllowlist, &kmp.CreatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, kmp)
	}
	return policies, nil
}

func (p *PostgresDB) DeleteKeyModelPolicy(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM key_model_policies WHERE id = $1`, id)
	return err
}
