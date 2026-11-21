-- media_files.container_damaged_bytes: bytes of the container that do not
-- parse (see library.MediaFile.ContainerDamagedBytes); a file with any is never
-- handed to a device whole. container_check_version: the check revision the
-- row was measured at (0 = owed one). direct_play_refused: a device's demuxer
-- refused this file; cleared when the file changes.
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS container_damaged_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS container_check_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS direct_play_refused BOOLEAN NOT NULL DEFAULT false;
