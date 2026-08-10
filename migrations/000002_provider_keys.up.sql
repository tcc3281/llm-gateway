-- Provider API Keys table (stateful: stored in DB, managed via admin API/CLI)
-- api_key column stores the secret at rest. If PROVIDER_MASTER_KEY is configured,
-- values are stored AES-GCM encrypted with the "enc:" prefix; otherwise plaintext (dev only).
CREATE TABLE IF NOT EXISTS provider_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL UNIQUE, -- openai | anthropic | gemini
    api_key TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_provider_keys_provider ON provider_keys (provider);
