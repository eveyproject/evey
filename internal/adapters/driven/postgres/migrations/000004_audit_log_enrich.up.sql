ALTER TABLE audit_log
    ADD COLUMN IF NOT EXISTS user_agent  TEXT,
    ADD COLUMN IF NOT EXISTS device_name TEXT;
