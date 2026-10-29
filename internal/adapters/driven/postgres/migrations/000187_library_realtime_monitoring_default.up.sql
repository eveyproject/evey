-- Real-time monitoring becomes the default. No library could have it before:
-- the web panel offered no switch and an update could not change it, so every
-- existing FALSE is the old impossibility, not an operator's choice. Without
-- it an added or deleted file waits for the next periodic walk; with it that
-- walk drops to an hourly backstop. A watch that cannot work (network share,
-- inotify limit) is caught by the liveness probe, which keeps the periodic
-- walk for that library.
ALTER TABLE libraries ALTER COLUMN realtime_monitoring SET DEFAULT TRUE;
UPDATE libraries SET realtime_monitoring = TRUE WHERE NOT realtime_monitoring;
