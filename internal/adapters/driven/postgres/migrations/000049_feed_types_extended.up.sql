-- Replace continue_watching + up_next with resume_playback + next_episode
-- Remove recently_added (handled by library sliders)
DELETE FROM feed_profile_config WHERE feed_type IN ('continue_watching', 'up_next', 'recently_added');
DELETE FROM feed_admin_config WHERE feed_type IN ('continue_watching', 'up_next', 'recently_added');

INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items) VALUES
    ('resume_playback', true,  0, 20),
    ('next_episode',    true,  1, 15),
    ('collections',     true,  2, 20),
    ('recommended',     true,  3, 20),
    ('most_popular',    true,  4, 20)
ON CONFLICT DO NOTHING;
