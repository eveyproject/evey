CREATE TABLE IF NOT EXISTS trickplay_manifests (
    id            TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    interval_sec  INT NOT NULL DEFAULT 10,
    width         INT NOT NULL DEFAULT 320,
    height        INT NOT NULL DEFAULT 0,
    tile_width    INT NOT NULL DEFAULT 10,
    tile_height   INT NOT NULL DEFAULT 10,
    image_count   INT NOT NULL DEFAULT 0,
    image_path    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_item_id)
);
CREATE INDEX IF NOT EXISTS idx_trickplay_manifests_media_item_id ON trickplay_manifests(media_item_id);
