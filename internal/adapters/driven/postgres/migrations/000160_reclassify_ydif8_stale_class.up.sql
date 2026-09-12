-- Migration 159 converted motion_mean to the canonical YDIF8 in place on rows
-- whose video bit depth was known, but left complexity_class untouched. For a
-- source >= 2160p that is harmless: Classify() decides a 4K title on resolution
-- and bitrate, not on motion. For a sub-2160p title in >8-bit (a 1080p HDR
-- release), the class was computed on the old native YDIF - up to 4x too high -
-- so a title now correctly measured as low-motion could keep a high-motion
-- class. Those rows never re-enter the classifier queue (their class is not
-- NULL), so the stale class would stand forever.
--
-- NULL the class for exactly those rows so the complexity worker reclassifies
-- them from the canonical metrics (a fresh sample + a fresh class in one pass),
-- keeping Classify() the single source of truth rather than re-deriving the
-- decision table in SQL. The set is small (only sub-2160p, known >8-bit) and
-- the metric_version stamp stays at 2, so the cost model never reads a native
-- value in the meantime - the row simply falls back to the class table until
-- the worker gets to it.
UPDATE media_items mi
SET complexity_class = NULL
WHERE mi.type IN ('movie', 'episode')
  AND mi.complexity_class IS NOT NULL
  AND COALESCE((mi.complexity_metrics->>'metric_version')::int, 1) >= 2
  AND COALESCE((mi.complexity_metrics->>'source_height')::int, 0) < 2160
  AND EXISTS (
        SELECT 1 FROM media_streams ms
        WHERE ms.media_item_id = mi.id
          AND ms.stream_type = 'video'
          AND ms.bit_depth IS NOT NULL
          AND ms.bit_depth > 8
      );
