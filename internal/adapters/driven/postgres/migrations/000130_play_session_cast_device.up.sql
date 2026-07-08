-- Cast relay: when a web client hands playback to a Chromecast, the session's
-- progress reports describe the REMOTE playback. Record which device actually
-- plays so the admin Now Playing panel shows the real target instead of the
-- (paused) browser that started the session. Empty = local playback.
ALTER TABLE play_sessions ADD COLUMN cast_device_name TEXT NOT NULL DEFAULT '';
