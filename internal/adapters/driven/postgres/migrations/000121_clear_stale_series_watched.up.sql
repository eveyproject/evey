-- Backfill: clear the stale "fully watched" flag on series that actually have an
-- unwatched episode for that profile. A series gets watch_progress.completed=true
-- when the profile finishes it, but adding a new episode never reset that flag,
-- so a watched series with a new episode stayed hidden in the home "Séries"
-- slider (which filters out completed series). The "Recently Added" feed already
-- recomputes this live; this aligns the stored flag for the slider, and
-- clearSeriesWatchedOnNewEpisode keeps it correct on every future episode add.
UPDATE watch_progress wp
SET completed = FALSE
WHERE wp.completed = TRUE
  AND EXISTS (
    SELECT 1 FROM media_items s
    WHERE s.id = wp.media_item_id AND s.type = 'series'
  )
  AND EXISTS (
    -- at least one episode of this series the profile has NOT completed
    SELECT 1
    FROM media_items ep
    JOIN media_items se ON se.id = ep.parent_id
    WHERE se.parent_id = wp.media_item_id
      AND ep.type = 'episode'
      AND NOT EXISTS (
        SELECT 1 FROM watch_progress ewp
        WHERE ewp.media_item_id = ep.id
          AND ewp.profile_id = wp.profile_id
          AND ewp.completed = TRUE
      )
  );
