DROP INDEX IF EXISTS idx_syncplay_rooms_code;
ALTER TABLE syncplay_rooms DROP COLUMN IF EXISTS code;
