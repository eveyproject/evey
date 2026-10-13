-- A file replaced at the same path keeps its media item, and two artifacts
-- derived from the old bytes were still served: the trickplay sprites (keyed
-- by item and algorithm version) and the detected intro/credits (keyed by
-- item and type). Each now names the file it was made from; a row of another
-- file is neither served nor counted as done, so the sweeps remake it.
--
-- Backfill with the file each row points at today: they were served as that
-- file's until now, and stamping them otherwise would regenerate the whole
-- library at once. Operator marks (manual, manual_propagated) describe no file
-- and stay NULL, which always matches.
ALTER TABLE trickplay_manifests ADD COLUMN file_size BIGINT;
ALTER TABLE trickplay_manifests ADD COLUMN mod_time TIMESTAMPTZ;
UPDATE trickplay_manifests tm
   SET file_size = mf.file_size, mod_time = mf.mod_time
  FROM media_files mf
 WHERE mf.media_item_id = tm.media_item_id;

ALTER TABLE media_segments ADD COLUMN source_fingerprint TEXT;
UPDATE media_segments ms
   SET source_fingerprint = mf.file_fingerprint
  FROM media_files mf
 WHERE mf.media_item_id = ms.media_item_id
   AND ms.source NOT IN ('manual', 'manual_propagated');
