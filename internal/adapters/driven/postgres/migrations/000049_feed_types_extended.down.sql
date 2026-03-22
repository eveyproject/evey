DELETE FROM feed_profile_config WHERE feed_type IN ('resume_playback', 'next_episode', 'collections', 'most_popular');
DELETE FROM feed_admin_config WHERE feed_type IN ('resume_playback', 'next_episode', 'collections', 'most_popular');

INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items) VALUES
    ('continue_watching', true, 0, 20),
    ('up_next', true, 1, 15),
    ('recently_added', true, 2, 20)
ON CONFLICT DO NOTHING;
