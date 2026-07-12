-- The home page "Recently added" feed runs an episode sub-aggregate
--   WHERE type = 'episode' AND added_at > NOW() - ($n || ' days')::interval
--   GROUP BY parent_id
-- on every home load. Episodes are the bulk of a real library (10k+ rows), and
-- the only added_at index so far is the partial idx_media_items_latest_child_added_at
-- (type='series' only, migration 000081) - so this filter sequentially scanned
-- every episode row on the single most frequent request. A (type, added_at)
-- composite turns it into a bounded range scan (equality on type, range on the
-- recency window) and also serves type-scoped recency reads for movies/albums.
CREATE INDEX IF NOT EXISTS idx_media_items_type_added_at
    ON media_items (type, added_at DESC);
