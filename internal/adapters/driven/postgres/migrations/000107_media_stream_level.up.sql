-- media_streams.level: codec level (e.g. H.264 level 4.0 = 40), NULL when the
-- prober did not report it. Needed with profile to build the exact RFC-6381
-- avc1.PPCCLL CODECS string for a video-copy rung. NULL gates copy off (no
-- accurate codec advertise) until a re-probe backfills it.
ALTER TABLE media_streams ADD COLUMN IF NOT EXISTS level INTEGER;
