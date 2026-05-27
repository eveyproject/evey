-- scan_on_startup and realtime_monitoring were accepted by the API and carried
-- through the domain model but never persisted (no column, absent from every
-- library_repo SQL), so they reset to false on every restart and the realtime
-- watcher never armed for any library. Add the backing columns. Default FALSE
-- preserves the current effective behaviour for existing rows; new values set
-- via Create/Update now stick.
ALTER TABLE libraries
    ADD COLUMN IF NOT EXISTS scan_on_startup     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS realtime_monitoring BOOLEAN NOT NULL DEFAULT FALSE;
