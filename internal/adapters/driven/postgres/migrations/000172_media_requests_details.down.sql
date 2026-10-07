ALTER TABLE media_requests
    DROP COLUMN IF EXISTS overview,
    DROP COLUMN IF EXISTS release_date,
    DROP COLUMN IF EXISTS rating;
