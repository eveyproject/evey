ALTER TABLE play_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
UPDATE play_sessions SET updated_at = started_at WHERE updated_at = NOW();
