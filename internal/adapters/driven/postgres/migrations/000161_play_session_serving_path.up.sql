-- What the server actually serves for the session (recorded at master
-- render): packager/encoder per track and the HW backend. Empty = unknown
-- (session predates the column or direct play).
ALTER TABLE play_sessions
  ADD COLUMN video_engine TEXT NOT NULL DEFAULT '',
  ADD COLUMN video_backend TEXT NOT NULL DEFAULT '',
  ADD COLUMN audio_engine TEXT NOT NULL DEFAULT '';
