DROP INDEX IF EXISTS idx_notifications_related_media_id;
ALTER TABLE notifications
    DROP COLUMN IF EXISTS related_media_id,
    DROP COLUMN IF EXISTS episode_count;
