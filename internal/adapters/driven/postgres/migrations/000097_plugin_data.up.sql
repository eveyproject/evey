-- plugin_data: a key/value store plugins use to persist their own data.
-- Two scopes share the table:
--   * user_id IS NULL  -> plugin-global data (one row per (plugin, key)).
--   * user_id set       -> per-user data, e.g. plugin-namespaced preferences.
-- Both foreign keys cascade on delete, so the store self-cleans:
--   * uninstalling a plugin (hard DELETE FROM plugins) wipes all its rows;
--   * deleting a user wipes that user's plugin data.
-- This keeps a plugin's persisted state contained to its own namespace and
-- guarantees no orphan rows survive removal.
CREATE TABLE IF NOT EXISTS plugin_data (
    plugin_id  TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    user_id    TEXT REFERENCES users(id) ON DELETE CASCADE, -- NULL = plugin-global
    key        TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '', -- opaque to the host; the plugin owns the encoding
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Uniqueness with a nullable user_id: a regular unique index treats NULLs as
-- distinct, so split global vs user-scoped into two partial unique indexes.
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_global
    ON plugin_data (plugin_id, key) WHERE user_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plugin_data_user
    ON plugin_data (plugin_id, user_id, key) WHERE user_id IS NOT NULL;

-- Hot path: look up / delete every row for a plugin or a (plugin, user).
CREATE INDEX IF NOT EXISTS idx_plugin_data_plugin_user ON plugin_data (plugin_id, user_id);
