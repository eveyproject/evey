ALTER TABLE plugins
    DROP COLUMN IF EXISTS update_state,
    DROP COLUMN IF EXISTS auto_update,
    DROP COLUMN IF EXISTS update_source_id,
    DROP COLUMN IF EXISTS update_source;
