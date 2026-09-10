-- idet_telecine: the idet content detection saw repeated fields on a source it
-- also judged interlaced = 3:2 pulldown. TRUE switches the software builder to
-- fieldmatch/decimate (inverse telecine) instead of bwdif. NULL = not analyzed.
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS idet_telecine BOOLEAN;
