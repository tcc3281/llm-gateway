-- Migration 000014: Alert Rules, Cooldown Tracking & Webhook Dispatch History
CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    threshold_value DOUBLE PRECISION NOT NULL,
    window_seconds INT NOT NULL DEFAULT 60,
    cooldown_seconds INT NOT NULL DEFAULT 900,
    webhook_url TEXT NOT NULL,
    webhook_secret TEXT DEFAULT '',
    channel_type VARCHAR(32) NOT NULL DEFAULT 'generic',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_history (
    id UUID PRIMARY KEY,
    rule_id UUID REFERENCES alert_rules(id) ON DELETE SET NULL,
    rule_name VARCHAR(255) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    tenant_id UUID,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    payload JSONB DEFAULT '{}',
    status VARCHAR(32) NOT NULL,
    status_code INT,
    error_message TEXT,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_triggered_at ON alert_history(triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_rules_event_type ON alert_rules(event_type) WHERE is_active = TRUE;
