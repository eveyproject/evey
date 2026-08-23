-- Lists media items whose provider metadata probably belongs to another work.
--
-- READ ONLY: nothing here writes, creates an extension or takes a lock.
-- Run with:
--   ssh <host> 'docker exec -i evey-postgres psql -U evey -d evey' \
--     < docs/runbooks/diagnose-mismatched-metadata.sql
--
-- The signal mirrors the runtime guard in
-- internal/app/metadata/match_confidence.go: the share of identity-bearing
-- words of the filesystem-derived title (scan_title, else the file name) that
-- are found in a title the provider knows the work by (title, original_title,
-- any locale of media_item_translations). Below 0.35 with no word-boundary
-- containment, the match names a different work.
--
-- SQL is stricter than the Go guard: word comparison is exact here where the
-- Go side tolerates a typo (Levenshtein >= 0.75 per word, >= 0.85 on the whole
-- compacted string). Expect a few extra rows - "Edge of Tomorow", "U.S.
-- Marshals" and the like are false positives of THIS script, not of the guard.

\set ON_ERROR_STOP on

WITH
stopwords AS (
    SELECT unnest(ARRAY[
        'the','a','an','of','and','or','in','on','to','for','with',
        'le','la','les','l','un','une','de','des','du','d','et',
        'en','au','aux','sur','il'
    ]) AS w
),
item AS (
    SELECT
        mi.id,
        mi.type,
        mi.title,
        mi.original_title,
        mi.scan_title,
        mi.updated_at,
        c.provider_ids ->> 'tmdb' AS tmdb_id,
        f.file_path,
        -- Query side: scan_title when the scanner still has it, else the file
        -- name stripped of its extension. Note that a past bad match can have
        -- overwritten scan_title (prod: "2001 L'odyssee de l'espace" now reads
        -- "2001"), which is why the file name is kept as the fallback.
        coalesce(
            nullif(mi.scan_title, ''),
            regexp_replace(regexp_replace(coalesce(f.file_path, ''), '^.*/', ''), '\.[A-Za-z0-9]{2,4}$', ''),
            mi.title
        ) AS query_title
    FROM media_items mi
    LEFT JOIN media_catalog c ON c.media_item_id = mi.id
    LEFT JOIN media_files  f ON f.media_item_id = mi.id
    WHERE mi.type IN ('movie', 'series')
      AND coalesce(c.provider_ids ->> 'tmdb', '') <> ''
),
-- Every label the provider knows the work by.
candidate AS (
    SELECT id, title AS label FROM item WHERE coalesce(title, '') <> ''
    UNION ALL
    SELECT id, original_title FROM item WHERE coalesce(original_title, '') <> ''
    UNION ALL
    SELECT t.media_item_id, t.title
    FROM media_item_translations t
    JOIN item i ON i.id = t.media_item_id
    WHERE coalesce(t.title, '') <> ''
),
norm AS (
    SELECT
        i.id,
        -- No unaccent extension installed in production, and CREATE EXTENSION
        -- is a write: fold the Latin-1 range by hand instead.
        btrim(regexp_replace(
            translate(lower(i.query_title),
                      'àáâãäåèéêëìíîïòóôõöùúûüýÿñçœæ',
                      'aaaaaaeeeeiiiiooooouuuuyyncoa'),
            '[^a-z0-9]+', ' ', 'g')) AS q,
        btrim(regexp_replace(
            translate(lower(c.label),
                      'àáâãäåèéêëìíîïòóôõöùúûüýÿñçœæ',
                      'aaaaaaeeeeiiiiooooouuuuyyncoa'),
            '[^a-z0-9]+', ' ', 'g')) AS c
    FROM item i
    JOIN candidate c ON c.id = i.id
),
-- Stopwords are dropped, EXCEPT when a label is made only of stopwords - a
-- scan title reduced to "La" must still be compared, not silently skipped.
qword AS (
    SELECT n.id, n.q, n.c, w.word
    FROM norm n,
         LATERAL unnest(string_to_array(n.q, ' ')) AS w(word)
    WHERE w.word <> ''
      AND (w.word NOT IN (SELECT w FROM stopwords)
           OR NOT EXISTS (
               SELECT 1 FROM unnest(string_to_array(n.q, ' ')) AS x(word)
               WHERE x.word <> '' AND x.word NOT IN (SELECT w FROM stopwords)))
),
cword AS (
    SELECT DISTINCT n.id, n.q, n.c, w.word
    FROM norm n,
         LATERAL unnest(string_to_array(n.c, ' ')) AS w(word)
    WHERE w.word <> ''
      AND (w.word NOT IN (SELECT w FROM stopwords)
           OR NOT EXISTS (
               SELECT 1 FROM unnest(string_to_array(n.c, ' ')) AS x(word)
               WHERE x.word <> '' AND x.word NOT IN (SELECT w FROM stopwords)))
),
pair AS (
    SELECT
        q.id, q.q, q.c,
        count(*)                                          AS q_words,
        count(*) FILTER (WHERE c.word IS NOT NULL)        AS hits,
        count(*) FILTER (WHERE q.word !~ '^[0-9]+$'
                           AND q.word NOT IN (SELECT w FROM stopwords)) AS q_ident
    FROM (SELECT DISTINCT id, q, c, word FROM qword) q
    LEFT JOIN cword c ON c.id = q.id AND c.q = q.q AND c.c = q.c AND c.word = q.word
    GROUP BY q.id, q.q, q.c
),
cident AS (
    SELECT id, q, c,
           count(*) FILTER (WHERE word !~ '^[0-9]+$'
                              AND word NOT IN (SELECT w FROM stopwords)) AS n
    FROM cword
    GROUP BY id, q, c
),
scored AS (
    SELECT
        p.id,
        max(
            CASE
                -- Word-boundary containment: the file name carries a marketing
                -- subtitle the provider title drops, or the reverse. The
                -- contained side must carry a word that is neither a stopword
                -- nor a bare number, otherwise a title reduced to "2001" or a
                -- scan title reduced to "La" swallows anything.
                WHEN length(p.c) <= length(p.q)
                 AND coalesce(ci.n, 0) > 0
                 AND position(' ' || p.c || ' ' IN ' ' || p.q || ' ') > 0
                THEN 1.0
                WHEN length(p.q) < length(p.c)
                 AND p.q_ident > 0
                 AND position(' ' || p.q || ' ' IN ' ' || p.c || ' ') > 0
                THEN 1.0
                ELSE p.hits::numeric / nullif(p.q_words, 0)
            END
        ) AS coverage,
        max(similarity(p.q, p.c)) AS trigram
    FROM pair p
    LEFT JOIN cident ci ON ci.id = p.id AND ci.q = p.q AND ci.c = p.c
    GROUP BY p.id
)
SELECT
    round(s.coverage, 2) AS coverage,
    round(s.trigram::numeric, 2) AS trigram,
    i.type,
    i.tmdb_id,
    i.query_title       AS parsed_from_disk,
    i.title             AS provider_title,
    i.original_title    AS provider_original_title,
    i.file_path,
    i.updated_at::date  AS matched_on,
    i.id                AS media_item_id
