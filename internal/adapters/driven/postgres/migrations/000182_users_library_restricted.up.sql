-- Library restriction is an explicit flag: "no grant rows" used to mean "sees
-- every library", so a grant removed by a library delete (ON DELETE CASCADE)
-- or a last revoke widened a restricted user to everything.
ALTER TABLE users ADD COLUMN IF NOT EXISTS library_restricted BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE users SET library_restricted = TRUE
 WHERE id IN (SELECT DISTINCT user_id FROM library_access);
