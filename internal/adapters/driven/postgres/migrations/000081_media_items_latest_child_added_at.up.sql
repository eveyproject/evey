-- Denormalized "most recent episode added_at" per series, used to sort
-- the home page "Recently added" by latest episode (series only) without
-- a LATERAL join per row.
ALTER TABLE media_items ADD COLUMN IF NOT EXISTS latest_child_added_at TIMESTAMPTZ;

-- Backfill for existing series: pick MAX(episode.added_at) among descendants.
-- Seasons sit between series and episodes in the hierarchy.
UPDATE media_items s SET latest_child_added_at = sub.max_added
FROM (
    SELECT season.parent_id AS series_id, MAX(ep.added_at) AS max_added
    FROM media_items ep
    INNER JOIN media_items season ON season.id = ep.parent_id AND season.type = 'season'
    WHERE ep.type = 'episode'
    GROUP BY season.parent_id
) sub
WHERE s.type = 'series' AND s.id = sub.series_id;

-- Partial index: only series rows have a value, and we always ORDER on it.
CREATE INDEX IF NOT EXISTS idx_media_items_latest_child_added_at
    ON media_items (latest_child_added_at DESC NULLS LAST)
    WHERE type = 'series';
