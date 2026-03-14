CREATE TABLE IF NOT EXISTS user_ratings (
    profile_id     TEXT NOT NULL,
    media_item_id  TEXT NOT NULL,
    value          SMALLINT NOT NULL CHECK (value BETWEEN -1 AND 1), -- -1=dislike, 0=neutral/removed, 1=like
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (profile_id, media_item_id)
);

CREATE INDEX IF NOT EXISTS idx_user_ratings_media_item ON user_ratings(media_item_id);
