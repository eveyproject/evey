ALTER TABLE plugins ALTER COLUMN kind DROP NOT NULL;
UPDATE plugins SET kind = '' WHERE kind IN ('plugin', 'theme');
