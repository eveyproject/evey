-- Per-profile play counts for music tracks. Music tracks live in music_tracks
-- (not media_items), so favorites/ratings/watch_progress - all FK'd to
-- media_items - can't capture music taste. This table is the music taste
-- signal used by the playlist generator.
CREATE TABLE IF NOT EXISTS music_track_plays (
    profile_id     TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    track_id       TEXT NOT NULL REFERENCES music_tracks(id) ON DELETE CASCADE,
    play_count     INTEGER NOT NULL DEFAULT 0,
    last_played_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (profile_id, track_id)
);

CREATE INDEX IF NOT EXISTS idx_music_track_plays_profile ON music_track_plays(profile_id);
