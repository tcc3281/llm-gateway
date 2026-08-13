-- 1. Create Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 2. Create API Keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_prefix VARCHAR(12) NOT NULL,
    key_hash VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    budget_limit NUMERIC(12, 6) DEFAULT 0, -- 0 means unlimited
    rate_limit_rpm INT DEFAULT 60,
    is_active BOOLEAN DEFAULT TRUE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 3. Create Model Pricings table (Price per 1M tokens)
CREATE TABLE IF NOT EXISTS model_pricings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    model_name VARCHAR(100) NOT NULL,
    prompt_price_per_1m NUMERIC(10, 6) NOT NULL,
    completion_price_per_1m NUMERIC(10, 6) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT unique_provider_model UNIQUE(provider, model_name)
);

-- 4. Create Usage Logs table (Timeseries & Telemetry)
CREATE TABLE IF NOT EXISTS usage_logs (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    user_id UUID NOT NULL,
    key_id UUID NOT NULL,
    inbound_format VARCHAR(20) NOT NULL,
    provider VARCHAR(20) NOT NULL,
    model VARCHAR(100) NOT NULL,
    prompt_tokens INT DEFAULT 0,
    completion_tokens INT DEFAULT 0,
    total_tokens INT DEFAULT 0,
    cost NUMERIC(12, 6) DEFAULT 0,
    ttft_ms INT DEFAULT 0,
    total_latency_ms INT DEFAULT 0,
    status_code INT NOT NULL,
    error_message TEXT
);

-- Indexes for fast analytics queries
CREATE INDEX IF NOT EXISTS idx_usage_logs_timeseries ON usage_logs (created_at DESC, user_id, provider);
CREATE INDEX IF NOT EXISTS idx_usage_logs_key ON usage_logs (key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_logs_user_model ON usage_logs (user_id, model, created_at DESC);

-- No default model pricing seeded. Admin must run 'llmgw model sync' to populate catalog.

-- Seed default Admin User
INSERT INTO users (id, email, name, role) VALUES
    ('00000000-0000-0000-0000-000000000001', 'admin@llm-gateway.local', 'Default Admin', 'admin')
ON CONFLICT (id) DO NOTHING;

-- Seed default API Key for testing: secret is "sk-gw-default-admin-key-12345"
-- SHA256 of "sk-gw-default-admin-key-12345" = 9b43ed5b6c86a2f8d3cf8cf40d859dfd3d4b68453ae668c2d1ed9e3b9aaef3e7 (example hashed key)
