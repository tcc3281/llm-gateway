-- 000011_group_admin_role.down.sql
DROP INDEX IF EXISTS idx_user_groups_role;
ALTER TABLE user_groups DROP COLUMN IF EXISTS group_role;
