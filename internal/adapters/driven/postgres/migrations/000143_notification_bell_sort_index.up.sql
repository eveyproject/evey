-- The notification bell polls ListByProfile, which is
--   WHERE profile_id = $1 [AND read_at IS NULL] ORDER BY created_at DESC, id DESC LIMIT 200
-- No existing index carries created_at in a position that serves the sort:
-- idx_notifications_profile_id is (profile_id) only, and idx_notifications_read_at
-- is (profile_id, read_at). So Postgres filters by profile_id then sorts every
-- matching row in memory before applying LIMIT. Notifications with a NULL
-- expires_at are never purged (DeleteExpired only removes dated ones), so a
-- busy profile accumulates thousands of rows and re-sorts them on every poll.
--
-- This index lets the planner walk rows already in (created_at DESC, id DESC)
-- order per profile and stop at LIMIT - no sort, no full-partition scan. The
-- unread-only variant still filters read_at on the fly against the same index.
--
-- Unlike the watch_progress indexes dropped in 000142 (that table takes one
-- write every 5s per viewer, the hottest write path), notifications are written
-- only on content events, so the per-insert maintenance cost is minor next to
-- the per-poll read saving.
CREATE INDEX IF NOT EXISTS idx_notifications_profile_created
    ON notifications (profile_id, created_at DESC, id DESC);
