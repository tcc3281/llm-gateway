package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
		INSERT INTO api_keys (id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`
	return p.QueryRow(query, key.ID, key.UserID, key.KeyPrefix, key.KeyHash, key.Name, key.BudgetLimit, key.RateLimitRPM, key.CacheTTLSeconds, key.IsActive, key.ExpiresAt).Scan(&key.CreatedAt)
}

func (p *PostgresDB) GetAPIKeyByHash(keyHash string) (*model.APIKey, error) {
	k := &model.APIKey{}
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at
		FROM api_keys WHERE key_hash = $1 AND is_active = TRUE`
	err := p.QueryRow(query, keyHash).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt)
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

	list := make([]model.ModelPricing, 0)
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

// ReplaceProviderModels atomically replaces the discovered catalog for one
// provider: it inserts the fresh model list and deletes rows that disappeared
// upstream, so the catalog never routes to stale models. An empty list clears
// the provider's catalog entirely.
func (p *PostgresDB) ReplaceProviderModels(provider string, modelNames []string) error {
	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	if _, err := tx.Exec(`DELETE FROM provider_models WHERE provider = $1`, provider); err != nil {
		return err
	}
	for _, name := range modelNames {
		if _, err := tx.Exec(
			`INSERT INTO provider_models (provider, model_name) VALUES ($1, $2)
			 ON CONFLICT (provider, model_name) DO NOTHING`,
			provider, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListCatalogModels returns every (provider, model) row of the discovered
// catalog, used to build the routing cache.
func (p *PostgresDB) ListCatalogModels() ([]model.CatalogModel, error) {
	query := `SELECT provider, model_name, created_at, updated_at FROM provider_models ORDER BY provider, model_name`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.CatalogModel, 0)
	for rows.Next() {
		var cm model.CatalogModel
		if err := rows.Scan(&cm.Provider, &cm.ModelName, &cm.CreatedAt, &cm.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, cm)
	}
	return list, rows.Err()
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
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at
		FROM api_keys WHERE id = $1`
	err := p.QueryRow(query, id).Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (p *PostgresDB) ListAPIKeys() ([]model.APIKey, error) {
	query := `
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at
		FROM api_keys ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt); err != nil {
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

// UpdateUser changes a user's name and/or role. Values are resolved by the
// handler from the current record so unchanged fields are preserved.
func (p *PostgresDB) UpdateUser(id uuid.UUID, name, role string) error {
	_, err := p.Exec(`UPDATE users SET name = $2, role = $3, updated_at = NOW() WHERE id = $1`, id, name, role)
	return err
}

func (p *PostgresDB) DeleteUser(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM users WHERE id = $1`, id)
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
		SELECT id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, cache_ttl_seconds, is_active, expires_at, created_at
		FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := p.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyPrefix, &k.KeyHash, &k.Name, &k.BudgetLimit, &k.RateLimitRPM, &k.CacheTTLSeconds, &k.IsActive, &k.ExpiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// Provider Key Repositories (stateful provider credentials)
func (p *PostgresDB) GetProviderKey(provider string) (*model.ProviderKey, error) {
	k := &model.ProviderKey{}
	query := `
		SELECT id, provider, api_key, base_url, protocol, fallback_priority, is_active, created_at, updated_at
		FROM provider_keys WHERE provider = $1 AND is_active = TRUE`
	err := p.QueryRow(query, provider).Scan(&k.ID, &k.Provider, &k.APIKey, &k.BaseURL, &k.Protocol, &k.FallbackPriority, &k.IsActive, &k.CreatedAt, &k.UpdatedAt)
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
		INSERT INTO provider_keys (id, provider, api_key, base_url, protocol, fallback_priority, is_active, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (provider) DO UPDATE SET
			api_key = EXCLUDED.api_key,
			base_url = EXCLUDED.base_url,
			protocol = EXCLUDED.protocol,
			fallback_priority = EXCLUDED.fallback_priority,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return p.QueryRow(query, pk.ID, pk.Provider, pk.APIKey, pk.BaseURL, pk.Protocol, pk.FallbackPriority, pk.IsActive).Scan(&pk.CreatedAt, &pk.UpdatedAt)
}

func (p *PostgresDB) ListProviderKeys() ([]model.ProviderKey, error) {
	query := `SELECT id, provider, api_key, base_url, protocol, fallback_priority, is_active, created_at, updated_at FROM provider_keys ORDER BY provider`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.ProviderKey
	for rows.Next() {
		var k model.ProviderKey
		if err := rows.Scan(&k.ID, &k.Provider, &k.APIKey, &k.BaseURL, &k.Protocol, &k.FallbackPriority, &k.IsActive, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// GetFallbackProviders returns active providers configured with a fallback_priority,
// ordered by fallback_priority ASC (lower = tried first), excluding the primary provider.
func (p *PostgresDB) GetFallbackProviders(primaryProvider string) ([]model.ProviderKey, error) {
	query := `
		SELECT id, provider, api_key, base_url, protocol, fallback_priority, is_active, created_at, updated_at
		FROM provider_keys
		WHERE is_active = TRUE AND provider != $1 AND fallback_priority IS NOT NULL
		ORDER BY fallback_priority ASC
		LIMIT 5`
	rows, err := p.Query(query, primaryProvider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.ProviderKey
	for rows.Next() {
		var k model.ProviderKey
		if err := rows.Scan(&k.ID, &k.Provider, &k.APIKey, &k.BaseURL, &k.Protocol, &k.FallbackPriority, &k.IsActive, &k.CreatedAt, &k.UpdatedAt); err != nil {
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
		(created_at, user_id, key_id, inbound_format, provider, model, prompt_tokens, completion_tokens, total_tokens, 
		 cached_input_tokens, reasoning_tokens, prompt_price_used, completion_price_used, pricing_source, pricing_fallback,
		 cost, ttft_ms, total_latency_ms, status_code, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`)
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
			l.PromptTokens, l.CompletionTokens, l.TotalTokens,
			l.CachedInputTokens, l.ReasoningTokens, l.PromptPriceUsed, l.CompletionPriceUsed, l.PricingSource, l.PricingFallback,
			l.Cost, l.TTFTMs, l.TotalLatencyMs, l.StatusCode, l.ErrorMessage,
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

// ------------------------------------------------------------
// Dashboard analytics queries (backing GET /admin/analytics/dashboard)
// ------------------------------------------------------------

func buildDashboardWhere(f model.DashboardFilter, from, to time.Time, tablePrefix string) (string, []interface{}) {
	var prefix string
	if tablePrefix != "" {
		prefix = tablePrefix + "."
	}
	clauses := []string{
		fmt.Sprintf("%screated_at >= $1", prefix),
		fmt.Sprintf("%screated_at <= $2", prefix),
	}
	args := []interface{}{from, to}

	if f.UserID != nil {
		args = append(args, *f.UserID)
		clauses = append(clauses, fmt.Sprintf("%suser_id = $%d", prefix, len(args)))
	}
	if f.KeyID != nil {
		args = append(args, *f.KeyID)
		clauses = append(clauses, fmt.Sprintf("%skey_id = $%d", prefix, len(args)))
	}
	if f.Provider != "" {
		args = append(args, f.Provider)
		clauses = append(clauses, fmt.Sprintf("%sprovider = $%d", prefix, len(args)))
	}
	if f.Model != "" {
		args = append(args, f.Model)
		clauses = append(clauses, fmt.Sprintf("%smodel = $%d", prefix, len(args)))
	}
	return strings.Join(clauses, " AND "), args
}

func (p *PostgresDB) GetUsageTotals(f model.DashboardFilter, from, to time.Time) (model.UsageTotal, error) {
	where, args := buildDashboardWhere(f, from, to, "")
	query := `
		SELECT
			COUNT(*) AS total_requests,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens,
			COALESCE(SUM(cached_input_tokens), 0) AS sum_cached_tokens,
			COALESCE(SUM(reasoning_tokens), 0) AS sum_reasoning_tokens,
			COALESCE(SUM((cached_input_tokens::numeric / 1000000.0) * (prompt_price_used * 0.5)), 0) AS cache_savings_usd,
			COALESCE(COUNT(*) FILTER (WHERE ttft_ms > 0), 0) AS streaming_requests,
			COALESCE(ROUND(AVG(ttft_ms) FILTER (WHERE ttft_ms > 0), 2), 0) AS avg_ttft_ms,
			COALESCE(ROUND(AVG(total_latency_ms), 2), 0) AS avg_latency_ms,
			COALESCE(ROUND(CAST(percentile_cont(0.50) WITHIN GROUP (ORDER BY total_latency_ms) AS numeric), 2), 0) AS p50_latency_ms,
			COALESCE(ROUND(CAST(percentile_cont(0.95) WITHIN GROUP (ORDER BY total_latency_ms) AS numeric), 2), 0) AS p95_latency_ms,
			COALESCE(ROUND(CAST(percentile_cont(0.99) WITHIN GROUP (ORDER BY total_latency_ms) AS numeric), 2), 0) AS p99_latency_ms,
			COALESCE(COUNT(*) FILTER (WHERE status_code >= 400), 0) AS error_count
		FROM usage_logs
		WHERE ` + where

	var t model.UsageTotal
	if err := p.QueryRow(query, args...).Scan(
		&t.TotalRequests, &t.TotalCost, &t.SumPromptTokens, &t.SumCompletionTokens,
		&t.SumCachedTokens, &t.SumReasoningTokens, &t.CacheSavingsUSD, &t.StreamingRequests,
		&t.AvgTTFTMs, &t.AvgLatencyMs, &t.P50LatencyMs, &t.P95LatencyMs, &t.P99LatencyMs, &t.ErrorCount,
	); err != nil {
		return t, err
	}
	if t.SumPromptTokens > 0 {
		t.CacheHitRate = (float64(t.SumCachedTokens) / float64(t.SumPromptTokens)) * 100.0
	}
	if t.TotalRequests > 0 {
		t.ErrorRate = float64(t.ErrorCount) / float64(t.TotalRequests)
	}
	return t, nil
}

func (p *PostgresDB) GetUsageTimeSeries(f model.DashboardFilter, interval string, from, to time.Time) ([]model.UsageSeriesPoint, error) {
	truncUnit := "hour"
	if interval == "day" {
		truncUnit = "day"
	}

	where, args := buildDashboardWhere(f, from, to, "")

	query := fmt.Sprintf(`
		WITH time_series AS (
			SELECT generate_series(
				date_trunc('%[1]s', $1::timestamp with time zone),
				date_trunc('%[1]s', $2::timestamp with time zone),
				'1 %[1]s'::interval
			) AS time_bucket
		),
		logged_data AS (
			SELECT
				date_trunc('%[1]s', created_at) AS bucket,
				COUNT(*) AS total_requests,
				COALESCE(SUM(cost), 0) AS total_cost,
				COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
				COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens,
				COALESCE(SUM(cached_input_tokens), 0) AS sum_cached_tokens,
				COALESCE(SUM(reasoning_tokens), 0) AS sum_reasoning_tokens,
				COALESCE(COUNT(*) FILTER (WHERE status_code >= 400), 0) AS error_count,
				COALESCE(ROUND(AVG(ttft_ms) FILTER (WHERE ttft_ms > 0), 2), 0) AS avg_ttft_ms,
				COALESCE(ROUND(AVG(total_latency_ms), 2), 0) AS avg_latency_ms
			FROM usage_logs
			WHERE %[2]s
			GROUP BY bucket
		)
		SELECT
			to_char(time_series.time_bucket, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS time_bucket,
			COALESCE(logged_data.total_requests, 0) AS total_requests,
			COALESCE(logged_data.total_cost, 0) AS total_cost,
			COALESCE(logged_data.sum_prompt_tokens, 0) AS sum_prompt_tokens,
			COALESCE(logged_data.sum_completion_tokens, 0) AS sum_completion_tokens,
			COALESCE(logged_data.sum_cached_tokens, 0) AS sum_cached_tokens,
			COALESCE(logged_data.sum_reasoning_tokens, 0) AS sum_reasoning_tokens,
			COALESCE(logged_data.error_count, 0) AS error_count,
			COALESCE(logged_data.avg_ttft_ms, 0) AS avg_ttft_ms,
			COALESCE(logged_data.avg_latency_ms, 0) AS avg_latency_ms
		FROM time_series
		LEFT JOIN logged_data ON time_series.time_bucket = logged_data.bucket
		ORDER BY time_series.time_bucket ASC`, truncUnit, where)

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []model.UsageSeriesPoint
	for rows.Next() {
		var s model.UsageSeriesPoint
		if err := rows.Scan(&s.TimeBucket, &s.TotalRequests, &s.TotalCost, &s.SumPromptTokens, &s.SumCompletionTokens, &s.SumCachedTokens, &s.SumReasoningTokens, &s.ErrorCount, &s.AvgTTFTMs, &s.AvgLatencyMs); err != nil {
			return nil, err
		}
		series = append(series, s)
	}
	return series, nil
}

func (p *PostgresDB) GetUsageByModel(f model.DashboardFilter, from, to time.Time) ([]model.UsageByModel, error) {
	where, args := buildDashboardWhere(f, from, to, "")
	query := `
		SELECT
			provider, model,
			COUNT(*) AS total_requests,
			COALESCE(SUM(cost), 0) AS total_cost,
			COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS sum_completion_tokens,
			COALESCE(SUM(cached_input_tokens), 0) AS sum_cached_tokens,
			COALESCE(SUM(reasoning_tokens), 0) AS sum_reasoning_tokens,
			COALESCE(ROUND(AVG(total_latency_ms), 2), 0) AS avg_latency_ms,
			COALESCE(ROUND(AVG(ttft_ms) FILTER (WHERE ttft_ms > 0), 2), 0) AS avg_ttft_ms
		FROM usage_logs
		WHERE ` + where + `
		GROUP BY provider, model ORDER BY total_cost DESC`

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []model.UsageByModel
	for rows.Next() {
		var m model.UsageByModel
		if err := rows.Scan(&m.Provider, &m.Model, &m.TotalRequests, &m.TotalCost, &m.SumPromptTokens, &m.SumCompletionTokens, &m.SumCachedTokens, &m.SumReasoningTokens, &m.AvgLatencyMs, &m.AvgTTFTMs); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

func (p *PostgresDB) GetUsageByUser(f model.DashboardFilter, from, to time.Time) ([]model.UsageByUser, error) {
	where, args := buildDashboardWhere(f, from, to, "l")
	query := `
		SELECT
			u.id::text AS user_id,
			COALESCE(u.email, '') AS email,
			COUNT(l.*) AS total_requests,
			COALESCE(SUM(l.cost), 0) AS total_cost,
			COALESCE(SUM(l.prompt_tokens), 0) AS sum_prompt_tokens,
			COALESCE(SUM(l.completion_tokens), 0) AS sum_completion_tokens
		FROM usage_logs l
		JOIN users u ON u.id = l.user_id
		WHERE ` + where + `
		GROUP BY u.id, u.email
		ORDER BY total_cost DESC`

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.UsageByUser
	for rows.Next() {
		var u model.UsageByUser
		if err := rows.Scan(&u.UserID, &u.Email, &u.TotalRequests, &u.TotalCost, &u.SumPromptTokens, &u.SumCompletionTokens); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (p *PostgresDB) GetRecentUsageLogs(f model.DashboardFilter, limit int, from, to time.Time) ([]model.UsageRecent, error) {
	where, args := buildDashboardWhere(f, from, to, "")
	query := `
		SELECT
			id, created_at, user_id, provider, model, status_code,
			COALESCE(error_message, '') AS error_message,
			COALESCE(cost, 0) AS cost,
			COALESCE(ttft_ms, 0) AS ttft_ms,
			COALESCE(total_latency_ms, 0) AS total_latency_ms
		FROM usage_logs
		WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(len(args)+1)

	args = append(args, limit)

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recents []model.UsageRecent
	for rows.Next() {
		var r model.UsageRecent
		if err := rows.Scan(&r.ID, &r.CreatedAt, &r.UserID, &r.Provider, &r.Model, &r.StatusCode, &r.ErrorMessage, &r.Cost, &r.TTFTMs, &r.TotalLatencyMs); err != nil {
			return nil, err
		}
		recents = append(recents, r)
	}
	return recents, nil
}

func (p *PostgresDB) GetStatusCodeBreakdown(f model.DashboardFilter, from, to time.Time) (map[int]int64, error) {
	where, args := buildDashboardWhere(f, from, to, "")
	query := `
		SELECT status_code, COUNT(*) AS cnt
		FROM usage_logs
		WHERE ` + where + `
		GROUP BY status_code
		ORDER BY cnt DESC`

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	breakdown := make(map[int]int64)
	for rows.Next() {
		var code int
		var count int64
		if err := rows.Scan(&code, &count); err != nil {
			return nil, err
		}
		breakdown[code] = count
	}
	return breakdown, nil
}

// ------------------------------------------------------------
// Paginated usage logs (backing GET /admin/logs)
// ------------------------------------------------------------

// ListUsageLogs returns a page of usage_logs rows (newest first) matching the
// filter. limit must be positive; offset may be 0.
func (p *PostgresDB) ListUsageLogs(f model.UsageLogFilter, limit, offset int) ([]model.UsageLog, error) {
	where, args := buildUsageLogWhere(f)
	query := `
		SELECT id, created_at, user_id, key_id, inbound_format, provider, model,
		       prompt_tokens, completion_tokens, total_tokens,
		       cached_input_tokens, reasoning_tokens,
		       prompt_price_used, completion_price_used, pricing_source, pricing_fallback,
		       cost, ttft_ms, total_latency_ms, status_code,
		       COALESCE(error_message, '') AS error_message
		FROM usage_logs
		WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := p.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.UsageLog
	for rows.Next() {
		var l model.UsageLog
		if err := rows.Scan(
			&l.ID, &l.CreatedAt, &l.UserID, &l.KeyID, &l.InboundFormat, &l.Provider, &l.Model,
			&l.PromptTokens, &l.CompletionTokens, &l.TotalTokens,
			&l.CachedInputTokens, &l.ReasoningTokens,
			&l.PromptPriceUsed, &l.CompletionPriceUsed, &l.PricingSource, &l.PricingFallback,
			&l.Cost, &l.TTFTMs, &l.TotalLatencyMs, &l.StatusCode,
			&l.ErrorMessage,
		); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// CountUsageLogs returns the number of rows matching the filter, for pagination.
func (p *PostgresDB) CountUsageLogs(f model.UsageLogFilter) (int64, error) {
	where, args := buildUsageLogWhere(f)
	var count int64
	if err := p.QueryRow("SELECT COUNT(*) FROM usage_logs WHERE "+where, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// buildUsageLogWhere turns a UsageLogFilter into a WHERE clause fragment plus
// its bound arguments. It always starts with 1=1 so callers can append
// ORDER BY / LIMIT without special-casing the empty-filter case.
func buildUsageLogWhere(f model.UsageLogFilter) (string, []interface{}) {
	clauses := []string{"1=1"}
	var args []interface{}

	add := func(cond string, val interface{}) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", cond, len(args)))
	}

	if f.From != nil {
		args = append(args, *f.From)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if f.To != nil {
		args = append(args, *f.To)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if f.UserID != nil {
		add("user_id", *f.UserID)
	}
	if f.KeyID != nil {
		add("key_id", *f.KeyID)
	}
	if f.Provider != "" {
		add("provider", f.Provider)
	}
	if f.Model != "" {
		add("model", f.Model)
	}
	if f.InboundFormat != "" {
		add("inbound_format", f.InboundFormat)
	}

	// Status accepts an exact code ("200") or a class ("2xx"/"4xx"/"5xx").
	if lo, hi, ok := parseStatusClass(f.Status); ok {
		args = append(args, lo)
		clauses = append(clauses, fmt.Sprintf("status_code >= $%d", len(args)))
		args = append(args, hi)
		clauses = append(clauses, fmt.Sprintf("status_code < $%d", len(args)))
	}

	return strings.Join(clauses, " AND "), args
}

// GetUsageLogByID returns a single usage log record enriched with user email
// and API key name for in-depth telemetry inspection.
func (p *PostgresDB) GetUsageLogByID(id int64) (*model.UsageLogDetail, error) {
	query := `
		SELECT l.id, l.created_at, l.user_id, l.key_id, l.inbound_format, l.provider, l.model,
		       l.prompt_tokens, l.completion_tokens, l.total_tokens,
		       l.cached_input_tokens, l.reasoning_tokens,
		       l.prompt_price_used, l.completion_price_used, l.pricing_source, l.pricing_fallback,
		       l.cost, l.ttft_ms, l.total_latency_ms, l.status_code,
		       COALESCE(l.error_message, '') AS error_message,
		       COALESCE(u.email, '') AS user_email,
		       COALESCE(k.name, '') AS key_name
		FROM usage_logs l
		LEFT JOIN users u ON u.id = l.user_id
		LEFT JOIN api_keys k ON k.id = l.key_id
		WHERE l.id = $1`

	var d model.UsageLogDetail
	if err := p.QueryRow(query, id).Scan(
		&d.ID, &d.CreatedAt, &d.UserID, &d.KeyID, &d.InboundFormat, &d.Provider, &d.Model,
		&d.PromptTokens, &d.CompletionTokens, &d.TotalTokens,
		&d.CachedInputTokens, &d.ReasoningTokens,
		&d.PromptPriceUsed, &d.CompletionPriceUsed, &d.PricingSource, &d.PricingFallback,
		&d.Cost, &d.TTFTMs, &d.TotalLatencyMs, &d.StatusCode,
		&d.ErrorMessage, &d.UserEmail, &d.KeyName,
	); err != nil {
		return nil, err
	}
	return &d, nil
}

// parseStatusClass interprets a status filter. "200" → (200, 201, true);
// "2xx" → (200, 300, true); empty or unrecognized → (0, 0, false).
func parseStatusClass(s string) (int, int, bool) {
	if s == "" {
		return 0, 0, false
	}
	if len(s) == 3 && s[1] == 'x' && s[2] == 'x' {
		base := int(s[0] - '0')
		if base < 1 || base > 5 {
			return 0, 0, false
		}
		return base * 100, (base + 1) * 100, true
	}
	code, err := strconv.Atoi(s)
	if err != nil || code < 100 || code > 599 {
		return 0, 0, false
	}
	return code, code + 1, true
}
