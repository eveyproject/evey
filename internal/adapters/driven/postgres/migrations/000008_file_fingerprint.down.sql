DROP INDEX IF EXISTS idx_media_items_file_fingerprint;
ALTER TABLE media_items DROP COLUMN IF EXISTS file_fingerprint;
