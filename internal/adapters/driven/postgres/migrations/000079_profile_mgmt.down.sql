DROP INDEX IF EXISTS idx_profiles_user_is_kids;
ALTER TABLE profiles DROP COLUMN IF EXISTS is_kids;
ALTER TABLE users DROP COLUMN IF EXISTS max_devices_per_profile;
ALTER TABLE users DROP COLUMN IF EXISTS max_profiles;
