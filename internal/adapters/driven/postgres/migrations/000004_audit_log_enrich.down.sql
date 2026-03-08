ALTER TABLE audit_log
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS device_name;
