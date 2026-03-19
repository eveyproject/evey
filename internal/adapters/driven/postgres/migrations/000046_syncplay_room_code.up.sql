ALTER TABLE syncplay_rooms ADD COLUMN IF NOT EXISTS code TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_syncplay_rooms_code ON syncplay_rooms (code) WHERE code != '';
