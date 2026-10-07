-- The level backfill's candidate query keys on level / color_transfer IS NULL,
-- but a stream whose probe succeeds and still reports neither (a VP9 stream
-- ffprobe never fills) persisted nothing, so it was re-probed at every boot
-- and the coverage counter stayed stuck one short.
--
-- level_probe_version records the library.LevelProbeVersion at which a probe
-- of this row's media item came back (0 = never probed). The candidate query
-- adds level_probe_version < <current version>, so a settled row drops out
-- until the version is bumped or a rescan recreates the stream rows.
ALTER TABLE media_streams
    ADD COLUMN IF NOT EXISTS level_probe_version INTEGER NOT NULL DEFAULT 0;
