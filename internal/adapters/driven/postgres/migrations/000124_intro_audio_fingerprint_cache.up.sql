CREATE TABLE IF NOT EXISTS intro_audio_fingerprint_cache (
    media_item_id    TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    file_fingerprint TEXT NOT NULL,
    window_sec       INTEGER NOT NULL,
    fingerprint      BYTEA NOT NULL,
    algo_version     SMALLINT NOT NULL,
    extracted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_intro_audio_fp_algo ON intro_audio_fingerprint_cache(algo_version);
