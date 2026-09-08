-- 000011_group_admin_role.up.sql
-- Add group_role to user_groups for departmental group leader / admin delegation

ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS group_role VARCHAR(20) NOT NULL DEFAULT 'member';
CREATE INDEX IF NOT EXISTS idx_user_groups_role ON user_groups (group_id, group_role);
