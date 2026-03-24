CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_watch_progress_profile_completed_last_played
    ON watch_progress (profile_id, completed, last_played_at DESC);
