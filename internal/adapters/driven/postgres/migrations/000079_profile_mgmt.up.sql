-- Per-user hard caps (NULL = unlimited) + kids flag on profile.
ALTER TABLE users ADD COLUMN IF NOT EXISTS max_profiles INT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS max_devices_per_profile INT;

ALTER TABLE profiles ADD COLUMN IF NOT EXISTS is_kids BOOLEAN NOT NULL DEFAULT FALSE;

-- Kids flag filter used on every profile list; partial index keeps the cost
-- flat on the common case where the user has no kids profile.
CREATE INDEX IF NOT EXISTS idx_profiles_user_is_kids ON profiles(user_id) WHERE is_kids = TRUE;
