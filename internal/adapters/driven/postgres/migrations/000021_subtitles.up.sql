CREATE TABLE IF NOT EXISTS downloaded_subtitles (
    id             TEXT PRIMARY KEY,
    media_item_id  TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    language       TEXT NOT NULL,
    format         TEXT NOT NULL DEFAULT 'srt',
    file_path      TEXT NOT NULL,
    source         TEXT NOT NULL DEFAULT 'opensubtitles',
    downloaded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_downloaded_subtitles_media_item_id ON downloaded_subtitles(media_item_id);
