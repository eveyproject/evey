CREATE TABLE IF NOT EXISTS media_segments (
    id            TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    type          TEXT NOT NULL,
    start_ms      BIGINT NOT NULL DEFAULT 0,
    end_ms        BIGINT NOT NULL DEFAULT 0,
    confidence    REAL NOT NULL DEFAULT 0,
    source        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_item_id, type)
);
CREATE INDEX IF NOT EXISTS idx_media_segments_media_item_id ON media_segments(media_item_id);
