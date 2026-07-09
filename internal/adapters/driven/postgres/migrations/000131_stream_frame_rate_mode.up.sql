-- media_streams.frame_rate_mode: the video stream's cfr/vfr classification
-- ("cfr", "vfr", or "" for unknown/never analyzed), from mkvgo's structural
-- ops.Analyze (v0.17.0). Defaults to '' (NOT NULL, unlike keyframes_ms's
-- NULL-means-unindexed) so existing rows read back as "unknown" without a
-- migration backfill. A vfr source gates the copy/remux rung off
-- (CopyRungEligible): the HLS copy segmenter cuts segments at keyframe
-- boundaries assuming a constant frame period, and a variable rate drifts
-- audio/video sync.
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS frame_rate_mode TEXT NOT NULL DEFAULT '';
