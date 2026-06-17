-- Defense-in-depth against a duplicate-user race in OIDC findOrCreateUser:
-- two concurrent first-logins for the same federated identity both miss
-- GetByOIDCSubject and both INSERT, leaving two users sharing one identity
-- (previously blocked only INCIDENTALLY by the username-unique constraint).
-- A partial unique index makes the second INSERT fail (23505) so the service
-- can recover by re-reading the winner. Partial (WHERE oidc_subject IS NOT NULL)
-- so the many local users with a NULL oidc_subject never collide with each other.
--
-- Dedup any pre-existing duplicate identities FIRST (mirrors 000111): this index
-- exists because the race CAN already have happened, so an instance that hit it
-- has two users sharing one identity, and a bare CREATE UNIQUE INDEX would fail
-- (23505) and leave the deploy stuck. Keep the oldest user row; the second
-- concurrent first-login is a phantom (only an auto-created default profile, no
-- history yet), so cascading it away is the correct resolution.
DELETE FROM users a USING users b
 WHERE a.oidc_subject IS NOT NULL
   AND a.oidc_issuer = b.oidc_issuer
   AND a.oidc_subject = b.oidc_subject
   AND (a.created_at > b.created_at
        OR (a.created_at = b.created_at AND a.id > b.id));

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc_identity
    ON users (oidc_issuer, oidc_subject)
    WHERE oidc_subject IS NOT NULL;
