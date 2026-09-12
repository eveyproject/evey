-- Irreversible in the value sense: the native YDIF is not recoverable from the
-- canonical one without the bit depth, and the resampled rows are new
-- measurements. The down migration only drops the version stamp so a
-- re-up re-runs; the motion_mean stays canonical (a re-up on a v2 row is a
-- no-op division by 1 for 8-bit and would double-divide 10-bit, so guard by
-- version - which is exactly what the up migration's COALESCE(...) < 2 does).
UPDATE media_items
SET complexity_metrics = complexity_metrics - 'metric_version'
WHERE complexity_metrics ? 'metric_version';
