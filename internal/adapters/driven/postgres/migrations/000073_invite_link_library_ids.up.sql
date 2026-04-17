ALTER TABLE invite_links
  ADD COLUMN IF NOT EXISTS library_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
