ALTER TABLE profiles
    ADD COLUMN parental_max_rating   TEXT NOT NULL DEFAULT '',
    ADD COLUMN parental_blocked_tags TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN parental_pin_hash     TEXT NOT NULL DEFAULT '',
    ADD COLUMN parental_enabled      BOOLEAN NOT NULL DEFAULT FALSE;
