-- What a series or a saga contains according to the catalog source (TMDB),
-- owned or not, so missing episodes and films are worked out without calling
-- the source on every read. Refreshed by the server at its own pace.
CREATE TABLE IF NOT EXISTS series_expected_episodes (
    series_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    season         INTEGER NOT NULL,
    episode        INTEGER NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    air_date       DATE,
    PRIMARY KEY (series_item_id, season, episode)
);

CREATE TABLE IF NOT EXISTS series_catalog_checks (
    series_item_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    checked_at     TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS collection_parts (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    tmdb_id       INTEGER NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    release_date  DATE,
    poster_url    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (collection_id, tmdb_id)
);

CREATE TABLE IF NOT EXISTS collection_catalog_checks (
    collection_id TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
    checked_at    TIMESTAMPTZ NOT NULL
);
