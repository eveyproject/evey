-- The per-profile "allow direct play / remux / transcoding" switches are gone
-- from every client and the server no longer reads them: turning a play
-- method off only ever broke playback. Older app builds still render the
-- switch from the stored value, so a profile that saved "false" would keep
-- showing a refusal that no longer applies. Drop the keys.
UPDATE profiles
SET preferences = preferences - 'allow_direct_play' - 'allow_direct_stream' - 'allow_transcoding'
WHERE preferences ?| ARRAY['allow_direct_play', 'allow_direct_stream', 'allow_transcoding'];
