-- An invite link can cap the number of profiles the account it creates may
-- have, like an admin-created user (users.max_profiles). NULL = no cap.
ALTER TABLE invite_links ADD COLUMN IF NOT EXISTS max_profiles INTEGER;
