-- Missing indexes identified in code quality audit 2026-03-13
CREATE INDEX IF NOT EXISTS idx_watch_progress_completed ON watch_progress(completed);
CREATE INDEX IF NOT EXISTS idx_media_people_media_item_id ON media_people(media_item_id);
CREATE INDEX IF NOT EXISTS idx_media_people_person_id ON media_people(person_id);
CREATE INDEX IF NOT EXISTS idx_play_sessions_media_item_id ON play_sessions(media_item_id);
CREATE INDEX IF NOT EXISTS idx_collection_items_media_item_id ON collection_items(media_item_id);
