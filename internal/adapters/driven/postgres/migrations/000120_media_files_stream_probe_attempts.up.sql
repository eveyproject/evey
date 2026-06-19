-- Bounded re-probe tracking. An item indexed while its ffmpeg stream probe
-- failed (worker-budget timeout on a large 4K file, NAS I/O hiccup) persists
-- with zero media_streams and duration 0. The next scan sees the file
-- unchanged (same size + fingerprint) and skips re-probing, so the broken row
-- never self-heals. stream_probe_attempts counts probe attempts that yielded no
-- streams; the scan service re-probes while it is in (0, max) and then gives up,
-- so a durably-unprobeable file is not re-probed on every scan.
ALTER TABLE media_files
    ADD COLUMN stream_probe_attempts INTEGER NOT NULL DEFAULT 0;

-- Backfill: any already-indexed file with no media_streams row was indexed on a
-- failed probe. Mark it (attempts = 1) so it gets a bounded re-probe on the next
-- scan instead of staying streamless forever.
UPDATE media_files mf
SET stream_probe_attempts = 1
WHERE NOT EXISTS (
    SELECT 1 FROM media_streams ms WHERE ms.media_item_id = mf.media_item_id
);
