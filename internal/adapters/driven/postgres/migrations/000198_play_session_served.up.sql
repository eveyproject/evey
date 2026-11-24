-- What a play session shows is what was observed: served = the latest bytes
-- the server sent per leg (engine, copy or encode, source track, codec read
-- from the init that went out), played = what the player reports it decodes.
-- They replace the per-track decisions and engines, which were written by five
-- different paths (the client's echo of its verdict, the master render, the
-- audio playlist, the wasm beacon) and disagreed with one another.
ALTER TABLE play_sessions
  ADD COLUMN served JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN played JSONB,
  DROP COLUMN IF EXISTS video_decision,
  DROP COLUMN IF EXISTS audio_decision,
  DROP COLUMN IF EXISTS video_engine,
  DROP COLUMN IF EXISTS video_backend,
  DROP COLUMN IF EXISTS audio_engine;
