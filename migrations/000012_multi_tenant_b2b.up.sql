-- 000012_multi_tenant_b2b.up.sql
-- Multi-Tenant B2B Architecture: Tenants, Credit Balances, Contract Ledgers & Permissions

-- 1. Tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(120) UNIQUE NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'external',           -- 'internal' | 'external'
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'prepaid',     -- 'prepaid' (Ví nạp) | 'postpaid' (Hóa đơn cuối tháng)
    credit_balance NUMERIC(14,6) NOT NULL DEFAULT 0,         -- Số dư khả dụng (USD)
    budget_limit_monthly NUMERIC(14,6) NOT NULL DEFAULT 0,   -- Hạn mức chi tiêu tháng (USD)
    allowed_models TEXT[] DEFAULT '{"*"}',                   -- Model boundary whitelist
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenants_active ON tenants(is_active);
CREATE INDEX IF NOT EXISTS idx_tenants_type ON tenants(type);

-- 2. Credit Transactions Ledger (Lịch sử nạp tiền / nạp hợp đồng / biến động số dư)
CREATE TABLE IF NOT EXISTS tenant_credit_transactions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    amount NUMERIC(14,6) NOT NULL,                           -- Số tiền nạp (+) hoặc điều chỉnh (-)
    balance_after NUMERIC(14,6) NOT NULL,                   -- Số dư ngay sau giao dịch
    type VARCHAR(30) NOT NULL,                              -- 'contract_deposit', 'admin_adjustment', 'usage_deduction', 'refund'
    contract_ref VARCHAR(100),                              -- Mã/Số hợp đồng kinh tế
    notes TEXT,                                             -- Ghi chú thanh toán / ủy nhiệm chi
    created_by UUID REFERENCES users(id) ON DELETE SET NULL, -- Admin đã thực hiện
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_transactions_tenant ON tenant_credit_transactions(tenant_id, created_at DESC);

-- 3. Extend users table with tenant association
ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_role VARCHAR(30) DEFAULT 'member'; -- 'tenant_admin', 'member', 'viewer'
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);

-- 4. Extend api_keys table with tenant scoping
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);

-- 5. Extend usage_logs table with tenant attribution for 100% data isolation & billing reports
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_usage_logs_tenant ON usage_logs(tenant_id, created_at DESC);
