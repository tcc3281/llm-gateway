-- 000010_content_agnostic_structural_tracing.up.sql
-- Add request_shape JSONB column to usage_logs for Zero-Leak Structural Tracing
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS request_shape JSONB DEFAULT NULL;

-- Indexes for querying multimodal traffic, tool calling, and structural hash
CREATE INDEX IF NOT EXISTS idx_usage_logs_request_shape_has_images 
    ON usage_logs (((request_shape->>'has_images')::boolean)) 
    WHERE request_shape IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_request_shape_has_audio 
    ON usage_logs (((request_shape->>'has_audio')::boolean)) 
    WHERE request_shape IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_request_shape_modality 
    ON usage_logs ((request_shape->>'modality')) 
    WHERE request_shape IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_request_shape_hash 
    ON usage_logs ((request_shape->>'structure_hash')) 
    WHERE request_shape IS NOT NULL;
