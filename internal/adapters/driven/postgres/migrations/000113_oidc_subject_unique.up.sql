-- Defense-in-depth against a duplicate-user race in OIDC findOrCreateUser:
-- two concurrent first-logins for the same federated identity both miss
-- GetByOIDCSubject and both INSERT, leaving two users sharing one identity
-- (previously blocked only INCIDENTALLY by the username-unique constraint).
-- A partial unique index makes the second INSERT fail (23505) so the service
-- can recover by re-reading the winner. Partial (WHERE oidc_subject IS NOT NULL)
-- so the many local users with a NULL oidc_subject never collide with each other.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc_identity
    ON users (oidc_issuer, oidc_subject)
    WHERE oidc_subject IS NOT NULL;
