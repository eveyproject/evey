ALTER TABLE media_items ADD COLUMN IF NOT EXISTS scan_title TEXT;

-- Backfill: for items that have never been overwritten by enrichment, title
-- IS the scan title. Imperfect for items already enriched (their title is
-- the provider's, not the folder's) — those keep their current title as
-- best-effort and can be reset via admin "Reset metadata".
UPDATE media_items SET scan_title = title WHERE scan_title IS NULL AND title <> '';
