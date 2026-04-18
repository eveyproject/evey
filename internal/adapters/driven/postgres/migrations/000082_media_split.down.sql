-- Reverse of 000082_media_split: re-add the columns on media_items,
-- copy the data back from the sub-tables, then drop the sub-tables.

BEGIN;

ALTER TABLE media_items
    ADD COLUMN file_path TEXT,
    ADD COLUMN file_size BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN file_fingerprint TEXT,
    ADD COLUMN mod_time TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01 00:00:00+00',
    ADD COLUMN duration_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN bitrate BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN container_format TEXT NOT NULL DEFAULT '',
    ADD COLUMN community_rating DOUBLE PRECISION,
    ADD COLUMN official_rating TEXT NOT NULL DEFAULT '',
    ADD COLUMN certifications_by_country JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN ratings JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN premiere_date TIMESTAMPTZ,
    ADD COLUMN year INTEGER,
    ADD COLUMN provider_ids JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN genres JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN studios JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN trailers JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE media_items mi SET
    file_path = mf.file_path,
    file_size = mf.file_size,
    file_fingerprint = mf.file_fingerprint,
    mod_time = mf.mod_time,
    duration_ms = mf.duration_ms,
    bitrate = mf.bitrate,
    container_format = mf.container_format
FROM media_files mf WHERE mf.media_item_id = mi.id;

UPDATE media_items mi SET
    premiere_date = mc.premiere_date,
    year = mc.year,
    provider_ids = mc.provider_ids,
    genres = mc.genres,
    studios = mc.studios,
    tags = mc.tags
FROM media_catalog mc WHERE mc.media_item_id = mi.id;

UPDATE media_items mi SET
    community_rating = mcl.community_rating,
    official_rating = mcl.official_rating,
    certifications_by_country = mcl.certifications_by_country,
    ratings = mcl.provider_ratings
FROM media_classifications mcl WHERE mcl.media_item_id = mi.id;

UPDATE media_items mi SET trailers = sub.arr
FROM (
    SELECT media_item_id, jsonb_agg(jsonb_build_object(
        'Site', site, 'Key', key, 'Language', language,
        'Official', official, 'Type', type,
        'UseSubtitles', use_subtitles, 'SubtitleLanguage', subtitle_language
    ) ORDER BY position) AS arr
    FROM media_trailers GROUP BY media_item_id
) sub WHERE sub.media_item_id = mi.id;

DROP TABLE media_trailers;
DROP TABLE media_classifications;
DROP TABLE media_catalog;
DROP TABLE media_files;

CREATE UNIQUE INDEX idx_media_items_file_fingerprint
    ON media_items (file_fingerprint) WHERE file_fingerprint IS NOT NULL;
CREATE INDEX idx_media_items_file_path ON media_items (file_path);
CREATE INDEX idx_media_items_genres_gin ON media_items USING gin (genres);
CREATE INDEX idx_media_items_year ON media_items (year) WHERE year IS NOT NULL;

COMMIT;
