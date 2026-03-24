CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_media_items_genres_gin ON media_items USING GIN (genres);
