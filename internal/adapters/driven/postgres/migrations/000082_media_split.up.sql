-- PR 1 - transparent SQL split. Separates four concerns out of the
-- 32-column media_items table into 1:1 (or 1:N) sub-tables; the Go
-- MediaItem struct keeps its flat shape and the repo reassembles every
-- column on read via LEFT JOIN + jsonb_agg subqueries. Net effect:
-- callers see no behaviour change, persistence is no longer one fat row,
-- and a future PR can promote the sub-tables to first-class aggregates.
--
-- Out of scope here: overview/tagline split into media_descriptions (kept
-- on media_items) and the existing media_item_translations table (left
-- untouched). Both move in PR 2.

BEGIN;

-- 1. media_files (1:1 optional - series/seasons/albums own no file)
CREATE TABLE media_files (
    media_item_id    TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    file_path        TEXT NOT NULL,
    file_size        BIGINT NOT NULL DEFAULT 0,
    file_fingerprint TEXT,
    mod_time         TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01 00:00:00+00',
    duration_ms      BIGINT NOT NULL DEFAULT 0,
    bitrate          BIGINT NOT NULL DEFAULT 0,
    container_format TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX idx_media_files_fingerprint
    ON media_files (file_fingerprint) WHERE file_fingerprint IS NOT NULL;
CREATE INDEX idx_media_files_path ON media_files (file_path);

INSERT INTO media_files (
    media_item_id, file_path, file_size, file_fingerprint,
    mod_time, duration_ms, bitrate, container_format
)
SELECT id, file_path, file_size, file_fingerprint,
       mod_time, duration_ms, bitrate, container_format
FROM media_items
WHERE file_path IS NOT NULL AND file_path != '';

-- 2. media_catalog (1:1 optional - created when any catalog field is set)
CREATE TABLE media_catalog (
    media_item_id  TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    premiere_date  TIMESTAMPTZ,
    year           INTEGER,
    provider_ids   JSONB NOT NULL DEFAULT '{}'::jsonb,
    genres         JSONB NOT NULL DEFAULT '[]'::jsonb,
    studios        JSONB NOT NULL DEFAULT '[]'::jsonb,
    tags           JSONB NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX idx_media_catalog_year ON media_catalog (year) WHERE year IS NOT NULL;
CREATE INDEX idx_media_catalog_genres_gin ON media_catalog USING gin (genres);

INSERT INTO media_catalog (
    media_item_id, premiere_date, year, provider_ids, genres, studios, tags
)
SELECT id, premiere_date, year, provider_ids, genres, studios, tags
FROM media_items
WHERE premiere_date IS NOT NULL
   OR year IS NOT NULL
   OR provider_ids != '{}'::jsonb
   OR genres != '[]'::jsonb
   OR studios != '[]'::jsonb
   OR tags != '[]'::jsonb;

-- 3. media_classifications (1:1 optional)
-- ratings (provider critic scores) is renamed provider_ratings to free
-- the unqualified `ratings` term for user ratings (already its own table).
CREATE TABLE media_classifications (
    media_item_id              TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    community_rating           DOUBLE PRECISION,
    official_rating            TEXT NOT NULL DEFAULT '',
    certifications_by_country  JSONB NOT NULL DEFAULT '{}'::jsonb,
    provider_ratings           JSONB NOT NULL DEFAULT '{}'::jsonb
);

INSERT INTO media_classifications (
    media_item_id, community_rating, official_rating,
    certifications_by_country, provider_ratings
)
SELECT id, community_rating, official_rating,
       certifications_by_country, ratings
FROM media_items
WHERE community_rating IS NOT NULL
   OR official_rating != ''
   OR certifications_by_country != '{}'::jsonb
   OR ratings != '{}'::jsonb;

-- 4. media_trailers (1:N - replaces JSONB array)
CREATE TABLE media_trailers (
    id                TEXT PRIMARY KEY,
    media_item_id     TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    position          INTEGER NOT NULL DEFAULT 0,
    site              TEXT NOT NULL DEFAULT '',
    key               TEXT NOT NULL DEFAULT '',
    language          TEXT NOT NULL DEFAULT '',
    official          BOOLEAN NOT NULL DEFAULT false,
    type              TEXT NOT NULL DEFAULT '',
    use_subtitles     BOOLEAN NOT NULL DEFAULT false,
    subtitle_language TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_media_trailers_media_item_id
    ON media_trailers (media_item_id, position);

INSERT INTO media_trailers (
    id, media_item_id, position, site, key, language, official, type,
    use_subtitles, subtitle_language
)
SELECT
    'tlr-' || mi.id || '-' || (ord - 1),
    mi.id,
    (ord - 1),
    COALESCE(elem ->> 'Site', elem ->> 'site', ''),
    COALESCE(elem ->> 'Key', elem ->> 'key', ''),
    COALESCE(elem ->> 'Language', elem ->> 'language', ''),
    COALESCE((elem ->> 'Official')::boolean, (elem ->> 'official')::boolean, false),
    COALESCE(elem ->> 'Type', elem ->> 'type', ''),
    COALESCE((elem ->> 'UseSubtitles')::boolean, (elem ->> 'use_subtitles')::boolean, false),
    COALESCE(elem ->> 'SubtitleLanguage', elem ->> 'subtitle_language', '')
FROM media_items mi,
     LATERAL jsonb_array_elements(mi.trailers) WITH ORDINALITY AS t(elem, ord)
WHERE jsonb_array_length(mi.trailers) > 0;

-- 5. Drop migrated columns from media_items + their now-orphan indexes.
DROP INDEX IF EXISTS idx_media_items_file_fingerprint;
DROP INDEX IF EXISTS idx_media_items_file_path;
DROP INDEX IF EXISTS idx_media_items_genres_gin;
DROP INDEX IF EXISTS idx_media_items_year;

ALTER TABLE media_items
    DROP COLUMN file_path,
    DROP COLUMN file_size,
    DROP COLUMN file_fingerprint,
    DROP COLUMN mod_time,
    DROP COLUMN duration_ms,
    DROP COLUMN bitrate,
    DROP COLUMN container_format,
    DROP COLUMN community_rating,
    DROP COLUMN official_rating,
    DROP COLUMN certifications_by_country,
    DROP COLUMN ratings,
    DROP COLUMN premiere_date,
    DROP COLUMN year,
    DROP COLUMN provider_ids,
    DROP COLUMN genres,
    DROP COLUMN studios,
    DROP COLUMN tags,
    DROP COLUMN trailers;

COMMIT;
