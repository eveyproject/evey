-- display_name is the name a plugin shows people, keyed by language tag
-- ({"en": "Dark Blue", "fr": "Bleu sombre"}). name stays the identifier an
-- update is matched on. An existing row reads {} and clients fall back on
-- name until the plugin is updated with a manifest that declares one.
ALTER TABLE plugins
    ADD COLUMN IF NOT EXISTS display_name JSONB NOT NULL DEFAULT '{}'::jsonb;
