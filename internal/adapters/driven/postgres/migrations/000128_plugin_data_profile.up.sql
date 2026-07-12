-- Per-profile plugin data. Adds a nullable profile_id so a plugin's preferences
-- can be scoped to the ACTIVE profile (first-class per-profile plugin prefs).
--
-- A profile belongs to exactly one user, so a per-profile row is identified by
-- profile_id ALONE - user_id would be redundant and is left NULL. The three
-- scopes on this table are therefore mutually exclusive:
--   * user_id IS NULL AND profile_id IS NULL -> plugin-global (backend)
--   * user_id set,      profile_id IS NULL   -> per-user (shared across the
--                                               user's profiles)
--   * profile_id set                         -> per-profile (user_id NULL)
-- profile_id cascades on profile delete; deleting a user cascades its profiles,
-- which cascades these rows too - so nothing is orphaned.
ALTER TABLE plugin_data
    ADD COLUMN profile_id TEXT REFERENCES profiles(id) ON DELETE CASCADE;

-- Rework the partial unique indexes so the three scopes never collide (the old
-- global index matched any user_id IS NULL row, which would now also catch
-- per-profile rows). Each scope gets exactly one qualifying index.
DROP INDEX IF EXISTS uq_plugin_data_global;
DROP INDEX IF EXISTS uq_plugin_data_user;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_global
    ON plugin_data (plugin_id, key) WHERE user_id IS NULL AND profile_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_user
    ON plugin_data (plugin_id, user_id, key) WHERE user_id IS NOT NULL AND profile_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_profile
    ON plugin_data (plugin_id, profile_id, key) WHERE profile_id IS NOT NULL;

-- Hot path: wipe every row for a profile (profile delete cascade already does
-- this via FK, but the index keeps per-profile lookups cheap).
CREATE INDEX IF NOT EXISTS idx_plugin_data_profile ON plugin_data (plugin_id, profile_id) WHERE profile_id IS NOT NULL;
