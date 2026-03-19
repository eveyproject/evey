CREATE INDEX IF NOT EXISTS idx_media_items_title_trgm ON media_items USING GIN (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_media_items_original_title_trgm ON media_items USING GIN (original_title gin_trgm_ops);
