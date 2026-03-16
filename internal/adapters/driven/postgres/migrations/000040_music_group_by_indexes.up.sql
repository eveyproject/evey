-- Compound indexes for music GROUP BY queries (GetAlbums, GetArtists)
CREATE INDEX IF NOT EXISTS idx_music_tracks_library_album_artist
    ON music_tracks(library_id, album, album_artist, artist);
