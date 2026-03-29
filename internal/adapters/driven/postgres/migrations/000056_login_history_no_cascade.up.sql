ALTER TABLE login_history
    DROP CONSTRAINT IF EXISTS login_history_user_id_fkey;

ALTER TABLE login_history
    ADD CONSTRAINT login_history_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE login_history
    ALTER COLUMN user_id DROP NOT NULL;
