ALTER TABLE media_items ADD COLUMN IF NOT EXISTS file_fingerprint TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_items_file_fingerprint ON media_items(file_fingerprint) WHERE file_fingerprint IS NOT NULL;
