-- 000012_multi_tenant_b2b.down.sql
DROP INDEX IF EXISTS idx_usage_logs_tenant;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_api_keys_tenant;
ALTER TABLE api_keys DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_users_tenant;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_role;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_id;

DROP TABLE IF EXISTS tenant_credit_transactions;
DROP TABLE IF EXISTS tenants;
