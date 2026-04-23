DROP INDEX IF EXISTS idx_music_tracks_library_album_folder;
ALTER TABLE music_tracks DROP COLUMN IF EXISTS album_folder;
