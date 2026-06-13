-- Recreate the legacy 1-10 star-rating table (matches 000027). The table is
-- unused by application code after the removal; this exists only so the
-- migration is reversible.
CREATE TABLE IF NOT EXISTS media_ratings (
    media_item_id TEXT NOT NULL,
    profile_id    TEXT NOT NULL,
    value         SMALLINT NOT NULL CHECK (value BETWEEN 1 AND 5),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (media_item_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_media_ratings_media_item_id ON media_ratings(media_item_id);
