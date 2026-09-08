-- 000009_user_auth_and_team_analytics.up.sql
-- Extend users table with password_hash and active flag for Member Authentication
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;

-- Create index on users email for fast login lookup
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);

-- Ensure indexes for user_groups joins
CREATE INDEX IF NOT EXISTS idx_user_groups_composite ON user_groups (user_id, group_id);
