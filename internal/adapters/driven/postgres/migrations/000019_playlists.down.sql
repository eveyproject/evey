DROP TABLE IF EXISTS playlist_items;
DROP TABLE IF EXISTS playlists;
-- The 000098 rollback stashes per-profile playlist ownership so a later re-up
-- is lossless; once the playlists table itself goes, the stash is meaningless.
DROP TABLE IF EXISTS _mig000098_playlist_profiles;
