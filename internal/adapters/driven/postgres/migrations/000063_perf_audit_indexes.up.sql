-- artwork.person_id: used in GetByPerson WHERE person_id = $1
CREATE INDEX IF NOT EXISTS idx_artwork_person_id ON artwork(person_id) WHERE person_id IS NOT NULL;

-- notifications.expires_at: used in CleanupExpired WHERE expires_at < NOW()
CREATE INDEX IF NOT EXISTS idx_notifications_expires_at ON notifications(expires_at) WHERE expires_at IS NOT NULL;

-- play_sessions ended_at + updated_at: used in CloseStale WHERE ended_at IS NULL AND updated_at < $1
CREATE INDEX IF NOT EXISTS idx_play_sessions_stale ON play_sessions(updated_at) WHERE ended_at IS NULL;

-- share_links.created_by: used in ListByUser WHERE created_by = $1
CREATE INDEX IF NOT EXISTS idx_share_links_created_by ON share_links(created_by);

-- webhook_deliveries: already has idx on webhook_id, OK

-- downloaded_subtitles: already has idx on media_item_id, OK

-- media_items.year: used in WHERE year = $x filters from collection/playlist rules
CREATE INDEX IF NOT EXISTS idx_media_items_year ON media_items(year) WHERE year IS NOT NULL;
