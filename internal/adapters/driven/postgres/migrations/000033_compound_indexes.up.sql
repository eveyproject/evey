-- Compound indexes for GROUP BY queries on music_tracks and photos
CREATE INDEX IF NOT EXISTS idx_music_tracks_library_album
    ON music_tracks(library_id, album)
    WHERE album IS NOT NULL AND album != '';

CREATE INDEX IF NOT EXISTS idx_photos_library_album
    ON photos(library_id, album)
    WHERE album IS NOT NULL AND album != '';
