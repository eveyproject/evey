-- Media requests: users ask the operator to add a movie/series/album; the
-- operator moderates (approve/reject/later) with a note and the requester is
-- notified. See ADR-0008 for the domain constructor pattern.

CREATE TABLE IF NOT EXISTS media_requests (
    id          TEXT PRIMARY KEY,
    profile_id  TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_type  TEXT NOT NULL,               -- movie | tv | music
    provider    TEXT NOT NULL DEFAULT '',    -- tmdb | imdb | musicbrainz | '' (free text)
    provider_id TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL,
    year        INTEGER,
    poster_url  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending', -- pending | approved | rejected | later
    admin_note  TEXT NOT NULL DEFAULT '',
    answered_by TEXT REFERENCES profiles(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at TIMESTAMPTZ
);

-- Requester view: their own requests, newest first.
CREATE INDEX IF NOT EXISTS idx_media_requests_profile ON media_requests(profile_id, created_at DESC);
-- Operator moderation view: filter by status, newest first.
CREATE INDEX IF NOT EXISTS idx_media_requests_status ON media_requests(status, created_at DESC);
-- Duplicate guard: fast lookup of an existing open request for a title.
CREATE INDEX IF NOT EXISTS idx_media_requests_dupe ON media_requests(profile_id, media_type, provider_id);

-- Per-user access override. Absence of a row means "fall back to the global
-- default" (server_config key mediarequest.enabled_default).
CREATE TABLE IF NOT EXISTS media_request_access (
    user_id    TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled    BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Global default: requests enabled for everyone unless an override says otherwise.
INSERT INTO server_config (key, value, updated_at)
VALUES ('mediarequest.enabled_default', 'true', NOW())
ON CONFLICT (key) DO NOTHING;
