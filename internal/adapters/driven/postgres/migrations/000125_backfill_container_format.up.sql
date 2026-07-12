-- Backfill media_files.container_format from the file extension.
--
-- The prober never populated this column, so every item carried
-- container_format='' and the playback decision flagged it "container_incompatible"
-- (an empty container matches no client list) - forcing a needless remux/transcode
-- on the whole library. That is what broke in-timeline seeking and shifted colors
-- on the Android player (ExoPlayer direct-plays MKV, but only when the container
-- is known and declared). New/re-probed items now get it from the prober; this
-- fills existing rows without a full re-probe.
--
-- Idempotent: only touches empty values, and the extension → container mapping
-- mirrors the prober's normalizeContainer (matroska/webm -> "matroska", the MP4
-- family -> "mp4", others by extension).
UPDATE media_files
SET container_format = CASE
    WHEN lower(file_path) LIKE '%.mkv'  THEN 'matroska'
    WHEN lower(file_path) LIKE '%.webm' THEN 'matroska'
    WHEN lower(file_path) LIKE '%.mp4'  THEN 'mp4'
    WHEN lower(file_path) LIKE '%.m4v'  THEN 'mp4'
    WHEN lower(file_path) LIKE '%.mov'  THEN 'mp4'
    WHEN lower(file_path) LIKE '%.avi'  THEN 'avi'
    WHEN lower(file_path) LIKE '%.ts'   THEN 'mpegts'
    WHEN lower(file_path) LIKE '%.m2ts' THEN 'mpegts'
    WHEN lower(file_path) LIKE '%.flv'  THEN 'flv'
    WHEN lower(file_path) LIKE '%.wmv'  THEN 'asf'
    WHEN lower(file_path) LIKE '%.mpg'  THEN 'mpeg'
    WHEN lower(file_path) LIKE '%.mpeg' THEN 'mpeg'
    ELSE container_format
END
WHERE (container_format IS NULL OR container_format = '')
  AND file_path IS NOT NULL
  AND file_path <> '';
