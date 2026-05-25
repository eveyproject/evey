-- Recreate the FK as NOT VALID so the rollback doesn't fail when audio rooms
-- (media_id referencing music_tracks, absent from media_items) already exist;
-- new rows are still checked.
ALTER TABLE syncplay_rooms
    ADD CONSTRAINT fk_syncplay_rooms_media_id
    FOREIGN KEY (media_id) REFERENCES media_items(id) ON DELETE CASCADE NOT VALID;
