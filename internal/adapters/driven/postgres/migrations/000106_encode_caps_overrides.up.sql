-- transcode_caps_overrides stores the CMCD-feedback adaptive cap.
-- One row per source class (height bucket). Worker UPSERTs when the
-- starvation ratio on a class exceeds the threshold; rows expire
-- automatically after expires_at so a one-off bad network period
-- doesn't lock the cap forever.
--
-- The HLS-VOD handler reads this table at request time and applies
-- effective_cap = MIN(benchmark_cap, override_cap). A NULL override
-- (no row) means "use the benchmark cap".
--
-- source_class is a short tag mapping source-side dimensions to a
-- bucket the worker recognises ("1080p", "1440p", "2160p"). We
-- intentionally keep the granularity coarse for Phase C MVP - finer
-- buckets (per-codec, per-title) come later if real data justifies it.

CREATE TABLE IF NOT EXISTS transcode_caps_overrides (
    source_class TEXT PRIMARY KEY,
    max_height   INT  NOT NULL CHECK (max_height >= 480 AND max_height <= 2160),
    reason       TEXT NOT NULL DEFAULT '',
    starved_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    samples_count INT NOT NULL DEFAULT 0,
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL
);

-- Full index on expires_at - the table is tiny (a handful of rows,
-- bounded by the number of source-class buckets) so we don't need a
-- partial WHERE here. (An earlier draft used `WHERE expires_at < NOW() +
-- INTERVAL '1 day'` but NOW() isn't IMMUTABLE and Postgres rejects it
-- in a predicate, so the migration would land dirty.)
CREATE INDEX IF NOT EXISTS idx_transcode_caps_overrides_expires
    ON transcode_caps_overrides (expires_at);
