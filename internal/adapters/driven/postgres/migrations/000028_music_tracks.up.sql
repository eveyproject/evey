CREATE TABLE IF NOT EXISTS music_tracks (
    id             TEXT PRIMARY KEY,
    library_id     TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    artist         TEXT NOT NULL DEFAULT '',
    album          TEXT NOT NULL DEFAULT '',
    album_artist   TEXT NOT NULL DEFAULT '',
    year           INTEGER NOT NULL DEFAULT 0,
    track_number   INTEGER NOT NULL DEFAULT 0,
    disk_number    INTEGER NOT NULL DEFAULT 0,
    duration       FLOAT NOT NULL DEFAULT 0,
    file_path      TEXT NOT NULL UNIQUE,
    file_size      BIGINT NOT NULL DEFAULT 0,
    mime_type      TEXT NOT NULL DEFAULT '',
    bitrate        INTEGER NOT NULL DEFAULT 0,
    sample_rate    INTEGER NOT NULL DEFAULT 0,
    channels       INTEGER NOT NULL DEFAULT 0,
    genre          TEXT NOT NULL DEFAULT '',
    cover_art_url  TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_music_tracks_library_id ON music_tracks(library_id);
CREATE INDEX IF NOT EXISTS idx_music_tracks_artist ON music_tracks(artist);
CREATE INDEX IF NOT EXISTS idx_music_tracks_album ON music_tracks(album, album_artist);
