CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_photos_title_trgm ON photos USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_photos_album_trgm ON photos USING gin (album gin_trgm_ops);
