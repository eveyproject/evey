-- The keyframe backfill worker's candidate query is "keyframes_ms IS NULL",
-- but a probe that fails or comes back empty (avi and other containers the
-- prober cannot index) persists NOTHING - so the same files re-entered the
-- candidate set at every boot and were re-walked forever (prod 2026-07-26:
-- 68 avi files re-probed on each restart).
--
-- keyframes_probe_version records the prober version that settled on "no
-- index possible" for this row (0 = never settled). The candidate query adds
-- keyframes_probe_version < <current version>, so stamped rows drop out until
-- either the prober version is bumped (it learned the container) or a rescan
-- of a replaced file recreates the stream rows. Same doctrine as
-- frame_rate_algo_version.
ALTER TABLE media_streams
    ADD COLUMN IF NOT EXISTS keyframes_probe_version SMALLINT NOT NULL DEFAULT 0;
