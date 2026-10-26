ALTER TABLE pending_search_index
    DROP COLUMN IF EXISTS dead_lettered_at,
    DROP COLUMN IF EXISTS attempts;
