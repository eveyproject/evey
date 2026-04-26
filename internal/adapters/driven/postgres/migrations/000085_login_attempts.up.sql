CREATE TABLE IF NOT EXISTS login_attempts (
    username      TEXT PRIMARY KEY,
    attempt_count INTEGER     NOT NULL DEFAULT 0,
    locked_until  TIMESTAMPTZ NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
