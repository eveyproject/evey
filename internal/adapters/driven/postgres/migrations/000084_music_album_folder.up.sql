-- Music albums are now identified by their parent folder, not by the
-- (album, album_artist) tuple derived from ID3 tags. This fixes cases
-- where a single compilation folder has inconsistent album_artist tags
-- across tracks (featurings) yet represents one album on disk.
ALTER TABLE music_tracks
    ADD COLUMN IF NOT EXISTS album_folder TEXT NOT NULL DEFAULT '';

-- Backfill from existing rows. regexp strips the last path segment.
UPDATE music_tracks
SET album_folder = regexp_replace(file_path, '/[^/]+$', '')
WHERE album_folder = '';

CREATE INDEX IF NOT EXISTS idx_music_tracks_library_album_folder
    ON music_tracks(library_id, album_folder);
