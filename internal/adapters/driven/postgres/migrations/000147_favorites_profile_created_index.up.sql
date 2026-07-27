CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_favorites_profile_created
    ON favorites (profile_id, created_at DESC, id DESC);
