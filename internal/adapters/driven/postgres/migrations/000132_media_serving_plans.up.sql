-- media_serving_plans: precomputed playback serving decision for a media
-- item's source file, produced at scan time by mkvgo's ops.Ingest (v0.17.0)
-- via the IngestPlanner port. One row per media item (1:1, PK = FK), so a
-- re-scan/re-plan overwrites in place rather than accumulating history.
-- target/strategy are pulled out of plan_json as queryable columns; plan_json
-- keeps the full decision (reasons, ladder, playability report) for a future
-- admin/debug surface. Consuming the plan in the serving engine itself is a
-- later iteration - this table only stores the result.
CREATE TABLE IF NOT EXISTS media_serving_plans (
    media_item_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    target        TEXT NOT NULL,
    strategy      TEXT NOT NULL,
    plan_json     JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
