-- Discovered model catalog.
--
-- The gateway fetches each configured provider's /models API and stores the
-- (provider → model) rows here. Routing looks a model's provider up in this
-- catalog (plus the explicitly registered model_pricings), so adding a provider
-- auto-populates the catalog and no model list is hardcoded in code.
CREATE TABLE IF NOT EXISTS provider_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL REFERENCES provider_keys(provider) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    -- Distinct name: "unique_provider_model" is already taken by model_pricings
    -- (000001), and constraint names are unique per schema in PostgreSQL.
    CONSTRAINT uq_provider_models_provider_model UNIQUE(provider, model_name)
);

CREATE INDEX IF NOT EXISTS idx_provider_models_model ON provider_models (model_name);
