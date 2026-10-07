-- Removing a media item deletes its notifications by media_item_id (every
-- other notification index starts with profile_id); without this index that
-- delete is a sequential scan of the whole table per removed item.
CREATE INDEX IF NOT EXISTS idx_notifications_media_item_id
    ON notifications (media_item_id)
    WHERE media_item_id <> '';
