-- Migration 000015: Multi-Config Alert Rules
-- Adds severity tiers, provider/model target filters, and multi-channel destinations

ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS severity VARCHAR(32) NOT NULL DEFAULT 'warning';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS filter_provider VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS filter_model VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS channels JSONB NOT NULL DEFAULT '[]'::jsonb;
