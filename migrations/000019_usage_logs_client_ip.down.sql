-- 000019_usage_logs_client_ip.down.sql
DROP INDEX IF EXISTS idx_usage_logs_client_ip;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS client_ip;
