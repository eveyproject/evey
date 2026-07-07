ALTER TABLE play_sessions
  DROP COLUMN IF EXISTS video_decision,
  DROP COLUMN IF EXISTS audio_decision;
