-- Login by username is case-insensitive from here on (prod, 2026-09: three
-- viewers typed their own name in lowercase and were told the account did not
-- exist, one of them locked out after five tries). The lookup compares
-- lower(username), so two accounts differing only by case would be ambiguous:
-- this index makes them impossible.
--
-- An instance that already holds such a pair must not be left with a deploy
-- stuck on 23505, and no account is deleted: the younger row of the pair is
-- renamed with a short id suffix (the operator sees it in the user list and
-- can rename it properly); the oldest keeps its name.
UPDATE users a
   SET username = a.username || '_' || left(a.id, 6)
  FROM users b
 WHERE lower(a.username) = lower(b.username)
   AND a.id <> b.id
   AND (a.created_at > b.created_at
        OR (a.created_at = b.created_at AND a.id > b.id));

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower
    ON users (lower(username));
