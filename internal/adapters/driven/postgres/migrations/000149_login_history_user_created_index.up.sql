-- login_history is read as "this user's logins, most recent first" by both the
-- offset page (GetByUserID) and the keyset page (ListByUserIDCursor), but the
-- only indexes on it were single-column: one on user_id, one on created_at.
-- Neither covers the ordering, so Postgres filtered by user then sorted the
-- matching rows in memory - and this table has no retention job, so it only
-- grows. Same shape as idx_favorites_profile_created (migration 000147).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_login_history_user_created
    ON login_history (user_id, created_at DESC, id DESC);
