-- share_links: CASCADE on created_by
ALTER TABLE share_links DROP CONSTRAINT share_links_created_by_fkey;
ALTER TABLE share_links ADD CONSTRAINT share_links_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;

-- invite_links: CASCADE on created_by
ALTER TABLE invite_links DROP CONSTRAINT invite_links_created_by_fkey;
ALTER TABLE invite_links ADD CONSTRAINT invite_links_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;

-- users.email: partial unique (skip empty for OIDC users)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique ON users(email) WHERE email != '';
