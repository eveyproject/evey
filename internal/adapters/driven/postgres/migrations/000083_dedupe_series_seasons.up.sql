-- Purge duplicate series and seasons that the TV-show scanner created
-- before the scan-title-based dedup fix. The scanner used to key off
-- MediaItem.Title, which metadata enrichment overwrites with the
-- enriched/localized title — so every subsequent scan looked up with
-- the raw FS title, missed the (already-renamed) existing row, and
-- inserted a new one. Dozens of duplicate series/season rows per
-- (library, scan_title) accumulated in long-running dev databases.

BEGIN;

-- 1. Pick canonical series per (library_id, scan_title): oldest wins.
CREATE TEMP TABLE _series_winners ON COMMIT DROP AS
SELECT DISTINCT ON (mi.library_id, mi.scan_title)
       mi.id, mi.library_id, mi.scan_title
FROM media_items mi
WHERE mi.type = 'series'
ORDER BY mi.library_id, mi.scan_title, mi.added_at ASC, mi.id ASC;

-- 2. Reparent seasons whose parent is a losing series → point at the winner.
UPDATE media_items s
   SET parent_id = w.id
  FROM media_items losing
  JOIN _series_winners w
    ON w.library_id = losing.library_id
   AND w.scan_title = losing.scan_title
 WHERE losing.type = 'series'
   AND losing.id <> w.id
   AND s.parent_id = losing.id;

-- 3. Delete losing series (cascade removes orphaned child rows).
DELETE FROM media_items
 WHERE type = 'series'
   AND id NOT IN (SELECT id FROM _series_winners);

-- 4. Within each series, pick canonical season per index_number: oldest wins.
CREATE TEMP TABLE _season_winners ON COMMIT DROP AS
SELECT DISTINCT ON (mi.parent_id, mi.index_number)
       mi.id, mi.parent_id, mi.index_number
FROM media_items mi
WHERE mi.type = 'season' AND mi.index_number IS NOT NULL
ORDER BY mi.parent_id, mi.index_number, mi.added_at ASC, mi.id ASC;

-- 5. Reparent episodes whose parent is a losing season → point at the winner.
UPDATE media_items e
   SET parent_id = w.id
  FROM media_items losing
  JOIN _season_winners w
    ON w.parent_id = losing.parent_id
   AND w.index_number = losing.index_number
 WHERE losing.type = 'season'
   AND losing.id <> w.id
   AND e.parent_id = losing.id;

-- 6. Delete losing seasons.
DELETE FROM media_items
 WHERE type = 'season'
   AND index_number IS NOT NULL
   AND id NOT IN (SELECT id FROM _season_winners);

COMMIT;
