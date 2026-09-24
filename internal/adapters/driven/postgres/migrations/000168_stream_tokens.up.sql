-- Stream tokens outlive a server restart.
--
-- They lived only in memory: every deploy forgot them, and the native players
-- (phone, Android television), whose one credential is the token fixed on
-- their source, were refused every segment until they gave up - a ~50 s cut
-- of every playback in progress (production, 2026-09-23 and 2026-09-24). The
-- token itself is never stored: token_hash is its SHA-256.
CREATE TABLE stream_tokens (
    token_hash    TEXT PRIMARY KEY,
    user_id       TEXT REFERENCES users(id) ON DELETE CASCADE,
    profile_id    TEXT NOT NULL DEFAULT '',
    media_item_id TEXT NOT NULL,
    scope         TEXT NOT NULL,
    cast_token    BOOLEAN NOT NULL DEFAULT FALSE,
    room_id       TEXT NOT NULL DEFAULT '',
    ttl_ms        BIGINT NOT NULL,
    issued_at     TIMESTAMPTZ NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_stream_tokens_user_id ON stream_tokens (user_id);
CREATE INDEX idx_stream_tokens_room_id ON stream_tokens (room_id) WHERE room_id <> '';
CREATE INDEX idx_stream_tokens_expires_at ON stream_tokens (expires_at);
