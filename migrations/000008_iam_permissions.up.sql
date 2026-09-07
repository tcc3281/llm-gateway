-- 000008_iam_permissions.up.sql
-- IAM-style permission system: groups, dynamic policies, key model restrictions, audit logs

-- 1. Extend users table with root flag
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_root BOOLEAN DEFAULT FALSE;

-- 2. Extend api_keys with budget period fields
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS budget_period VARCHAR(20) DEFAULT NULL;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS budget_period_reset_at TIMESTAMPTZ DEFAULT NULL;

-- 3. Groups table
CREATE TABLE IF NOT EXISTS groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    budget_limit_monthly NUMERIC(12,6) DEFAULT 0,
    rate_limit_rpm INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. Group Policies table (IAM JSON documents)
CREATE TABLE IF NOT EXISTS group_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    policy JSONB NOT NULL,
    priority INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_group_policy_name UNIQUE(group_id, name)
);

CREATE INDEX IF NOT EXISTS idx_group_policies_group ON group_policies (group_id, priority DESC);

-- 5. User-Group mapping table
CREATE TABLE IF NOT EXISTS user_groups (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_groups_user ON user_groups (user_id);
CREATE INDEX IF NOT EXISTS idx_user_groups_group ON user_groups (group_id);

-- 6. Key-level model restrictions
CREATE TABLE IF NOT EXISTS key_model_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    model_pattern VARCHAR(200) NOT NULL,
    is_allowlist BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_key_model_pattern UNIQUE(key_id, model_pattern)
);

CREATE INDEX IF NOT EXISTS idx_key_model_policies_key ON key_model_policies (key_id);

-- 7. Audit logs table
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id UUID,
    actor_type VARCHAR(20) NOT NULL,
    actor_ip INET,
    action VARCHAR(50) NOT NULL,
    target_type VARCHAR(50),
    target_id VARCHAR(100),
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs (actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs (target_type, target_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs (created_at DESC);

-- 8. Seed Default Root User & Groups
UPDATE users SET is_root = TRUE WHERE email = 'admin@llm-gateway.local';

-- Seed default Full Access Group
INSERT INTO groups (id, name, description, budget_limit_monthly, rate_limit_rpm, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'Full Access',
    'Default administrator & developer group with full access to all models and resources',
    0,
    0,
    TRUE
)
ON CONFLICT (name) DO NOTHING;

-- Seed default Full Access Policy (Statement: Allow * on *)
INSERT INTO group_policies (group_id, name, policy, priority, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'AdministratorFullAccess',
    '{"Version":"1","Statement":[{"Sid":"AllowAll","Effect":"Allow","Action":["*:*","*"],"Resource":["*"]}]}',
    100,
    TRUE
)
ON CONFLICT (group_id, name) DO NOTHING;

-- Assign all existing users to the Full Access group for zero-breakage backward compatibility
INSERT INTO user_groups (user_id, group_id)
SELECT u.id, '00000000-0000-0000-0000-000000000002'
FROM users u
ON CONFLICT (user_id, group_id) DO NOTHING;
