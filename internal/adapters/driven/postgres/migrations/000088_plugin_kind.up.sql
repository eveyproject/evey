ALTER TABLE plugins ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'plugin';
UPDATE plugins SET kind = 'theme' WHERE language = 'css';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS frontend JSONB;
