-- Trickplay generation parameters were never versioned: when the sampling
-- changed (10s/10x10 grid at the original migration, 7s/5x5 today) every
-- pre-existing manifest silently kept the old density forever, because the
-- missing-sweep only targets items with NO manifest. 0 = legacy/unknown
-- parameters; the generator stamps its current version and the sweep
-- re-lists anything below it (throttled like any generation).
ALTER TABLE trickplay_manifests ADD COLUMN algo_version INT NOT NULL DEFAULT 0;

-- Inverted segment ranges (end <= start) used to be persisted verbatim - a
-- final "Générique" chapter with no recorded end stored end_ms=0. Purge the
-- class and forbid it at the schema. Chapters keep no such constraint on
-- purpose: end_time_ms=0 is their legitimate "unset, ends at next chapter"
-- convention.
DELETE FROM media_segments WHERE end_ms <= start_ms;
ALTER TABLE media_segments ADD CONSTRAINT media_segments_range_check CHECK (end_ms > start_ms);
