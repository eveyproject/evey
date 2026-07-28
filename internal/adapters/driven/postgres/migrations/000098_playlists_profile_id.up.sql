-- Playlists move from user-scoped to profile-scoped ownership: each profile of
-- an account keeps its own playlists (like favorites/watch history), instead of
-- all profiles of a user sharing one set.

ALTER TABLE playlists ADD COLUMN profile_id TEXT;

-- A prior rollback stashed the real per-profile ownership: restore it first,
-- so a down/up round trip is lossless. Fresh installs skip this (no table).
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = '_mig000098_playlist_profiles') THEN
        UPDATE playlists pl
        SET profile_id = b.profile_id
        FROM _mig000098_playlist_profiles b
        WHERE pl.id = b.id
          AND EXISTS (SELECT FROM profiles p WHERE p.id = b.profile_id);
        DROP TABLE _mig000098_playlist_profiles;
    END IF;
END $$;

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
