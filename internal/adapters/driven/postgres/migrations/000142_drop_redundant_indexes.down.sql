-- Recreate the indexes dropped by the up migration, with the exact definitions
-- they had in the migrations that first created them (000001, 000018, 000023,
-- 000055, 000068, 000087, 000013). Redundant, but a down migration restores the
-- previous state rather than an improved one.

CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_id ON watch_progress (profile_id);
CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_media ON watch_progress (profile_id, media_item_id);
CREATE INDEX IF NOT EXISTS idx_watch_progress_profile_media_completed ON watch_progress (profile_id, media_item_id, completed);
CREATE INDEX IF NOT EXISTS idx_watch_progress_completed ON watch_progress (completed);

CREATE INDEX IF NOT EXISTS idx_trickplay_manifests_media_item_id ON trickplay_manifests (media_item_id);

CREATE INDEX IF NOT EXISTS idx_media_segments_media_item_id ON media_segments (media_item_id);

CREATE INDEX IF NOT EXISTS idx_favorites_profile_id ON favorites (profile_id);
