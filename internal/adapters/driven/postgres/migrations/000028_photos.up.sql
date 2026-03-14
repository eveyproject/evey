CREATE TABLE IF NOT EXISTS photos (
    id            TEXT PRIMARY KEY,
    library_id    TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    file_path     TEXT NOT NULL UNIQUE,
    file_size     BIGINT NOT NULL DEFAULT 0,
    mime_type     TEXT NOT NULL DEFAULT '',
    width         INTEGER NOT NULL DEFAULT 0,
    height        INTEGER NOT NULL DEFAULT 0,
    taken_at      TIMESTAMPTZ,
    latitude      DOUBLE PRECISION,
    longitude     DOUBLE PRECISION,
    camera_model  TEXT NOT NULL DEFAULT '',
    album         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_photos_library_id ON photos(library_id);
CREATE INDEX IF NOT EXISTS idx_photos_album ON photos(library_id, album);
