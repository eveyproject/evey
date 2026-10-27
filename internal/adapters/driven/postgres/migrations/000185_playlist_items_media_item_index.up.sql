-- The orphan purge and the per-media lookups filter playlist_items on
-- media_item_id, only the second column of the (playlist_id, media_item_id)
-- primary key.
CREATE INDEX IF NOT EXISTS idx_playlist_items_media_item_id ON playlist_items (media_item_id);
