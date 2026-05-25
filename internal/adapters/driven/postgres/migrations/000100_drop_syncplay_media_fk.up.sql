-- syncplay_rooms.media_id is polymorphic: it references media_items for video
-- but music_tracks for audio, disambiguated by media_type. The rigid FK to
-- media_items rejected audio rooms (a music track is not a media_item), which
-- surfaced as an internal error when starting a track in sync-play.
-- Existence and access are already validated per media_type in the syncplay
-- handler (requireMediaAccess -> GetTrack for music, GetMediaItem otherwise),
-- so the FK is redundant for video and wrong for audio. Drop it.
ALTER TABLE syncplay_rooms DROP CONSTRAINT IF EXISTS fk_syncplay_rooms_media_id;
