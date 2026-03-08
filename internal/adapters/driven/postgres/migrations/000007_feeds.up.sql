CREATE TABLE IF NOT EXISTS feed_admin_config (
    feed_type     TEXT PRIMARY KEY,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    display_order INT NOT NULL DEFAULT 0,
    max_items     INT NOT NULL DEFAULT 20,
    params        JSONB NOT NULL DEFAULT '{}'
);
INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items) VALUES
    ('continue_watching', true, 0, 20),
    ('recently_added',    true, 1, 20),
    ('recommended',       true, 2, 20)
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS feed_profile_config (
    profile_id    TEXT NOT NULL,
    feed_type     TEXT NOT NULL,
    enabled       BOOLEAN,
    display_order INT,
    max_items     INT,
    params        JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (profile_id, feed_type)
);
