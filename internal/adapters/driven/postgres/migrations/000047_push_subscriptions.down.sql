DROP TABLE IF EXISTS push_subscriptions;

CREATE TABLE IF NOT EXISTS fcm_device_tokens (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id    TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    platform   TEXT NOT NULL DEFAULT 'web',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_profile_id ON fcm_device_tokens(profile_id);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_user_id ON fcm_device_tokens(user_id);
