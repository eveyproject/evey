-- file_events has had ZERO Go writers or readers since the watcher pipeline
-- went in-memory (debounce + direct scan trigger): rows were never inserted,
-- so the table is dead weight - two indexes maintained for nothing and a trap
-- for anyone reading the schema as documentation. Confirmed dead by audit
-- 2026-07-21 and re-verified 2026-07-23 (only references = migration 000001).
DROP TABLE IF EXISTS file_events;
