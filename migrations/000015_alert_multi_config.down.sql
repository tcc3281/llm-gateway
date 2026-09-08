-- Migration 000015 rollback: Multi-Config Alert Rules
ALTER TABLE alert_rules DROP COLUMN IF EXISTS channels;
ALTER TABLE alert_rules DROP COLUMN IF EXISTS filter_model;
ALTER TABLE alert_rules DROP COLUMN IF EXISTS filter_provider;
ALTER TABLE alert_rules DROP COLUMN IF EXISTS severity;
