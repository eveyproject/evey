-- pixel_format is ffprobe's pix_fmt (yuv420p, yuv420p10le, yuv444p…). The
-- prober already read it and threw it away before the domain, so the proto
-- field and both StreamInfoPanel rows that display it were dead on arrival.
--
-- Nullable rather than NOT NULL DEFAULT '': an existing row has never been
-- probed for it, and "we never asked" is not the same as "the stream has no
-- pixel format". A rescan fills it; until then the panel simply omits the row.
ALTER TABLE media_streams
    ADD COLUMN IF NOT EXISTS pixel_format TEXT;
