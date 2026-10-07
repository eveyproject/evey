-- A request used to carry only title, year and poster. The operator judging it
-- had to open a browser to learn what the title is, when it came out and how it
-- is rated - the three things the requester saw on the search result. Kept with
-- the request, optional: rows written before stay empty rather than guessed.
ALTER TABLE media_requests
    ADD COLUMN IF NOT EXISTS overview     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS release_date DATE,
    ADD COLUMN IF NOT EXISTS rating       DOUBLE PRECISION;
