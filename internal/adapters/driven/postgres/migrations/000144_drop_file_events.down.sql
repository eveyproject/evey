-- Recreate exactly as 000001 defined it (the table was always empty in
-- practice, so structure-only restoration is lossless).
CREATE TABLE IF NOT EXISTS file_events (
    id           TEXT PRIMARY KEY,
    library_id   TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    processed    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_file_events_library_id ON file_events(library_id);
CREATE INDEX IF NOT EXISTS idx_file_events_processed ON file_events(processed);
