-- One answer to "has this profile watched this title", for every query that
-- asks it (home rails, library lists, the watched mark on a card). A film or
-- an episode is watched by its own completed row. A series or a season is
-- watched once it has episodes and every one of them is completed: a new
-- episode brings it back. The rule was copied inline into each feed query and
-- re-derived by each client, and the copies drifted.
CREATE OR REPLACE FUNCTION media_watched(p_profile TEXT, p_item TEXT) RETURNS BOOLEAN
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT COALESCE((
    SELECT CASE
      WHEN i.type IN ('series', 'season') THEN
        EXISTS (
          SELECT 1 FROM media_items e
          WHERE e.type = 'episode'
            AND (e.parent_id = i.id OR e.parent_id IN (SELECT s.id FROM media_items s WHERE s.parent_id = i.id))
        )
        AND NOT EXISTS (
          SELECT 1 FROM media_items e
          WHERE e.type = 'episode'
            AND (e.parent_id = i.id OR e.parent_id IN (SELECT s.id FROM media_items s WHERE s.parent_id = i.id))
            AND NOT EXISTS (
              SELECT 1 FROM watch_progress wp
              WHERE wp.profile_id = p_profile AND wp.media_item_id = e.id AND wp.completed
            )
        )
      ELSE COALESCE((
        SELECT wp.completed FROM watch_progress wp
        WHERE wp.profile_id = p_profile AND wp.media_item_id = i.id
      ), FALSE)
    END
    FROM media_items i WHERE i.id = p_item
  ), FALSE)
$$;
