-- Migration 000016: Virtual Models & Dynamic Routing (v0.3.0)
-- Provides model virtualization, traffic splitting (Canary / A/B), and multi-tier fallback chains.

CREATE TABLE IF NOT EXISTS virtual_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    routing_strategy VARCHAR(32) NOT NULL DEFAULT 'weighted', -- 'weighted' | 'fallback' | 'round_robin'
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique index per name: global (tenant_id IS NULL) or scoped to a tenant
CREATE UNIQUE INDEX IF NOT EXISTS idx_virtual_models_name_global 
    ON virtual_models(name) 
    WHERE tenant_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_virtual_models_name_tenant 
    ON virtual_models(tenant_id, name) 
    WHERE tenant_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_virtual_models_tenant 
    ON virtual_models(tenant_id);

CREATE TABLE IF NOT EXISTS virtual_model_targets (
    id BIGSERIAL PRIMARY KEY,
    virtual_model_id UUID NOT NULL REFERENCES virtual_models(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    model_name VARCHAR(128) NOT NULL,
    weight INT NOT NULL DEFAULT 100,      -- For weighted traffic splitting (e.g. 90 vs 10)
    priority INT NOT NULL DEFAULT 1,    -- Priority tier: 1 = primary, 2 = secondary fallback, etc.
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vm_targets_vmid 
    ON virtual_model_targets(virtual_model_id, priority ASC, is_active);