FROM scored s
JOIN item i ON i.id = s.id
WHERE s.coverage < 0.35
ORDER BY s.coverage, i.type, i.title;

-- Second signal, independent of titles: two items sharing one TMDB id. Most
-- groups are the same film in two qualities (legitimate), so read the file
-- names - a group whose members are clearly different works is a bad match.
SELECT
    c.provider_ids ->> 'tmdb' AS tmdb_id,
    count(*)                  AS items,
    string_agg(coalesce(f.file_path, mi.id), ' | ' ORDER BY f.file_path) AS files,
    min(mi.title)             AS provider_title
FROM media_items mi
JOIN media_catalog c ON c.media_item_id = mi.id
LEFT JOIN media_files f ON f.media_item_id = mi.id
WHERE mi.type = 'movie'
  AND coalesce(c.provider_ids ->> 'tmdb', '') <> ''
GROUP BY 1
HAVING count(*) > 1
ORDER BY 2 DESC, 1;

-- ---------------------------------------------------------------------------
-- Repairing a row this script flags (nothing below is executed by the script).
--
-- 1. A past bad match can have overwritten scan_title with the wrong title
--    ("2001 L'odyssee de l'espace" now reads "2001"), so read file_path, never
--    scan_title, to decide what the file really is.
-- 2. Force a re-search for the whole library - the forced path drops the year
--    filter and re-runs the provider chain, and the guard in
--    match_confidence.go now refuses an unrelated result instead of storing it:
--      POST /api/v1/admin/metadata/refresh-all?library_id={libraryID}
--    or, for a single row, POST /api/v1/admin/media/{mediaItemID}/refresh-metadata
-- 3. Rows the provider still cannot match are left as orphans (no tmdb id).
--    Correct those by hand from the admin UI: editing the title of an orphan
--    writes a metadata_overrides row AND is treated as a search request, so a
--    corrected title re-triggers the match (see autoEnrichItem).
-- 4. Rows whose file name is simply wrong ("The Megeladon" for Megalodon) are
--    best fixed on disk, then re-scanned: the parsed title is the only input
--    the matcher has.
