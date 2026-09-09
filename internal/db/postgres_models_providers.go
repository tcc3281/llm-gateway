package db

import (
	"fmt"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ==========================================================================
// Model Pricing & Catalog Repositories
// ==========================================================================

func (p *PostgresDB) GetPricings() (map[string]model.ModelPricing, error) {
	query := `SELECT id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, cache_read_price_per_1m, updated_at FROM model_pricings`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pricings := make(map[string]model.ModelPricing)
	for rows.Next() {
		var pr model.ModelPricing
		if err := rows.Scan(&pr.ID, &pr.Provider, &pr.ModelName, &pr.PromptPricePer1M, &pr.CompletionPricePer1M, &pr.CacheReadPricePer1M, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		// Key format: provider:model_name (e.g., "openai:gpt-4o")
		key := fmt.Sprintf("%s:%s", pr.Provider, pr.ModelName)
		pricings[key] = pr
	}
	return pricings, nil
}

func (p *PostgresDB) GetAllModelsList() ([]model.ModelPricing, error) {
	query := `SELECT id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, cache_read_price_per_1m, updated_at FROM model_pricings ORDER BY provider, model_name`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]model.ModelPricing, 0)
	for rows.Next() {
		var pr model.ModelPricing
		if err := rows.Scan(&pr.ID, &pr.Provider, &pr.ModelName, &pr.PromptPricePer1M, &pr.CompletionPricePer1M, &pr.CacheReadPricePer1M, &pr.UpdatedAt); err != nil {
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
		INSERT INTO model_pricings (id, provider, model_name, prompt_price_per_1m, completion_price_per_1m, cache_read_price_per_1m, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (provider, model_name) DO UPDATE SET
			prompt_price_per_1m = EXCLUDED.prompt_price_per_1m,
			completion_price_per_1m = EXCLUDED.completion_price_per_1m,
			cache_read_price_per_1m = EXCLUDED.cache_read_price_per_1m,
			updated_at = NOW()
		RETURNING updated_at`
	return p.QueryRow(query, pr.ID, pr.Provider, pr.ModelName, pr.PromptPricePer1M, pr.CompletionPricePer1M, pr.CacheReadPricePer1M).Scan(&pr.UpdatedAt)
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

// ==========================================================================
// Provider Key Repositories (stateful provider credentials)
// ==========================================================================

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
