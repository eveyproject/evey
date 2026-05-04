-- Cache for external intro/recap/credits metadata fetched from TIDB / IntroDB.
-- Keyed by (tmdb_id, season, episode) which is what TIDB indexes on; rows for
-- movies use season=0, episode=0. `payload` carries the full JSON returned by
-- the provider, denormalised into MediaSegments by the app layer at read time.
CREATE TABLE IF NOT EXISTS intro_metadata_cache (
    tmdb_id    INTEGER NOT NULL,
    season     INTEGER NOT NULL DEFAULT 0,
    episode    INTEGER NOT NULL DEFAULT 0,
    source     TEXT    NOT NULL,
    payload    JSONB   NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tmdb_id, season, episode, source)
);

CREATE INDEX IF NOT EXISTS idx_intro_metadata_cache_fetched_at
    ON intro_metadata_cache(fetched_at);
