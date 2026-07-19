-- Widen the re-classification candidate set from "carries a stale verdict" to
-- "carries no verdict from the CURRENT classifier", which also covers rows that
-- were never classified at all (frame_rate_mode = '').
--
-- Those rows are not harmless: '' fails OPEN at the copy gate
-- (CopyRungEligible only blocks a confirmed 'vfr'), so a genuinely
-- variable-rate source sitting at '' is served as a -c:v copy and drifts A/V -
-- the exact failure the gate exists to prevent. And they never self-heal: the
-- classification only ever rode along a keyframe write, so the 437 rows that
-- are already keyframe-indexed would stay unclassified forever.
--
-- The 000136 index filtered on frame_rate_mode <> '' and can no longer serve
-- that query, hence the swap rather than an addition.
DROP INDEX IF EXISTS idx_media_streams_frame_rate_stale;

CREATE INDEX IF NOT EXISTS idx_media_streams_frame_rate_algo_version
    ON media_streams (frame_rate_algo_version)
    WHERE stream_type = 'video';
