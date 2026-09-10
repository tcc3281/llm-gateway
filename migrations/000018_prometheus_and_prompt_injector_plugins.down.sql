-- Migration 000018 Down
DELETE FROM plugins WHERE name IN (
    'prometheus-metrics',
    'system-prompt-injector',
    'ip-restriction',
    'canary-traffic-splitter',
    'prompt-compression',
    'response-watermarking',
    'webhook-notifier',
    'content-moderation'
);
