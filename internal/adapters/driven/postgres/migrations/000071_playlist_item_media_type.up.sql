ALTER TABLE playlist_items ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'video';
ALTER TABLE playlist_items DROP CONSTRAINT IF EXISTS playlist_items_media_item_id_fkey;
