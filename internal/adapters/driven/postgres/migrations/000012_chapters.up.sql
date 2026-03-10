CREATE TABLE IF NOT EXISTS chapters (
    id            TEXT PRIMARY KEY,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '',
    start_time_ms BIGINT NOT NULL DEFAULT 0,
    end_time_ms   BIGINT NOT NULL DEFAULT 0,
    sort_order    INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_chapters_media_item_id ON chapters(media_item_id);
