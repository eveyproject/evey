-- Missing indexes
CREATE INDEX IF NOT EXISTS idx_user_ratings_profile_id
    ON user_ratings(profile_id);

CREATE INDEX IF NOT EXISTS idx_media_items_library_type
    ON media_items(library_id, type);

CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_media
    ON watch_progress(profile_id, media_item_id);

-- Missing foreign keys
ALTER TABLE user_ratings
    ADD CONSTRAINT fk_user_ratings_profile
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE user_ratings
    ADD CONSTRAINT fk_user_ratings_media_item
    FOREIGN KEY (media_item_id) REFERENCES media_items(id) ON DELETE CASCADE;

ALTER TABLE push_subscriptions
    ADD CONSTRAINT fk_push_subscriptions_profile
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;
