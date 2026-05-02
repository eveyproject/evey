CREATE TABLE IF NOT EXISTS intro_fingerprint_cache (
    media_item_id    TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    file_fingerprint TEXT NOT NULL,
    phash_tiles      BYTEA NOT NULL,
    phash_timestamps BYTEA NOT NULL,
    algo_version     SMALLINT NOT NULL,
    extracted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_intro_fp_algo ON intro_fingerprint_cache(algo_version);
