ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS related_media_id TEXT,
    ADD COLUMN IF NOT EXISTS episode_count    INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_notifications_related_media_id
    ON notifications (profile_id, type, related_media_id, created_at)
    WHERE related_media_id IS NOT NULL;
