-- Composite index for the per-profile recency scans. GetFavoriteGenres runs
-- `WHERE profile_id = $1 ORDER BY started_at DESC LIMIT 200` on every Recommended
-- feed load (home page), and GetUserStats filters/groups by profile_id too. The
-- existing indexes only cover profile_id OR started_at separately, so Postgres
-- index-scanned every session row for the profile then top-N heap-sorted them -
-- a full per-profile sort on each home load for heavy users. A leading-profile,
-- descending-started index lets it read the first 200 in order and stop.
CREATE INDEX IF NOT EXISTS idx_play_sessions_profile_started
    ON play_sessions (profile_id, started_at DESC);
