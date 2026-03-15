-- pg_trgm GIN indexes for efficient ILIKE search on photos and music_tracks
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_photos_title_trgm ON photos USING GIN (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_photos_album_trgm ON photos USING GIN (album gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_music_tracks_title_trgm ON music_tracks USING GIN (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_music_tracks_artist_trgm ON music_tracks USING GIN (artist gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_music_tracks_album_trgm ON music_tracks USING GIN (album gin_trgm_ops);
