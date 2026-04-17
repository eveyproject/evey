ALTER TABLE profiles ADD COLUMN IF NOT EXISTS parental_max_rating TEXT;
ALTER TABLE profiles DROP COLUMN IF EXISTS parental_max_age;
