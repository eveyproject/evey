-- media_files.bitrate has been 0 on every row since the column existed: the
-- probers only ever report per-TRACK bitrates (a Matroska BPS tag that ~80% of
-- a real library lacks), never a container-level one, so nothing wrote it.
--
-- Backfilled rather than re-scanned because the value is derivable from columns
-- already stored: ffprobe's format-level bit_rate IS total bytes over total
-- time, which is exactly what this computes - the same formula the playback
-- handler already trusted as its fallback. A re-scan would cost a full library
-- walk to arrive at identical numbers.
--
-- Only rows that have both inputs and no value yet are touched, so this is
-- idempotent and never overwrites a real measurement.
UPDATE media_files
   SET bitrate = (file_size * 8) / (duration_ms / 1000)
 WHERE bitrate = 0
   AND file_size > 0
   AND duration_ms >= 1000;
