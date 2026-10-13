ALTER TABLE media_segments DROP COLUMN IF EXISTS source_fingerprint;
ALTER TABLE trickplay_manifests DROP COLUMN IF EXISTS mod_time;
ALTER TABLE trickplay_manifests DROP COLUMN IF EXISTS file_size;
