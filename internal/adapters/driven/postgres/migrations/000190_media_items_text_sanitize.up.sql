-- Provider text used to reach the catalogue as delivered: control characters
-- (C0 but line and tab breaks, C1), bidi overrides (U+202A-202E), the
-- zero-width space and the byte-order mark. The joiners, LRM/RLM and the bidi
-- isolates are text a title is written with and are kept. They are now
-- stopped at the provider boundary; the rows written before are cleaned here,
-- once. Idempotent: a clean row matches nothing.
-- people.name is left alone: it is unique, and two names that differ only by
-- such a character would collide.
UPDATE media_items
   SET title = regexp_replace(title, '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]', '', 'g'),
       original_title = regexp_replace(original_title, '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]', '', 'g'),
       overview = regexp_replace(overview, '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]', '', 'g'),
       tagline = regexp_replace(tagline, '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]', '', 'g')
 WHERE title ~ '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]'
    OR original_title ~ '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]'
    OR overview ~ '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]'
    OR tagline ~ '[\x01-\x08\x0B\x0C\x0E-\x1F\x7F\u0080-\u009F\u200B\u202A-\u202E\uFEFF]';
