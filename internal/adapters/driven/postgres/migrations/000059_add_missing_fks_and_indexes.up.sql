-- Clean orphaned rows before adding FK constraints
DELETE FROM notifications WHERE profile_id NOT IN (SELECT id FROM profiles);
DELETE FROM music_tracks WHERE library_id NOT IN (SELECT id FROM libraries);
DELETE FROM photos WHERE library_id NOT IN (SELECT id FROM libraries);
DELETE FROM syncplay_rooms WHERE host_id NOT IN (SELECT id FROM profiles);
DELETE FROM syncplay_rooms WHERE media_id NOT IN (SELECT id FROM media_items);

-- Add FK constraints
ALTER TABLE notifications
    ADD CONSTRAINT fk_notifications_profile_id
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE music_tracks
    ADD CONSTRAINT fk_music_tracks_library_id
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE;

ALTER TABLE photos
    ADD CONSTRAINT fk_photos_library_id
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE;

ALTER TABLE syncplay_rooms
    ADD CONSTRAINT fk_syncplay_rooms_host_id
    FOREIGN KEY (host_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE syncplay_rooms
    ADD CONSTRAINT fk_syncplay_rooms_media_id
    FOREIGN KEY (media_id) REFERENCES media_items(id) ON DELETE CASCADE;

-- Partial index for active play sessions
CREATE INDEX IF NOT EXISTS idx_play_sessions_active ON play_sessions(profile_id) WHERE ended_at IS NULL;
