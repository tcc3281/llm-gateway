package db

import (
	"errors"
	"time"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ==========================================================================
// User Repositories
// ==========================================================================

func (p *PostgresDB) CreateUser(user *model.User) error {
	user.ID = uuid.New()
	if !user.IsActive {
		user.IsActive = true
	}
	if user.TenantRole == "" {
		user.TenantRole = "member"
	}
	query := `
		INSERT INTO users (id, email, name, role, is_root, is_active, password_hash, tenant_id, tenant_role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`
	return p.QueryRow(query, user.ID, user.Email, user.Name, user.Role, user.IsRoot, user.IsActive, user.PasswordHash, user.TenantID, user.TenantRole).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (p *PostgresDB) GetUserByID(id uuid.UUID) (*model.User, error) {
	u := &model.User{}
	query := `SELECT id, email, name, role, is_root, COALESCE(is_active, TRUE), COALESCE(password_hash, ''), created_at, updated_at, tenant_id, COALESCE(tenant_role, 'member') FROM users WHERE id = $1`
	err := p.QueryRow(query, id).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsRoot, &u.IsActive, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.TenantID, &u.TenantRole)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (p *PostgresDB) GetUserByEmail(email string) (*model.User, error) {
	u := &model.User{}
	query := `SELECT id, email, name, role, is_root, COALESCE(is_active, TRUE), COALESCE(password_hash, ''), created_at, updated_at, tenant_id, COALESCE(tenant_role, 'member') FROM users WHERE LOWER(email) = LOWER($1)`
	err := p.QueryRow(query, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsRoot, &u.IsActive, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.TenantID, &u.TenantRole)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (p *PostgresDB) SetUserPassword(id uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	res, err := p.Exec(query, passwordHash, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("user not found")
	}
	return nil
}

func (p *PostgresDB) UpdateUserStatus(id uuid.UUID, isActive bool) error {
	query := `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`
	_, err := p.Exec(query, isActive, id)
	return err
}

func (p *PostgresDB) ListUsers() ([]model.User, error) {
	query := `SELECT id, email, name, role, is_root, COALESCE(is_active, TRUE), COALESCE(password_hash, ''), created_at, updated_at, tenant_id, COALESCE(tenant_role, 'member') FROM users ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsRoot, &u.IsActive, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.TenantID, &u.TenantRole); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// UpdateUser changes a user's name, role, and root status. Values are resolved by the
// handler from the current record so unchanged fields are preserved.
func (p *PostgresDB) UpdateUser(id uuid.UUID, name, role string, isRoot bool) error {
	_, err := p.Exec(`UPDATE users SET name = $2, role = $3, is_root = $4, updated_at = NOW() WHERE id = $1`, id, name, role, isRoot)
	return err
}

func (p *PostgresDB) UpdateUserTenant(id uuid.UUID, tenantID *uuid.UUID, tenantRole string) error {
	_, err := p.Exec(`UPDATE users SET tenant_id = $2, tenant_role = $3, updated_at = NOW() WHERE id = $1`, id, tenantID, tenantRole)
	return err
}

func (p *PostgresDB) DeleteUser(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM users WHERE id = $1`, id)
	return err
}

func (p *PostgresDB) ListTenantUsers(tenantID uuid.UUID) ([]model.User, error) {
	query := `SELECT id, email, name, role, is_root, COALESCE(is_active, TRUE), COALESCE(password_hash, ''), created_at, updated_at, tenant_id, COALESCE(tenant_role, 'member') FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`
	rows, err := p.Query(query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.IsRoot, &u.IsActive, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt, &u.TenantID, &u.TenantRole); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ==========================================================================
// API Key Repositories
// ==========================================================================

func (p *PostgresDB) CreateAPIKey(key *model.APIKey) error {
	key.ID = uuid.New()
	query := `
		INSERT INTO api_keys (id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at`
	return p.QueryRow(query, key.ID, key.UserID, key.KeyPrefix, key.KeyHash, key.Name, key.BudgetLimit, key.RateLimitRPM, key.CacheTTLSeconds, key.IsActive, key.ExpiresAt, key.TenantID).Scan(&key.CreatedAt)
}

func (p *PostgresDB) GetAPIKeyByHash(keyHash string) (*model.APIKey, error) {
	k := &model.APIKey{}
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at, tenant_id
		FROM api_keys WHERE key_hash = $1 AND is_active = TRUE`
	err := p.QueryRow(query, keyHash).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt, &k.TenantID)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (p *PostgresDB) GetAPIKeyByID(id uuid.UUID) (*model.APIKey, error) {
	k := &model.APIKey{}
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at, tenant_id
		FROM api_keys WHERE id = $1`
	err := p.QueryRow(query, id).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt, &k.TenantID)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (p *PostgresDB) ListAPIKeys() ([]model.APIKey, error) {
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at, tenant_id
		FROM api_keys ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt, &k.TenantID); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (p *PostgresDB) DeactivateAPIKey(id uuid.UUID) error {
	_, err := p.Exec(`UPDATE api_keys SET is_active = FALSE WHERE id = $1`, id)
	return err
}

// UpdateAPIKey edits a key's name, budget, rate limit, cache TTL and optional expiry.
func (p *PostgresDB) UpdateAPIKey(id uuid.UUID, name string, budgetLimit float64, rateLimitRPM int, cacheTTLSeconds int, expiresAt *time.Time) error {
	_, err := p.Exec(`UPDATE api_keys SET name = $2, budget_limit = $3, rate_limit_rpm = $4, cache_ttl_seconds = $5, expires_at = $6 WHERE id = $1`,
		id, name, budgetLimit, rateLimitRPM, cacheTTLSeconds, expiresAt)
	return err
}

func (p *PostgresDB) ReactivateAPIKey(id uuid.UUID) error {
	_, err := p.Exec(`UPDATE api_keys SET is_active = TRUE WHERE id = $1`, id)
	return err
}

func (p *PostgresDB) ListAPIKeysByUser(userID uuid.UUID) ([]model.APIKey, error) {
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at, tenant_id
		FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := p.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt, &k.TenantID); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}
