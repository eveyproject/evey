-- Bind a refresh token to the profile that was active when it was issued, so a
-- token refresh preserves the current profile instead of silently falling back
-- to the user's default profile (which let a kids profile escape parental
-- control on the next silent refresh). Nullable: legacy tokens (NULL) keep the
-- previous default-profile behaviour.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS profile_id TEXT REFERENCES profiles(id) ON DELETE CASCADE;
