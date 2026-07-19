DROP INDEX IF EXISTS idx_media_streams_frame_rate_algo_version;

CREATE INDEX IF NOT EXISTS idx_media_streams_frame_rate_stale
    ON media_streams (frame_rate_algo_version)
    WHERE stream_type = 'video' AND frame_rate_mode <> '';
