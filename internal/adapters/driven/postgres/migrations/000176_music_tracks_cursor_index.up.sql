-- The music library pages by keyset: WHERE library_id = $1 AND
-- (album_artist, album, disk_number, track_number, id) > (...) ORDER BY the
-- same five columns. No index covered that order, so every page sorted the
-- whole library's tracks; this one lets Postgres walk straight to the cursor.
CREATE INDEX IF NOT EXISTS idx_music_tracks_library_cursor
    ON music_tracks (library_id, album_artist, album, disk_number, track_number, id);
