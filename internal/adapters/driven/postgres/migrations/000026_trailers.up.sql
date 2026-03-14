ALTER TABLE media_items ADD COLUMN trailers jsonb NOT NULL DEFAULT '[]'::jsonb;
