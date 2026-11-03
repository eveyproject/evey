-- A missing artwork used to be fetched every hour for good, a dead source
-- included. fetch_attempts counts the misses, next_fetch_at says when the next
-- try is due (doubling wait), last_fetch_error says why; the hourly pass lists
-- only what is due and below the maximum. Rows are kept: the source URL is
-- what tells a re-enrichment the artwork set is unchanged.
ALTER TABLE artwork
    ADD COLUMN IF NOT EXISTS fetch_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_fetch_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_fetch_error TEXT;
