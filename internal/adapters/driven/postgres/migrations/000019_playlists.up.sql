CREATE TABLE IF NOT EXISTS playlists (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL DEFAULT 'manual',
    rules       JSONB NOT NULL DEFAULT '[]',
    sort_by     TEXT NOT NULL DEFAULT 'sort_order',
    sort_order  TEXT NOT NULL DEFAULT 'asc',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_playlists_user_id ON playlists(user_id);

CREATE TABLE IF NOT EXISTS playlist_items (
    playlist_id   TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    sort_order    INT NOT NULL DEFAULT 0,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (playlist_id, media_item_id)
);
CREATE INDEX IF NOT EXISTS idx_playlist_items_playlist_id ON playlist_items(playlist_id);
