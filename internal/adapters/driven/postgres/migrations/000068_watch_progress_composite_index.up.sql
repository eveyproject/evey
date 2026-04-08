CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_media_completed
    ON watch_progress(profile_id, media_item_id, completed);
