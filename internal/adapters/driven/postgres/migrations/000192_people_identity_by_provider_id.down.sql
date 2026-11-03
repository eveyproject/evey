-- The full unique name comes back only if no two rows share a name: two
-- homonyms with distinct provider ids are two people, and choosing which one
-- to delete is not a migration's call. Refusing loudly beats a schema the
-- previous code (ON CONFLICT (name)) cannot insert into. The cleaned names
-- carried nothing a reader wanted; nothing to put back.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM people GROUP BY name HAVING count(*) > 1) THEN
        RAISE EXCEPTION 'people holds homonyms with distinct provider ids; merge or rename them before reverting 000192';
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_name_unique ON people(name);
DROP INDEX IF EXISTS idx_people_name_unique_no_tmdb;
DROP INDEX IF EXISTS idx_people_tmdb_id;
