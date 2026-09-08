-- Revert Migration 000013
ALTER TABLE model_pricings DROP COLUMN IF EXISTS cache_read_price_per_1m;
