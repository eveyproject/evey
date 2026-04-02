CREATE INDEX idx_media_items_file_fingerprint ON media_items(file_fingerprint);

CREATE INDEX idx_media_items_episode_nextup ON media_items(parent_id, index_number) WHERE type = 'episode' AND index_number IS NOT NULL;
