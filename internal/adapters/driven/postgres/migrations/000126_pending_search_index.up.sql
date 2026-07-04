-- Durable outbox for search-index writes that failed while Meilisearch was down.
--
-- The resilient search wrapper used to SWALLOW a write when the engine was
-- unreachable, relying on a count-based reconcile to heal later — but that
-- reconcile can't see set-drift that nets to zero (equal adds+deletes during an
-- outage), so items silently fell out of search. Instead we now record the
-- failed write here and a drain worker replays it (re-reading the item from the
-- DB, the source of truth) until Meili accepts it. Durable so a restart mid-
-- outage doesn't lose the pending work.
--
-- One row per item (PK): a newer op supersedes an older pending one
-- (upsert-then-delete during an outage → the delete wins), via ON CONFLICT.
CREATE TABLE IF NOT EXISTS pending_search_index (
    media_item_id TEXT PRIMARY KEY,
    op            TEXT NOT NULL CHECK (op IN ('upsert', 'delete')),
    enqueued_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pending_search_index_enqueued_at
    ON pending_search_index (enqueued_at);
