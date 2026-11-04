ALTER TABLE artwork
    DROP COLUMN IF EXISTS fetch_attempts,
    DROP COLUMN IF EXISTS next_fetch_at,
    DROP COLUMN IF EXISTS last_fetch_error;
