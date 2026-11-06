-- A person was identified by its name alone (idx_people_name_unique), so two
-- people the provider knows under two ids but credited under one name were
-- merged into one row: one biography, one photo, one filmography. The
-- provider id becomes the identity and the name only identifies a person the
-- provider gave no id for.
--
-- The tmdb id index is NOT unique: rows credited under two spellings of one
-- id may already exist, and a unique index would refuse to build on them.
CREATE INDEX IF NOT EXISTS idx_people_tmdb_id ON people ((provider_ids->>'tmdb'));

-- Built before the full unique is dropped: its rows are a subset of a column
-- that is unique today, so it cannot fail on existing data.
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_name_unique_no_tmdb ON people (name)
    WHERE COALESCE(provider_ids->>'tmdb', '') = '';

DROP INDEX IF EXISTS idx_people_name_unique;

-- The names left out of the media_items text clean-up (000190) because the
-- unique name made two cleaned names collide. A row with a provider id no
-- longer collides on its name. A row without one is cleaned only when its
-- cleaned name is free among the id-less rows; the others keep their name,
-- the enrichment finds them by id from now on. Idempotent: a clean row
-- matches nothing.
WITH dirty AS (
    SELECT id,
           regexp_replace(name, '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]', '', 'g') AS clean,
           COALESCE(provider_ids->>'tmdb', '') <> '' AS has_id
      FROM people
     WHERE name ~ '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]'
), ranked AS (
    SELECT id, clean, has_id,
           row_number() OVER (PARTITION BY clean, has_id ORDER BY id) AS rn
      FROM dirty
     WHERE clean <> ''
)
UPDATE people p
   SET name = r.clean
  FROM ranked r
 WHERE p.id = r.id
   AND (r.has_id
        OR (r.rn = 1 AND NOT EXISTS (
                SELECT 1 FROM people o
                 WHERE o.name = r.clean
                   AND COALESCE(o.provider_ids->>'tmdb', '') = '')));
