ALTER TABLE collections ADD COLUMN IF NOT EXISTS rules JSONB NOT NULL DEFAULT '[]';
ALTER TABLE collections ADD COLUMN IF NOT EXISTS sort_by TEXT NOT NULL DEFAULT 'sort_title';
ALTER TABLE collections ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE collections SET rules = CASE
    WHEN filter_rules IS NOT NULL AND filter_rules != '{}'::jsonb THEN filter_rules
    ELSE '[]'::jsonb
END;
UPDATE collections SET sort_by = sort_field WHERE sort_field IS NOT NULL AND sort_field != '';

ALTER TABLE collections DROP COLUMN IF EXISTS filter_rules;
ALTER TABLE collections DROP COLUMN IF EXISTS sort_field;
ALTER TABLE collections DROP COLUMN IF EXISTS poster_path;
