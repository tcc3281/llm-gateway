package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"llm-gateway/internal/config"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	*sql.DB
}

func NewPostgres(cfg *config.Config) (*PostgresDB, error) {
	dsn := cfg.GetDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	pg := &PostgresDB{DB: db}
	if err := pg.runMigrations(); err != nil {
		log.Printf("Warning running migrations: %v", err)
	}

	return pg, nil
}

func (p *PostgresDB) runMigrations() error {
	dir := "migrations"
	if _, err := os.Stat(dir); err != nil {
		dir = "../../migrations" // Fallback when running from cmd/
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Warning: cannot read migrations dir %q: %v", dir, err)
		return nil
	}

	// Apply in filename order so 000001 runs before 000002, etc.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	applied := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("failed reading migration %s: %w", e.Name(), err)
		}
		if _, err := p.Exec(string(content)); err != nil {
			return fmt.Errorf("failed executing migration %s: %w", e.Name(), err)
		}
		applied++
		log.Printf("Applied migration %s", e.Name())
	}

	log.Printf("PostgreSQL schema migration completed (%d file(s)).", applied)
	return nil
}

// User Repositories
func (p *PostgresDB) CreateUser(user *model.User) error {
	user.ID = uuid.New()
	query := `INSERT INTO users (id, email, name, role) VALUES ($1, $2, $3, $4) RETURNING created_at, updated_at`
	return p.QueryRow(query, user.ID, user.Email, user.Name, user.Role).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (p *PostgresDB) GetUserByID(id uuid.UUID) (*model.User, error) {
	u := &model.User{}
	query := `SELECT id, email, name, role, created_at, updated_at FROM users WHERE id = $1`
	err := p.QueryRow(query, id).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// API Key Repositories
func (p *PostgresDB) CreateAPIKey(key *model.APIKey) error {
	key.ID = uuid.New()
	query := `
		INSERT INTO api_keys (id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, is_active, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`
	return p.QueryRow(query, key.ID, key.UserID, key.KeyPrefix, key.KeyHash, key.Name, key.BudgetLimit, key.RateLimitRPM, key.IsActive, key.ExpiresAt).Scan(&key.CreatedAt)
}

func (p *PostgresDB) GetAPIKeyByHash(keyHash string) (*model.APIKey, error) {
	k := &model.APIKey{}
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, is_active, expires_at, created_at
		FROM api_keys WHERE key_hash = $1 AND is_active = TRUE`
	err := p.QueryRow(query, keyHash).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.IsActive, &k.ExpiresAt, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// Model Pricing Repositories
func (p *PostgresDB) GetPricings() (map[string]model.ModelPricing, error) {
	query := `SELECT id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, updated_at FROM model_pricings`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pricings := make(map[string]model.ModelPricing)
	for rows.Next() {
		var pr model.ModelPricing
		if err := rows.Scan(&pr.ID, &pr.Provider, &pr.ModelName, &pr.PromptPricePer1M, &pr.CompletionPricePer1M, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		// Key format: provider:model_name (e.g., "openai:gpt-4o")
		key := fmt.Sprintf("%s:%s", pr.Provider, pr.ModelName)
		pricings[key] = pr
	}
	return pricings, nil
}

func (p *PostgresDB) GetAllModelsList() ([]model.ModelPricing, error) {
	query := `SELECT id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, updated_at FROM model_pricings ORDER BY provider, model_name`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ModelPricing
	for rows.Next() {
		var pr model.ModelPricing
		if err := rows.Scan(&pr.ID, &pr.Provider, &pr.ModelName, &pr.PromptPricePer1M, &pr.CompletionPricePer1M, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, pr)
	}
	return list, nil
}

func (p *PostgresDB) UpsertModelPricing(pr *model.ModelPricing) error {
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	query := `
		INSERT INTO model_pricings (id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (provider, model_name) DO UPDATE SET
			prompt_price_per_1m = EXCLUDED.prompt_price_per_1m,
			completion_price_per_1m = EXCLUDED.completion_price_per_1m,
			updated_at = NOW()
		RETURNING updated_at`
	return p.QueryRow(query, pr.ID, pr.Provider, pr.ModelName, pr.PromptPricePer1M, pr.CompletionPricePer1M).Scan(&pr.UpdatedAt)
}

func (p *PostgresDB) DeleteModelPricing(id uuid.UUID) error {
	query := `DELETE FROM model_pricings WHERE id = $1`
	_, err := p.Exec(query, id)
	return err
}

func (p *PostgresDB) GetSpentCostByKeyID(keyID uuid.UUID) (float64, error) {
	var totalCost float64
	query := `SELECT COALESCE(SUM(cost), 0) FROM usage_logs WHERE key_id = $1`
	err := p.QueryRow(query, keyID).Scan(&totalCost)
	return totalCost, err
}

// User Management
func (p *PostgresDB) ListUsers() ([]model.User, error) {
	query := `SELECT id, email, name, role, created_at, updated_at FROM users ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// API Key Management
func (p *PostgresDB) GetAPIKeyByID(id uuid.UUID) (*model.APIKey, error) {
	k := &model.APIKey{}
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, is_active, expires_at, created_at
		FROM api_keys WHERE id = $1`
	err := p.QueryRow(query, id).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.IsActive, &k.ExpiresAt, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (p *PostgresDB) ListAPIKeys() ([]model.APIKey, error) {
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, is_active, expires_at, created_at
		FROM api_keys ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.IsActive, &k.ExpiresAt, &k.CreatedAt); err != nil {
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

// Provider Key Repositories (stateful provider credentials)
func (p *PostgresDB) GetProviderKey(provider string) (*model.ProviderKey, error) {
	k := &model.ProviderKey{}
	query := `
		SELECT id, provider, api_key, is_active, created_at, updated_at
		FROM provider_keys WHERE provider = $1 AND is_active = TRUE`
	err := p.QueryRow(query, provider).Scan(&k.ID, &k.Provider, &k.APIKey, &k.IsActive, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (p *PostgresDB) UpsertProviderKey(pk *model.ProviderKey) error {
	if pk.ID == uuid.Nil {
		pk.ID = uuid.New()
	}
	query := `
		INSERT INTO provider_keys (id, provider, api_key, is_active, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (provider) DO UPDATE SET
			api_key = EXCLUDED.api_key,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return p.QueryRow(query, pk.ID, pk.Provider, pk.APIKey, pk.IsActive).Scan(&pk.CreatedAt, &pk.UpdatedAt)
}

func (p *PostgresDB) ListProviderKeys() ([]model.ProviderKey, error) {
	query := `SELECT id, provider, api_key, is_active, created_at, updated_at FROM provider_keys ORDER BY provider`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.ProviderKey
	for rows.Next() {
		var k model.ProviderKey
		if err := rows.Scan(&k.ID, &k.Provider, &k.APIKey, &k.IsActive, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (p *PostgresDB) DeleteProviderKey(provider string) error {
	_, err := p.Exec(`DELETE FROM provider_keys WHERE provider = $1`, provider)
	return err
}



// Usage Telemetry Repositories (Batch Insert)
func (p *PostgresDB) InsertUsageLogsBatch(logs []model.UsageLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO usage_logs 
		(created_at, user_id, key_id, inbound_format, provider, model, prompt_tokens, completion_tokens, total_tokens, cost, ttft_ms, total_latency_ms, status_code, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range logs {
		if l.CreatedAt.IsZero() {
			l.CreatedAt = time.Now()
		}
		_, err := stmt.Exec(
			l.CreatedAt, l.UserID, l.KeyID, l.InboundFormat, l.Provider, l.Model,
			l.PromptTokens, l.CompletionTokens, l.TotalTokens, l.Cost,
			l.TTFTMs, l.TotalLatencyMs, l.StatusCode, l.ErrorMessage,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Analytics Timeseries Query
func (p *PostgresDB) GetUsageStats(userID *uuid.UUID, interval string, from, to time.Time) ([]model.UsageStats, error) {
	truncUnit := "hour"
	if interval == "day" {
		truncUnit = "day"
	}

	query := fmt.Sprintf(`
		SELECT 
			to_char(date_trunc('%s', created_at), 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS time_bucket,
			provider,
			model,
			COUNT(*) AS total_requests,
			COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(ROUND(AVG(ttft_ms), 2), 0) AS avg_ttft_ms,
			COALESCE(ROUND(AVG(total_latency_ms), 2), 0) AS avg_latency_ms
		FROM usage_logs
		WHERE created_at >= $1 AND created_at <= $2
		`, truncUnit)

	args := []interface{}{from, to}
	if userID != nil {
		query += " AND user_id = $3"
		args = append(args, *userID)
	}

	query += fmt.Sprintf(" GROUP BY time_bucket, provider, model ORDER BY time_bucket DESC")

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []model.UsageStats
	for rows.Next() {
		var s model.UsageStats
		if err := rows.Scan(&s.TimeBucket, &s.Provider, &s.Model, &s.TotalRequests, &s.SumPromptTokens, &s.SumCompletionTokens, &s.TotalCost, &s.AvgTTFTMs, &s.AvgLatencyMs); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, nil
}
