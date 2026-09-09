-- Migration 000018: Advanced Enterprise Plugins Suite
-- Extends first-party plugin catalog with Prometheus, System Prompt Injector, IP Restriction,
-- Canary Traffic Splitter, Prompt Compression, Response Watermarking, Webhook Notifier, and Content Moderation.

INSERT INTO plugins (name, display_name, description, category, hooks, config_schema, is_builtin, version)
VALUES 
(
    'prometheus-metrics',
    'Prometheus Metrics Collector',
    'Collects granular, per-scope Prometheus metrics for requests, latencies, and token consumption with customizable cardinality.',
    'observability',
    ARRAY['post_upstream'],
    '{
        "type": "object",
        "properties": {
            "record_latency": { "type": "boolean", "title": "Record Latency Histogram", "default": true },
            "record_tokens": { "type": "boolean", "title": "Record Token Counters", "default": true },
            "record_status_codes": { "type": "boolean", "title": "Record Status Code Counter", "default": true },
            "include_tenant_label": { "type": "boolean", "title": "Include Tenant Label", "default": true },
            "include_model_label": { "type": "boolean", "title": "Include Model Label", "default": true },
            "include_provider_label": { "type": "boolean", "title": "Include Provider Label", "default": true }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'system-prompt-injector',
    'System Prompt Injector & Steering',
    'Injects, prepends, appends, or overrides system prompt instructions dynamically based on tenant, role, or model scope.',
    'traffic',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "system_prompt": { "type": "string", "title": "System Prompt", "default": "" },
            "mode": { "type": "string", "title": "Injection Mode", "enum": ["prepend", "append", "override"], "default": "prepend" }
        },
        "required": ["system_prompt"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'ip-restriction',
    'IP & CIDR Restriction Guard',
    'Enforces network-level allowlist and denylist filtering based on client IP or CIDR subnet before authentication.',
    'security',
    ARRAY['pre_auth'],
    '{
        "type": "object",
        "properties": {
            "allowlist": { "type": "array", "title": "IP Allowlist", "items": { "type": "string" } },
            "denylist": { "type": "array", "title": "IP Denylist", "items": { "type": "string" } }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'canary-traffic-splitter',
    'Canary Traffic Splitter & A/B Testing',
    'Dynamically splits a configurable percentage of request traffic to an alternative experimental model for A/B testing.',
    'traffic',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "canary_model": { "type": "string", "title": "Canary Target Model" },
            "canary_percentage": { "type": "integer", "title": "Canary Traffic Percentage (1-100)", "default": 10, "minimum": 1, "maximum": 100 },
            "canary_provider": { "type": "string", "title": "Canary Upstream Provider Override" }
        },
        "required": ["canary_model"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'prompt-compression',
    'Prompt Token Compressor',
    'Reduces input token consumption by trimming redundant consecutive whitespace and excess newlines in prompts.',
    'performance',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "collapse_spaces": { "type": "boolean", "title": "Collapse Consecutive Spaces", "default": true },
            "collapse_newlines": { "type": "boolean", "title": "Collapse Excess Newlines", "default": true },
            "trim_whitespace": { "type": "boolean", "title": "Trim Outer Whitespace", "default": true }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'response-watermarking',
    'Response Watermarking & Stamp',
    'Injects cryptographic or header verification stamps into outbound LLM completions to trace and authenticate corporate generation.',
    'security',
    ARRAY['post_upstream'],
    '{
        "type": "object",
        "properties": {
            "watermark_id": { "type": "string", "title": "Watermark Stamp ID", "default": "llm-gw-verified" },
            "header_name": { "type": "string", "title": "Verification Header Name", "default": "X-Gateway-Watermark" },
            "append_footer": { "type": "boolean", "title": "Append Visual Footer Text", "default": false },
            "footer_text": { "type": "string", "title": "Custom Footer Text", "default": "\n\n---\n*Verified by LLM Gateway*" }
        }
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'webhook-notifier',
    'Proactive Webhook Alert Notifier',
    'Asynchronously dispatches webhook event payloads to Slack, Discord, or alerting endpoints on errors or threshold breaches.',
    'observability',
    ARRAY['post_upstream'],
    '{
        "type": "object",
        "properties": {
            "webhook_url": { "type": "string", "title": "Destination Webhook URL" },
            "notify_on_errors": { "type": "boolean", "title": "Notify on HTTP Errors (>= 400)", "default": true },
            "notify_on_slow": { "type": "boolean", "title": "Notify on Slow Requests", "default": true },
            "slow_threshold_seconds": { "type": "number", "title": "Slow Threshold Duration (Seconds)", "default": 5.0 },
            "min_tokens_threshold": { "type": "integer", "title": "Token Consumption Surge Alert Threshold", "default": 0 }
        },
        "required": ["webhook_url"]
    }'::jsonb,
    TRUE,
    '1.0.0'
),
(
    'content-moderation',
    'Automated Content Moderation Guard',
    'Scans prompts for hate speech, violent instructions, self-harm, or custom banned terminology and rejects non-compliant requests.',
    'security',
    ARRAY['pre_routing'],
    '{
        "type": "object",
        "properties": {
            "block_hate": { "type": "boolean", "title": "Block Hate Speech & Extremism", "default": true },
            "block_violence": { "type": "boolean", "title": "Block Weapons & Violence Directives", "default": true },
            "block_self_harm": { "type": "boolean", "title": "Block Self-Harm Content", "default": true },
            "action": { "type": "string", "title": "Enforcement Action", "enum": ["block", "warn"], "default": "block" },
            "custom_banned_words": { "type": "array", "title": "Custom Banned Keywords", "items": { "type": "string" } }
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
