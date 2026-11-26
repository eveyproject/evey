-- A session the stale sweep ended was not stopped by anyone: its player only
-- went quiet (a laptop asleep on a paused film). Its next tick reopens it.
ALTER TABLE play_sessions ADD COLUMN ended_by_sweep BOOLEAN NOT NULL DEFAULT false;
