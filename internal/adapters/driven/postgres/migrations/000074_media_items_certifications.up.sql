ALTER TABLE media_items
  ADD COLUMN IF NOT EXISTS certifications_by_country JSONB NOT NULL DEFAULT '{}'::jsonb;
