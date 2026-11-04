-- source_track_id is the number the file gives the track (Matroska
-- TrackNumber, MP4 tkhd track_ID). stream_index is only a position among the
-- surfaced tracks, so a packager that leaves one track out shifts every
-- position after it; the file's number names the same track on both sides.
--
-- Nullable: an existing row has never been probed for it, and "never asked"
-- is not a track number. A probe that reads it fills it; until then the
-- rendition pairing falls back on the name and the language.
ALTER TABLE media_streams
    ADD COLUMN IF NOT EXISTS source_track_id INTEGER;
