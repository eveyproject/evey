-- media_streams.keyframes_ms: the video stream's keyframe presentation times in
-- milliseconds (sorted ascending), NULL when not yet indexed. These are the only
-- valid segment boundaries for a -c:v copy (remux) rung; a NULL gates the
-- byte-range/per-segment copy path off until a background backfill fills it.
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS keyframes_ms BIGINT[];
