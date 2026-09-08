-- Migration 000017 Down: Kong-like Plugin Architecture
DROP TABLE IF EXISTS plugin_bindings CASCADE;
DROP TABLE IF EXISTS plugins CASCADE;
