CREATE INDEX IF NOT EXISTS idx_photos_title_lower ON photos (lower(title));
CREATE INDEX IF NOT EXISTS idx_photos_album_lower ON photos (lower(album));
