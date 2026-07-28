-- Revert playlists to user-scoped ownership, deriving the user from each
-- playlist's profile.

ALTER TABLE playlists ADD COLUMN user_id TEXT;

UPDATE playlists pl
SET user_id = (
    SELECT p.user_id
    FROM profiles p
    WHERE p.id = pl.profile_id
)
WHERE pl.user_id IS NULL;

ALTER TABLE playlists ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE playlists ADD CONSTRAINT playlists_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_playlists_user_id ON playlists(user_id);
DROP INDEX IF EXISTS idx_playlists_profile_id;

-- Stash the per-profile ownership: without it, a later re-up would reassign
-- EVERY playlist to its user's default profile (the backfill's only guess).
-- The up migration restores from this table when it exists, then drops it.
CREATE TABLE IF NOT EXISTS _mig000098_playlist_profiles AS
    SELECT id, profile_id FROM playlists;

ALTER TABLE playlists DROP CONSTRAINT IF EXISTS fk_playlists_profile_id;
ALTER TABLE playlists DROP COLUMN profile_id;
