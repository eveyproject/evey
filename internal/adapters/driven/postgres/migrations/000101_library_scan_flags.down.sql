ALTER TABLE libraries
    DROP COLUMN IF EXISTS scan_on_startup,
    DROP COLUMN IF EXISTS realtime_monitoring;
