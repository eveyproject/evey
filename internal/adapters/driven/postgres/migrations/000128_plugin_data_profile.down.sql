-- Reverse 000128: drop the per-profile scope and restore the original indexes.
DROP INDEX IF EXISTS idx_plugin_data_profile;
DROP INDEX IF EXISTS uq_plugin_data_profile;
DROP INDEX IF EXISTS uq_plugin_data_user;
DROP INDEX IF EXISTS uq_plugin_data_global;

-- Per-profile rows have no user_id, so they can't collapse into the per-user
-- scope; just remove them before dropping the column.
DELETE FROM plugin_data WHERE profile_id IS NOT NULL;
ALTER TABLE plugin_data DROP COLUMN IF EXISTS profile_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_global
    ON plugin_data (plugin_id, key) WHERE user_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_user
    ON plugin_data (plugin_id, user_id, key) WHERE user_id IS NOT NULL;
