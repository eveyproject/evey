-- A plugin can follow the repository it came from. update_source is that
-- repository (empty: it follows none), update_source_id its permanent GitHub
-- id, which releases are read by (a name can be taken over, the id cannot;
-- 0 until known), auto_update lets a release install
-- without an admin when nothing in it is new and sensitive (on by default),
-- update_state is the background check's last record. Plugin updates rewrite
-- the row through the full UPDATE, which never names these three columns, so
-- an upgrade keeps what the admin chose.
ALTER TABLE plugins
    ADD COLUMN IF NOT EXISTS update_source TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS update_source_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS auto_update BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS update_state JSONB NOT NULL DEFAULT '{}'::jsonb;
