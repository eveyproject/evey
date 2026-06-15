-- Index every ON DELETE CASCADE foreign-key column that lacked a leading index.
--
-- Without an index on the child's FK column, Postgres full-scans the child table
-- once PER deleted parent row to find the rows to cascade. Deleting a library
-- cascades through media_items (9000+ on a real instance), and for each one it
-- was seq-scanning watch_progress / favorites / transcode_sessions / share_links
-- (and music_track_plays for music libs) — O(parents × child_rows) — which blew
-- the 30s statement timeout, so the whole DELETE rolled back and the library
-- "wouldn't delete". The same pathology hit user/profile/role deletes
-- (refresh_tokens, user_roles, syncplay_rooms, plugin_data). A plain btree on
-- each FK column turns the cascade into index lookups.
--
-- Note: existing composite indexes (e.g. watch_progress(profile_id, media_item_id))
-- do NOT help — the cascade looks up by media_item_id, which is not the leading
-- column there.

-- Library-delete cascade path (library → media_items / music_tracks / library).
CREATE INDEX IF NOT EXISTS idx_watch_progress_media_item_id    ON watch_progress (media_item_id);
CREATE INDEX IF NOT EXISTS idx_favorites_media_item_id         ON favorites (media_item_id);
CREATE INDEX IF NOT EXISTS idx_transcode_sessions_media_item_id ON transcode_sessions (media_item_id);
CREATE INDEX IF NOT EXISTS idx_share_links_media_item_id       ON share_links (media_item_id);
CREATE INDEX IF NOT EXISTS idx_library_access_library_id       ON library_access (library_id);
CREATE INDEX IF NOT EXISTS idx_music_track_plays_track_id      ON music_track_plays (track_id);

-- Same class on user / profile / role deletes.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_profile_id       ON refresh_tokens (profile_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id             ON user_roles (role_id);
CREATE INDEX IF NOT EXISTS idx_syncplay_rooms_host_id          ON syncplay_rooms (host_id);
CREATE INDEX IF NOT EXISTS idx_plugin_data_user_id            ON plugin_data (user_id);
