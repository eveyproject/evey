CREATE TABLE IF NOT EXISTS favorites (
    id            TEXT PRIMARY KEY,
    profile_id    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, media_item_id)
);
CREATE INDEX IF NOT EXISTS idx_favorites_profile_id ON favorites(profile_id);
