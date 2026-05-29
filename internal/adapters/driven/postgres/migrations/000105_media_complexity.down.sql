DROP INDEX IF EXISTS idx_media_items_unclassified;
ALTER TABLE media_items
    DROP COLUMN IF EXISTS complexity_computed_at,
    DROP COLUMN IF EXISTS complexity_metrics,
    DROP COLUMN IF EXISTS complexity_class;
