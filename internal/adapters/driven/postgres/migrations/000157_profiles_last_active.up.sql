-- When a profile was last used: stamped on login (default profile) and on every
-- token refresh (silent or profile switch). NULL = never used since this column
-- exists. Deliberately NOT stamped by admin impersonation, which would name a
-- profile that nobody actually watched under.
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ;
