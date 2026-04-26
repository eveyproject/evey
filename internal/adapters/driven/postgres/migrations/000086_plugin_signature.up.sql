-- Persist cosign signature verification result so the security dashboard
-- survives server restarts. NULL for legacy rows and when CosignPolicy is "off".
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS signature JSONB;
