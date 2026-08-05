ALTER TABLE notifications
    DROP COLUMN IF EXISTS subject,
    DROP COLUMN IF EXISTS season_count;
