-- A document the search engine refuses (not an outage: the engine answers and
-- rejects it) used to be replayed every drain forever. attempts counts the
-- refusals; past the limit the row is dead-lettered and no longer listed.
ALTER TABLE pending_search_index
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMPTZ;
