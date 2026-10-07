-- Prober-version-driven re-probe: a mkvgo bump that adds fields (VP9
-- profile/level/colour, AAC profile, DocType) refreshed no existing row, as the
-- scanner only re-probes streamless files. stream_probe_version records the
-- library.StreamProbeVersion a row's streams were last probed at (0 = before
-- versioning); the scan re-probes rows below the current version head-only.
ALTER TABLE media_files
    ADD COLUMN IF NOT EXISTS stream_probe_version INTEGER NOT NULL DEFAULT 0;
