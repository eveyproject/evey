CREATE TABLE IF NOT EXISTS library_access (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, library_id)
);
CREATE INDEX IF NOT EXISTS idx_library_access_user_id ON library_access(user_id);
