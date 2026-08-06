ALTER TABLE media_segments DROP CONSTRAINT IF EXISTS media_segments_range_check;
ALTER TABLE trickplay_manifests DROP COLUMN IF EXISTS algo_version;
