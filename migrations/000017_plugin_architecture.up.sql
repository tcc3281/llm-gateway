-- Migration 000017: Kong-like Plugin Architecture (v0.3.0)
-- Provides pluggable lifecycle hooks, 7-level scoping, and dynamic configuration.

CREATE TABLE IF NOT EXISTS plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category VARCHAR(32) NOT NULL, -- 'traffic' | 'security' | 'performance' | 'observability'
    hooks VARCHAR(32)[] NOT NULL,  -- e.g. ARRAY['post_auth', 'pre_routing']
    config_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_builtin BOOLEAN NOT NULL DEFAULT TRUE,
    version VARCHAR(16) NOT NULL DEFAULT '1.0.0',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plugin_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_name VARCHAR(64) NOT NULL REFERENCES plugins(name) ON DELETE CASCADE,
    scope_level VARCHAR(16) NOT NULL, -- 'global' | 'provider' | 'model' | 'tenant' | 'role' | 'user' | 'api_key'
    scope_provider VARCHAR(64),
    scope_model VARCHAR(128),
    scope_tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    scope_role VARCHAR(64),
    scope_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    scope_key_id UUID REFERENCES api_keys(id) ON DELETE CASCADE,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 100,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uniq_plugin_binding UNIQUE NULLS NOT DISTINCT (
        plugin_name, scope_level, scope_provider, scope_model,
        scope_tenant_id, scope_role, scope_user_id, scope_key_id
    )
);

CREATE INDEX IF NOT EXISTS idx_pb_global
    ON plugin_bindings(plugin_name, is_active)
    WHERE scope_level = 'global';

CREATE INDEX IF NOT EXISTS idx_pb_tenant
    ON plugin_bindings(scope_tenant_id, is_active, priority)
    WHERE scope_level = 'tenant';

CREATE INDEX IF NOT EXISTS idx_pb_key
    ON plugin_bindings(scope_key_id, is_active)
    WHERE scope_level = 'api_key';

CREATE INDEX IF NOT EXISTS idx_pb_user
    ON plugin_bindings(scope_user_id, is_active)
    WHERE scope_level = 'user';

CREATE INDEX IF NOT EXISTS idx_pb_model
    ON plugin_bindings(scope_model, is_active)
    WHERE scope_level = 'model';

CREATE INDEX IF NOT EXISTS idx_pb_provider
    ON plugin_bindings(scope_provider, is_active)
    WHERE scope_level = 'provider';

