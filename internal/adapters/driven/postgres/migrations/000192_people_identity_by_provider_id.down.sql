-- The previous code inserts with ON CONFLICT (name), so the full unique name
-- must come back. Homonyms exist by now (two provider ids under one name, and
-- the name clean-up of the up can itself have made two names one): no row is
-- dropped, the homonyms after the first (the id-less one, else the lowest row
-- id) are renamed with their provider id, or their row id when that provider
-- id is shared too. Deterministic, so a replayed down gives the same names.
-- The cleaned names carried nothing a reader wanted; nothing to put back.
WITH ranked AS (
    SELECT id, name,
           NULLIF(provider_ids->>'tmdb', '') AS tmdb,
           row_number() OVER (PARTITION BY name
                              ORDER BY (COALESCE(provider_ids->>'tmdb', '') <> ''), id) AS rn,
           count(*) OVER (PARTITION BY name, COALESCE(provider_ids->>'tmdb', '')) AS same_id
      FROM people
)
UPDATE people p
   SET name = r.name || ' (' ||
              CASE WHEN r.tmdb IS NOT NULL AND r.same_id = 1 THEN 'tmdb ' || r.tmdb ELSE r.id END || ')'
  FROM ranked r
 WHERE p.id = r.id AND r.rn > 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_name_unique ON people(name);
DROP INDEX IF EXISTS idx_people_name_unique_no_tmdb;
DROP INDEX IF EXISTS idx_people_tmdb_id;
