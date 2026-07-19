DROP INDEX IF EXISTS idx_media_streams_frame_rate_stale;
ALTER TABLE media_streams DROP COLUMN IF EXISTS frame_rate_algo_version;
