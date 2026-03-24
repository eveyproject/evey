CREATE TABLE share_links (
    id TEXT PRIMARY KEY,
    token TEXT UNIQUE NOT NULL,
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    created_by TEXT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    max_views INT,
    view_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_share_links_token ON share_links(token);
