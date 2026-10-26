-- A metadata plugin overlays, field by field, the text TMDB provided: TMDB
-- stays the item's identity (ids, translations, collections) and a plugin's
-- text is kept beside it, per plugin and per locale, so a TMDB refresh never
-- overwrites it and uninstalling the plugin takes it away.
CREATE TABLE IF NOT EXISTS media_item_plugin_texts (
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    plugin_id     TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    locale        TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    overview      TEXT NOT NULL DEFAULT '',
    tagline       TEXT NOT NULL DEFAULT '',
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (media_item_id, plugin_id, locale)
);
CREATE INDEX IF NOT EXISTS idx_media_item_plugin_texts_plugin ON media_item_plugin_texts (plugin_id);

-- The fields the administrator lets each plugin replace. No row: the default
-- (overview and tagline, never the title until the administrator says so).
CREATE TABLE IF NOT EXISTS plugin_metadata_settings (
    plugin_id TEXT PRIMARY KEY REFERENCES plugins(id) ON DELETE CASCADE,
    fields    TEXT[] NOT NULL
);
