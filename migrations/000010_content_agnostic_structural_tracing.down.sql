-- 000010_content_agnostic_structural_tracing.down.sql
DROP INDEX IF EXISTS idx_usage_logs_request_shape_hash;
DROP INDEX IF EXISTS idx_usage_logs_request_shape_modality;
DROP INDEX IF EXISTS idx_usage_logs_request_shape_has_audio;
DROP INDEX IF EXISTS idx_usage_logs_request_shape_has_images;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS request_shape;
