-- 000019_usage_logs_client_ip.up.sql
-- Add client_ip column to usage_logs for security audit, telemetry, and IP policy tracking
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS client_ip VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_usage_logs_client_ip ON usage_logs(client_ip);
