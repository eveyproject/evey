-- Legacy-codec (DVD-era mpeg2/mpeg4) serving metadata, B3 chantier.
--
-- field_order: ffprobe field_order (progressive/tt/bb/tb/bt), NULL when the
-- probe didn't report it. A tt/bb/tb/bt flag means the decoder already tags
-- frames interlaced and bwdif deint=interlaced handles them.
--
-- idet_interlaced: verdict of the idet CONTENT detection for sources whose
-- flags are absent or lie (repacked DVD rips encode interlaced fields as
-- "progressive" frames → combing the flag-gated bwdif never fixes). NULL =
-- not analyzed; TRUE switches the builder to bwdif deint=all for this title.
--
-- sample_aspect_ratio: ffprobe sample_aspect_ratio ("64:45" on anamorphic
-- DVD 720x576 DAR 16:9). The transcode's historical `scale=W:-2,setsar=1`
-- preserved the STORAGE ratio then forced square pixels → distorted output
-- for any SAR != 1:1 source; the builder now computes the height from the
-- display aspect when this column says anamorphic.
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS field_order TEXT;
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS idet_interlaced BOOLEAN;
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS sample_aspect_ratio TEXT;
