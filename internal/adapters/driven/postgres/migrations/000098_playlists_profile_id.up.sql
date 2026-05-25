-- Playlists move from user-scoped to profile-scoped ownership: each profile of
-- an account keeps its own playlists (like favorites/watch history), instead of
-- all profiles of a user sharing one set.

ALTER TABLE playlists ADD COLUMN profile_id TEXT;

-- Backfill: each existing playlist goes to its user's default profile (or the
-- oldest profile if no default flag is set). Every user has at least one
-- profile, so this leaves no NULLs; if it somehow does, the NOT NULL below
-- fails loudly rather than silently dropping data.
UPDATE playlists pl
SET profile_id = (
    SELECT p.id
    FROM profiles p
    WHERE p.user_id = pl.user_id
    ORDER BY p.is_default DESC, p.created_at ASC
    LIMIT 1
)
WHERE pl.profile_id IS NULL;

ALTER TABLE playlists ALTER COLUMN profile_id SET NOT NULL;
ALTER TABLE playlists ADD CONSTRAINT fk_playlists_profile_id
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_playlists_profile_id ON playlists(profile_id);
DROP INDEX IF EXISTS idx_playlists_user_id;

ALTER TABLE playlists DROP COLUMN user_id;
