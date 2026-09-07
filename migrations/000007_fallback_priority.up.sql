-- Add fallback_priority to provider_keys.
-- Lower number = higher priority in fallback chain. NULL = not in fallback pool.
ALTER TABLE provider_keys
    ADD COLUMN IF NOT EXISTS fallback_priority INT DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_provider_keys_fallback
    ON provider_keys (fallback_priority ASC NULLS LAST)
    WHERE is_active = TRUE;

COMMENT ON COLUMN provider_keys.fallback_priority IS
    'Fallback order when primary provider fails. NULL = excluded from fallback pool. Lower = tried first.';
