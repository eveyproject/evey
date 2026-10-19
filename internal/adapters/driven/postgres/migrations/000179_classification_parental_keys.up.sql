-- The parental age cap was applied in Go, item by item, so the filter panel's
-- facets for a kids profile walked the whole library 500 rows at a time to
-- learn which items it may see. These columns hold the half of the rule that
-- does not depend on the viewer (ratings/precomputed.go): the highest age
-- across the country certifications, and the official rating as the tables
-- key it. The viewer's region settles the rest inside the query.
--
-- parental_rev names the rating tables the two values were computed with.
-- NULL (every row right after this migration) or a stale revision reads as
-- unknown, which a kids profile does not see; the server recomputes those
-- rows at startup.
ALTER TABLE media_classifications ADD COLUMN cert_min_age SMALLINT NOT NULL DEFAULT -1;
ALTER TABLE media_classifications ADD COLUMN rating_key TEXT NOT NULL DEFAULT '';
ALTER TABLE media_classifications ADD COLUMN parental_rev BIGINT;
