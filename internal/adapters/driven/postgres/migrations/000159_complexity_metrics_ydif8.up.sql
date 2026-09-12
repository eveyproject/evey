-- Migrate complexity_metrics.motion_mean from the native-bit-depth signalstats
-- YDIF to the canonical 8-bit YDIF8, and stamp metric_version=2. A 10-bit HDR
-- title had a motion_mean ~4x an 8-bit one of the same motion (the defect found
-- 2026-09-12), which made the cost model over-price HDR. YDIF8 = native /
-- 2^(bit_depth-8).
--
-- Deterministic where the video bit depth is known: divide and stamp v2.
-- Where it is not, the conversion is not deterministic, so the class is NULLed
-- and the row re-enters the complexity worker's queue to be resampled with the
-- new (yuv420p) definition. Either way no native YDIF is left readable as v2.

-- 1. Known depth: convert in place.
WITH depth AS (
    SELECT mi.id, MAX(ms.bit_depth) AS bd
    FROM media_items mi
    JOIN media_streams ms ON ms.media_item_id = mi.id AND ms.stream_type = 'video'
    WHERE mi.complexity_metrics IS NOT NULL
      AND COALESCE((mi.complexity_metrics->>'metric_version')::int, 1) < 2
      AND ms.bit_depth IS NOT NULL
    GROUP BY mi.id
)
UPDATE media_items mi
SET complexity_metrics = jsonb_set(
        jsonb_set(
            mi.complexity_metrics,
            '{motion_mean}',
            to_jsonb(
                COALESCE((mi.complexity_metrics->>'motion_mean')::float, 0)
                / power(2::float, GREATEST(d.bd, 8) - 8)
            )
        ),
        '{metric_version}', '2'::jsonb
    )
FROM depth d
WHERE mi.id = d.id;

-- 2. Unknown depth: cannot convert -> invalidate the class for a resample,
--    and stamp v2 so the row is not migrated again.
UPDATE media_items mi
SET complexity_class = NULL,
    complexity_metrics = jsonb_set(mi.complexity_metrics, '{metric_version}', '2'::jsonb)
WHERE mi.complexity_metrics IS NOT NULL
  AND COALESCE((mi.complexity_metrics->>'metric_version')::int, 1) < 2;
