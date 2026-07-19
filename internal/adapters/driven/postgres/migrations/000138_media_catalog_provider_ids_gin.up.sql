-- The media-request search asks "is this title already on the server?" for every
-- candidate it shows, matched on media_catalog.provider_ids (e.g. {"tmdb":"438631"}).
-- That runs on a user-facing search path, so it must not sequential-scan the
-- catalogue as the library grows.
--
-- jsonb_path_ops rather than the default jsonb_ops: the lookup only ever uses
-- containment (provider_ids @> '{"tmdb":"438631"}'), which is exactly what
-- jsonb_path_ops indexes - it is smaller and faster than the general operator
-- class, whose extra key-existence support we never query.
CREATE INDEX IF NOT EXISTS idx_media_catalog_provider_ids_gin
    ON media_catalog USING gin (provider_ids jsonb_path_ops);
