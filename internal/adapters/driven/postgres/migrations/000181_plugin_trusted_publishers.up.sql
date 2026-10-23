-- Community plugin signing: the signing key travels with the package, its
-- fingerprint is pinned on the plugin at install (an update must carry the same
-- key) and the instance keeps a list of publisher keys it trusts.
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS publisher_key_fingerprint TEXT;
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS publisher_key_pem TEXT;

CREATE TABLE IF NOT EXISTS plugin_trusted_publishers (
    fingerprint    TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    public_key_pem TEXT NOT NULL,
    seeded         BOOLEAN NOT NULL DEFAULT FALSE,
    added_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
