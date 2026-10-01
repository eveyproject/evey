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
- Every session of the user ends: its refresh tokens are deleted and its
  access tokens revoked. An app that is open notices within about 30
  seconds and asks to sign in again.
- The plaintext is never logged. If you lose the output you have to
  run the command again.

## rotate-jwt-key

Replace the RSA key that signs access tokens.

```sh
evey admin rotate-jwt-key
```

Behavior:
- The current key file (path = `EVEY_JWT_PRIVATE_KEY_PATH`, default
  `/data/jwt.key`) is renamed to `<path>.rotated-<UTC-timestamp>`.
- On next server start, `LoadOrGenerateRSAKey` regenerates a fresh
  2048-bit RSA key.
- Every session ends at once, without waiting for the restart: the
  command deletes every refresh token and revokes every access token in
  the database. Open apps notice within about 30 seconds and everyone
  signs in again.
- If the database cannot be reached, the key is still moved aside and the
  command says so: run it again once the database answers.

When to use:
- Suspected key leak: nobody can mint valid access tokens with the old
  key any more.
- Mass session kill. For one account, disabling it in **Admin → Users**,
  or `reset-password`, is enough.

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

<!-- sources
reset-password / rotate-jwt-key end sessions: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), endAllSessions ; epoch cache TTL 30 s: internal/app/user/token_epoch.go
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser (DeleteByUserID + revokeTokens)
pre-migration retention: internal/adapters/driven/postgres/migrate_backup.go - pruneOldBackups
-->
