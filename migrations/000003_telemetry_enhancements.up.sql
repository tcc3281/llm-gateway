-- Alter usage_logs table to support detailed modern token counts (cached input, reasoning tokens)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cached_input_tokens INT DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS reasoning_tokens INT DEFAULT 0;

-- Alter usage_logs table to support cost audit trail (actual prices used, source tracking)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS prompt_price_used NUMERIC(12, 6) DEFAULT 0.0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS completion_price_used NUMERIC(12, 6) DEFAULT 0.0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS pricing_source VARCHAR(50) DEFAULT 'database';
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS pricing_fallback BOOLEAN DEFAULT FALSE;
