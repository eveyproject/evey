ALTER TABLE profiles ADD COLUMN IF NOT EXISTS parental_max_age INT;

-- Backfill from the legacy string MaxRating for existing profiles.
UPDATE profiles SET parental_max_age = CASE UPPER(TRIM(parental_max_rating))
    WHEN 'G' THEN 0 WHEN 'TV-Y' THEN 0 WHEN 'TV-G' THEN 0
    WHEN 'TV-Y7' THEN 7
    WHEN 'PG' THEN 10 WHEN 'TV-PG' THEN 10
    WHEN 'PG-13' THEN 13 WHEN 'TV-14' THEN 14
    WHEN 'R' THEN 17 WHEN 'TV-MA' THEN 17
    WHEN 'NC-17' THEN 17
    ELSE NULL
END
WHERE parental_max_rating IS NOT NULL AND parental_max_rating <> '' AND parental_max_age IS NULL;

ALTER TABLE profiles DROP COLUMN IF EXISTS parental_max_rating;
