ALTER TABLE play_sessions
  DROP COLUMN IF EXISTS video_engine,
  DROP COLUMN IF EXISTS video_backend,
  DROP COLUMN IF EXISTS audio_engine;
