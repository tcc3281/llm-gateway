-- Migration 000013: Multi-tier Cache Pricing support for Prompt Caching (KV-Cache reuse)
ALTER TABLE model_pricings ADD COLUMN IF NOT EXISTS cache_read_price_per_1m DOUBLE PRECISION DEFAULT NULL;
