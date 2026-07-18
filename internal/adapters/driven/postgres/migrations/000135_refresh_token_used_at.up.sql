-- used_at stamps a refresh token's rotation. It bounds the rotation grace
-- window: a rotated token presented again shortly after (a multi-tab race
-- loser sharing the HttpOnly cookie) is granted a sibling session instead of
-- a tab-killing 401; past the grace it is a replay and stays rejected.
ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ;
