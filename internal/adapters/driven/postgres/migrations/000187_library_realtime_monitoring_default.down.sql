-- The default goes back to FALSE. The values the up migration switched on are
-- left as they are: by now the operator may have chosen them.
ALTER TABLE libraries ALTER COLUMN realtime_monitoring SET DEFAULT FALSE;
