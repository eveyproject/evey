-- Backfill kind for rows created before Phase 1 (kind column added in 000088).
-- CSS language → theme; everything else → plugin.
UPDATE plugins SET kind = 'theme' WHERE kind = '' AND language = 'css';
UPDATE plugins SET kind = 'plugin' WHERE kind = '';
-- Ensure no NULLs or empty strings remain.
ALTER TABLE plugins ALTER COLUMN kind SET NOT NULL;