-- Seed First-Party Builtin Plugins
INSERT INTO plugins (name, display_name, description, category, hooks, config_schema, is_builtin, version)
VALUES 
(
    'rate-limiting',
    'Rate Limiting & Traffic Quota',
    'Controls request throughput and token velocity using token bucket algorithm in Redis.',
    'traffic',
    ARRAY['post_auth'],
    '{
        "type": "object",
        "properties": {
            "requests_per_minute": {
                "type": "integer",
                "title": "Requests Per Minute (RPM)",
                "description": "Max allowed requests per minute (0 = unlimited)",
                "default": 60,
                "minimum": 0
            },
            "tokens_per_minute": {
                "type": "integer",
                "title": "Tokens Per Minute (TPM)",
                "description": "Max allowed tokens per minute (0 = unlimited)",
                "default": 100000,
                "minimum": 0
            },
            "burst_multiplier": {
                "type": "number",
                "title": "Burst Multiplier",
                "description": "Burst tolerance buffer above RPM limit",
                "default": 1.5,
                "minimum": 1.0
            }
        },
        "required": ["requests_per_minute"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'budget-control',
    'Budget & Spend Control',
    'Enforces USD budget thresholds per scope with automatic blocking or warning when limits are exceeded.',
    'traffic',
    ARRAY['post_auth'],
    '{
        "type": "object",
        "properties": {
            "budget_limit": {
                "type": "number",
                "title": "Budget Limit (USD)",
                "description": "Max spending limit in USD for this scope",
                "default": 100.0,
                "minimum": 0.0
            },
            "action": {
                "type": "string",
                "title": "Action On Exceeded",
                "description": "Whether to block requests or emit warning",
                "enum": ["block", "warn"],
                "default": "block"
            }
        },
        "required": ["budget_limit"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'pii-masking',
    'PII Masking & Privacy Redaction',
    'Detects and redacts sensitive personally identifiable information (emails, phones, credit cards, national IDs, API keys) before forwarding to LLM.',
    'security',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "patterns": {
                "type": "array",
                "title": "Patterns to Redact",
                "description": "Sensitive entities to mask",
                "items": {
                    "type": "string",
                    "enum": ["email", "phone", "credit_card", "national_id", "api_key"]
                },
                "default": ["email", "phone", "credit_card", "national_id", "api_key"]
            },
            "replacement": {
                "type": "string",
                "title": "Redaction Replacement Text",
                "description": "Placeholder string to replace sensitive content",
                "default": "[REDACTED]"
            }
        },
        "required": ["patterns"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'prompt-guard',
    'Prompt Guard & Anti-Jailbreak',
    'Prevents prompt injection, jailbreak exploits (DAN mode), system prompt leaks, and blocks prohibited toxic keywords.',
    'security',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "check_injection": {
                "type": "boolean",
                "title": "Check Prompt Injection",
                "description": "Block heuristic injection and override attempts",
                "default": true
            },
            "check_jailbreak": {
                "type": "boolean",
                "title": "Check Jailbreaks (DAN)",
                "description": "Block DAN mode and safety guideline bypass patterns",
                "default": true
            },
            "check_system_leak": {
                "type": "boolean",
                "title": "Check System Prompt Leakage",
                "description": "Detect attempts to extract instructions",
                "default": true
            },
            "banned_keywords": {
                "type": "array",
                "title": "Banned Keywords",
                "description": "Custom keywords or phrases that trigger immediate blocking",
                "items": { "type": "string" },
                "default": []
            },
            "action": {
                "type": "string",
                "title": "Enforcement Action",
                "enum": ["block", "log_only"],
                "default": "block"
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'semantic-cache',
    'Semantic & Response Cache',
    'Caches non-streaming LLM responses in Redis to deliver sub-10ms latency and eliminate duplicate token costs.',
    'performance',
    ARRAY['pre_routing', 'post_upstream'],
    '{
        "type": "object",
        "properties": {
            "ttl_seconds": {
                "type": "integer",
                "title": "Cache TTL (Seconds)",
                "description": "How long to keep response in cache (0 = disabled)",
                "default": 300,
                "minimum": 0
            },
            "cache_header": {
                "type": "boolean",
                "title": "Emit X-Cache Header",
                "description": "Return X-Cache: HIT / MISS in HTTP response",
                "default": true
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'request-hedging',
    'Request Hedging (Dual-Dispatch)',
    'Dispatches redundant requests to secondary providers for VIP traffic to minimize tail latency (P99).',
    'performance',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "hedging_delay_ms": {
                "type": "integer",
                "title": "Hedging Delay (ms)",
                "description": "Wait time before dispatching parallel hedging request",
                "default": 300,
                "minimum": 50
            },
            "secondary_provider": {
                "type": "string",
                "title": "Secondary Provider",
                "description": "Fallback provider alias for hedged request",
                "default": ""
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'header-transform',
    'Header Transformation',
    'Adds, mutates, or removes custom HTTP headers sent to upstream AI providers.',
    'traffic',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "set": {
                "type": "object",
                "title": "Set Headers",
                "description": "Key-value map of headers to inject into upstream request",
                "additionalProperties": { "type": "string" }
            },
            "remove": {
                "type": "array",
                "title": "Remove Headers",
                "description": "List of header names to strip before sending upstream",
                "items": { "type": "string" }
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'retry-circuit-breaker',
    'Retry & Circuit Breaker',
    'Automatically handles retries on 429/5xx and trips circuit breaker when upstream error rate exceeds threshold.',
    'traffic',
    ARRAY['pre_routing', 'post_upstream'],
    '{
        "type": "object",
        "properties": {
            "failure_threshold": {
                "type": "integer",
                "title": "Failure Threshold",
                "description": "Number of consecutive errors before opening circuit breaker",
                "default": 5,
                "minimum": 1
            },
            "cooldown_seconds": {
                "type": "integer",
                "title": "Cooldown Duration (Seconds)",
                "description": "Duration circuit stays OPEN before half-open test",
                "default": 30,
                "minimum": 5
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'langfuse-exporter',
    'Langfuse Observability Exporter',
    'Asynchronously exports request traces, prompt tokens, generation latencies, and metadata to Langfuse.',
    'observability',
    ARRAY['post_upstream'],
    '{
        "type": "object",
        "properties": {
            "host": {
                "type": "string",
                "title": "Langfuse Host URL",
                "description": "Base URL of Langfuse server or cloud endpoint",
                "default": "https://cloud.langfuse.com"
            },
            "public_key": {
                "type": "string",
                "title": "Public Key",
                "description": "Langfuse project public key"
            },
            "secret_key": {
                "type": "string",
                "title": "Secret Key",
                "description": "Langfuse project secret key"
            }
        },
        "required": ["public_key", "secret_key"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'audit-trail',
    'Audit Trail & Compliance Logger',
    'Emits immutable audit logs or webhook events for enterprise compliance and security analysis.',
    'observability',
    ARRAY['post_upstream'],
    '{
        "type": "object",
        "properties": {
            "destination": {
                "type": "string",
                "title": "Audit Destination",
                "enum": ["log", "webhook"],
                "default": "log"
            },
            "webhook_url": {
                "type": "string",
                "title": "Webhook URL",
                "description": "External HTTP endpoint to receive audit JSON payloads",
                "default": ""
            }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
)
ON CONFLICT (name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    hooks = EXCLUDED.hooks,
    config_schema = EXCLUDED.config_schema,
    is_builtin = EXCLUDED.is_builtin,
    version = EXCLUDED.version,
    updated_at = NOW();
