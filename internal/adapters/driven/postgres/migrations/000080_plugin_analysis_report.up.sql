-- Persist the static analysis output so admins can audit non-critical
-- warnings post-install. NULL for legacy rows (no report available).
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS analysis_report JSONB;
