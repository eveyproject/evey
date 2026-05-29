-- Per-title encoding light: classify each media item at scan time so the ABR
-- ladder can be picked per content (animation → light ladder, action 4K HDR
-- → heavy ladder) instead of the one-size-fits-all fixed ladder we used
-- before. Nullable so existing rows behave exactly like before (fall back to
-- LadderNormal in code); a background worker fills these in idle time.
ALTER TABLE media_items
    ADD COLUMN IF NOT EXISTS complexity_class       TEXT,
    ADD COLUMN IF NOT EXISTS complexity_metrics     JSONB,
    ADD COLUMN IF NOT EXISTS complexity_computed_at TIMESTAMPTZ;

-- Sparse index: most rows will have a non-null class once the worker has
-- caught up, but we query for "what's left to classify" frequently when
-- bootstrapping or after a library rescan.
CREATE INDEX IF NOT EXISTS idx_media_items_unclassified
    ON media_items (id)
    WHERE complexity_class IS NULL;
