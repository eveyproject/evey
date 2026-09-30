# Recovery Runbook

CLI procedures for cases where you can no longer log in, the JWT key
is suspected compromised, or the database needs to be rolled back to a
pre-migration backup.

All commands run as `evey admin <subcommand>` and read configuration
from the same `EVEY_*` env vars as the server. They do **not** start
the server - most are safe to run while the server is stopped.

## reset-password

Generate a fresh password for an existing user. The new password is
printed once to stdout - capture it immediately.

```sh
evey admin reset-password --user admin@example.com
```

Output:
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

Notes:
- The user record's `updated_at` is bumped, but no other state changes.
- Existing sessions remain valid until their natural expiry. If you
  also want to log the user out everywhere, run `rotate-jwt-key`
  afterward.
- The plaintext is never logged. If you lose the output you have to
  run the command again.

## rotate-jwt-key

Force-invalidate every active session by replacing the RSA signing key.

```sh
evey admin rotate-jwt-key
```

Behavior:
- The current key file (path = `EVEY_JWT_PRIVATE_KEY_PATH`, default
  `/data/jwt.key`) is renamed to `<path>.rotated-<UTC-timestamp>`.
- On next server start, `LoadOrGenerateRSAKey` regenerates a fresh
  2048-bit RSA key.
- All previously-issued access tokens fail signature verification →
  every user is logged out.
- Refresh tokens are JWTs signed with the same key, so they're dead
  too - no separate cleanup needed.

When to use:
- Suspected key leak.
- Mass session-kill required (incident response, contractor offboard).

## restore-backup

Apply a `pg_dump.gz` produced by the pre-migration backup mechanism (or
any other source) into the configured database. **Overwrites existing
data.** The file must end in `.sql.gz`: the `.tar.gz` backups of
**Admin → Database Backup** are restored from the panel, not by this command.

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Procedure:
1. The command pauses for 5 seconds before doing anything - Ctrl+C
   aborts.
2. The file is streamed through `gunzip | psql` against the configured
   database, with `--single-transaction` and `ON_ERROR_STOP=1`.
3. On any error, psql rolls back the transaction - the database is
   left in its prior state.

Requirements:
- `psql` and `gunzip` must be on PATH (both ship in the production
  image alongside `pg_dump`).
- The target database must exist; the dump only contains DROP/CREATE
  for tables, not the database itself.
- The configured `EVEY_DB_USER` must own the target database.

Pre-migration backups are produced automatically before each
`migrate.Up()` and live under `EVEY_BACKUP_DIR` (default
`/data/backups`). The newest dump of each of the 3 most recent schema
versions is kept; the others are pruned.
