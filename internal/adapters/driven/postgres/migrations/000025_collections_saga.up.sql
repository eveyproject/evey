ALTER TABLE collections
  ADD COLUMN IF NOT EXISTS tmdb_id INTEGER,
  ADD COLUMN IF NOT EXISTS poster_url TEXT,
  ADD COLUMN IF NOT EXISTS backdrop_url TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_tmdb_id ON collections(tmdb_id) WHERE tmdb_id IS NOT NULL;

ALTER TABLE collections
  DROP COLUMN IF EXISTS type,
  DROP COLUMN IF EXISTS rules,
  DROP COLUMN IF EXISTS sort_by,
  DROP COLUMN IF EXISTS sort_order;
