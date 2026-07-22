-- Drop indexes that duplicate an existing UNIQUE constraint, or that are a
-- strict left prefix of one. Postgres can serve a prefix lookup from the wider
-- index, so these earn nothing on reads while every INSERT and UPDATE has to
-- maintain them.
--
-- watch_progress is the reason this matters: it takes one UPSERT every 5s per
-- viewer (watch_progress_buffer), making it the hottest write path during
-- playback, and it carried 7 secondary indexes of which 4 were dead weight.
--
-- Each drop below is justified against the live schema, not guessed:
--
--   watch_progress  UNIQUE (profile_id, media_item_id)
--     - idx_watch_progress_profile_id ........... left prefix of the unique
--     - idx_watch_progress_profile_media ........ EXACT duplicate of the unique
--     - idx_watch_progress_profile_media_completed
--           the unique already resolves (profile_id, media_item_id) to at most
--           one row, so trailing `completed` cannot narrow anything further
--     - idx_watch_progress_completed ............ a lone boolean. No query
--           filters on completed without also pinning profile_id (checked
--           across feed_querier.go and watch_progress_repo.go), and those are
--           served by idx_watch_progress_profile_completed_last_played. A
--           two-value column is not selective enough for the planner to pick
--           it anyway.
--
--   trickplay_manifests  UNIQUE (media_item_id)
--     - idx_trickplay_manifests_media_item_id ... EXACT duplicate
--
--   media_segments  UNIQUE (media_item_id, type)
--     - idx_media_segments_media_item_id ........ left prefix of the unique
--
--   favorites  UNIQUE (profile_id, media_item_id)
--     - idx_favorites_profile_id ................ left prefix of the unique
--
-- Deliberately KEPT: idx_watch_progress_media_item_id and
-- idx_favorites_media_item_id. media_item_id leads no other index on those
-- tables, and both back an ON DELETE CASCADE foreign key - dropping them would
-- turn deleting a media item into a sequential scan per child table.
--
-- Indexes only, no data and no constraints: reads keep the same plans and the
-- down migration restores them exactly.

DROP INDEX IF EXISTS idx_watch_progress_profile_id;
DROP INDEX IF EXISTS idx_watch_progress_profile_media;
DROP INDEX IF EXISTS idx_watch_progress_profile_media_completed;
DROP INDEX IF EXISTS idx_watch_progress_completed;

DROP INDEX IF EXISTS idx_trickplay_manifests_media_item_id;

DROP INDEX IF EXISTS idx_media_segments_media_item_id;

DROP INDEX IF EXISTS idx_favorites_profile_id;
