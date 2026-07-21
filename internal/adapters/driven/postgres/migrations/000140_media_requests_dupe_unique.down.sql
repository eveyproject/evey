-- The collapsed duplicate rows cannot be restored; only the constraint is.
DROP INDEX IF EXISTS idx_media_requests_dupe_unique;
