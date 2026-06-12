-- token_epoch is a per-user revocation counter embedded in access-token JWTs.
-- Bumping it (disable, role change, password change/reset, admin session kill)
-- invalidates every outstanding access token of the user within the validator
-- cache TTL (~30s) instead of waiting out the full token expiry.
ALTER TABLE users ADD COLUMN token_epoch BIGINT NOT NULL DEFAULT 0;
