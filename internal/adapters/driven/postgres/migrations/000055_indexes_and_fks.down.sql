ALTER TABLE push_subscriptions DROP CONSTRAINT IF EXISTS fk_push_subscriptions_profile;
ALTER TABLE user_ratings DROP CONSTRAINT IF EXISTS fk_user_ratings_media_item;
ALTER TABLE user_ratings DROP CONSTRAINT IF EXISTS fk_user_ratings_profile;

DROP INDEX IF EXISTS idx_watch_progress_profile_media;
DROP INDEX IF EXISTS idx_media_items_library_type;
DROP INDEX IF EXISTS idx_user_ratings_profile_id;
