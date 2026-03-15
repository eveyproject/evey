CREATE TABLE IF NOT EXISTS syncplay_rooms (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    host_id TEXT NOT NULL,
    media_id TEXT NOT NULL,
    members JSONB NOT NULL DEFAULT '[]',
    state JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
