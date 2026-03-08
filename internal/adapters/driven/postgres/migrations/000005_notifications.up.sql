CREATE TABLE IF NOT EXISTS notifications (
    id           TEXT PRIMARY KEY,
    profile_id   TEXT NOT NULL,
    type         TEXT NOT NULL,
    title        TEXT NOT NULL,
    message      TEXT NOT NULL DEFAULT '',
    media_item_id TEXT NOT NULL DEFAULT '',
    level        TEXT NOT NULL DEFAULT 'all',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_notifications_profile_id ON notifications(profile_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read_at ON notifications(profile_id, read_at);

CREATE TABLE IF NOT EXISTS notification_settings (
    profile_id   TEXT PRIMARY KEY,
    movie_level  TEXT NOT NULL DEFAULT 'all',
    series_level TEXT NOT NULL DEFAULT 'all',
    music_level  TEXT NOT NULL DEFAULT 'all'
);

CREATE TABLE IF NOT EXISTS notification_settings_global (
    id           INT PRIMARY KEY DEFAULT 1,
    movie_level  TEXT NOT NULL DEFAULT 'all',
    series_level TEXT NOT NULL DEFAULT 'all',
    music_level  TEXT NOT NULL DEFAULT 'all'
);
INSERT INTO notification_settings_global (id) VALUES (1) ON CONFLICT DO NOTHING;
