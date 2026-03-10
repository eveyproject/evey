ALTER TABLE profiles
    DROP COLUMN IF EXISTS parental_max_rating,
    DROP COLUMN IF EXISTS parental_blocked_tags,
    DROP COLUMN IF EXISTS parental_pin_hash,
    DROP COLUMN IF EXISTS parental_enabled;
