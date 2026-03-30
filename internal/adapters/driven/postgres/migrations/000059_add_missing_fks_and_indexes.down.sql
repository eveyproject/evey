DROP INDEX IF EXISTS idx_play_sessions_active;

ALTER TABLE syncplay_rooms DROP CONSTRAINT IF EXISTS fk_syncplay_rooms_media_id;
ALTER TABLE syncplay_rooms DROP CONSTRAINT IF EXISTS fk_syncplay_rooms_host_id;
ALTER TABLE photos DROP CONSTRAINT IF EXISTS fk_photos_library_id;
ALTER TABLE music_tracks DROP CONSTRAINT IF EXISTS fk_music_tracks_library_id;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_profile_id;
