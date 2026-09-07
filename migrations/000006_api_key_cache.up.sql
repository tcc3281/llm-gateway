-- Per-key cache settings.
-- cache_ttl_seconds: -1 = disabled, 0 = use global default, >0 = custom TTL in seconds.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS cache_ttl_seconds INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN api_keys.cache_ttl_seconds IS
    '-1 = cache disabled for this key; 0 = use global default (CACHE_RESPONSE_TTL_SECONDS); >0 = custom TTL in seconds';
