-- A double-tapped Create landed two identical playlists: nothing enforced
-- name uniqueness per profile. Rename existing duplicates (the id suffix is
-- guaranteed collision-free - ids are unique), then make the pair unique.
UPDATE playlists p
SET name = p.name || ' [' || p.id || ']'
FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY profile_id, name ORDER BY created_at, id) AS rn
    FROM playlists
) d
WHERE p.id = d.id AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_playlists_profile_name ON playlists (profile_id, name);
