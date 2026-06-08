-- The photo grid (photo_repo.go ListByLibrary/ListByAlbum/Search) filters by
-- library_id then sorts the WHOLE matching set by `taken_at DESC NULLS LAST,
-- created_at DESC`. Only library_id and (library_id, album) were indexed, so
-- Postgres read + sorted every photo in the library on each page. This composite
-- index serves the equality filter AND the sort in one ordered scan. The NULLS
-- LAST / DESC opclasses match the query's ORDER BY exactly so the planner can
-- use it to avoid the sort.
CREATE INDEX IF NOT EXISTS idx_photos_library_taken_at
    ON photos (library_id, taken_at DESC NULLS LAST, created_at DESC);
