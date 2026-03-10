ALTER TABLE collections ADD COLUMN IF NOT EXISTS filter_rules JSONB NOT NULL DEFAULT '{}';
ALTER TABLE collections ADD COLUMN IF NOT EXISTS sort_field TEXT NOT NULL DEFAULT 'sort_title';
ALTER TABLE collections ADD COLUMN IF NOT EXISTS poster_path TEXT;

UPDATE collections SET filter_rules = rules, sort_field = sort_by;

ALTER TABLE collections DROP COLUMN IF EXISTS rules;
ALTER TABLE collections DROP COLUMN IF EXISTS sort_by;
ALTER TABLE collections DROP COLUMN IF EXISTS updated_at;
