-- Audit entries record WHICH ACCOUNT acted but never which profile. On a
-- household account ("Papa", "Les enfants") that is the half of the identity
-- the admin actually cares about, so the column is persisted with the entry
-- rather than resolved later: a profile can be renamed or deleted.
ALTER TABLE audit_log
    ADD COLUMN IF NOT EXISTS profile_id TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_log_profile_id ON audit_log(profile_id);
