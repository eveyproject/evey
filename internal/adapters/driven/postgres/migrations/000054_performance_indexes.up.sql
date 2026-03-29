-- compound index for episode/season listings (WHERE library_id = ? AND parent_id = ?)
CREATE INDEX IF NOT EXISTS idx_media_items_library_parent
    ON media_items (library_id, parent_id);
