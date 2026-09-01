DROP INDEX IF EXISTS idx_audit_log_profile_id;
ALTER TABLE audit_log DROP COLUMN IF EXISTS profile_id;
