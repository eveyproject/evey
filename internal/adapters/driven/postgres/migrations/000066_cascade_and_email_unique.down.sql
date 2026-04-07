DROP INDEX IF EXISTS idx_users_email_unique;

ALTER TABLE invite_links DROP CONSTRAINT invite_links_created_by_fkey;
ALTER TABLE invite_links ADD CONSTRAINT invite_links_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);

ALTER TABLE share_links DROP CONSTRAINT share_links_created_by_fkey;
ALTER TABLE share_links ADD CONSTRAINT share_links_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);
