-- media_streams.frame_rate_algo_version: which classifier produced the row's
-- frame_rate_mode. 0 means "an unversioned one" - every row written before
-- this column existed, including the false 'vfr' verdicts of the pre-mkvgo
-- v0.25.1 classifier, whose variance test counted a B-frame decode-order
-- artifact as a frame-duration outlier and mislabelled CFR sources.
--
-- Existing rows do NOT re-classify on their own: frame_rate_mode is only ever
-- written alongside a keyframe index, and those paths skip already-indexed
-- files. This column is what makes a stale verdict findable, so bumping the
-- analyzer's version constant re-analyzes the affected rows with no DB
-- surgery and no admin action (same mechanism as intro_detection_attempts's
-- algo_version, migration 000122).
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS frame_rate_algo_version SMALLINT NOT NULL DEFAULT 0;

-- Partial index: the re-classification drain asks only for rows that carry a
-- verdict from an older classifier, which is a shrinking minority of the
-- table (unclassified rows keep frame_rate_mode = '' and are served by the
-- keyframe backfill instead).
CREATE INDEX IF NOT EXISTS idx_media_streams_frame_rate_stale
    ON media_streams (frame_rate_algo_version)
    WHERE stream_type = 'video' AND frame_rate_mode <> '';
