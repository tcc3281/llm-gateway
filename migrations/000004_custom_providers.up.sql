-- Custom OpenAI/Anthropic-compatible providers: a provider_keys row may carry a
-- custom base_url ('' = built-in provider with hardcoded upstream URL) and a
-- wire protocol ("openai" | "anthropic") the gateway must speak to that URL.
-- Columns are added idempotently (runMigrations re-runs every .up.sql at boot).
ALTER TABLE provider_keys
    ADD COLUMN IF NOT EXISTS base_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS protocol VARCHAR(10) NOT NULL DEFAULT 'openai';

-- Custom provider names can exceed the previous 20-char width; widen so the
-- telemetry insert (usage_logs.provider) never truncates/overflows.
ALTER TABLE usage_logs ALTER COLUMN provider TYPE VARCHAR(50);
