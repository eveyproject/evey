DROP INDEX IF EXISTS idx_media_items_latest_child_added_at;
ALTER TABLE media_items DROP COLUMN IF EXISTS latest_child_added_at;
