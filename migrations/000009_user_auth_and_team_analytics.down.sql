-- 000009_user_auth_and_team_analytics.down.sql
DROP INDEX IF EXISTS idx_user_groups_composite;
DROP INDEX IF EXISTS idx_users_email;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
