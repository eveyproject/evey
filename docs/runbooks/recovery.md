# Recovery

Open this page when nobody can sign in any more, when the key that signs
access tokens may have leaked, or when the database has to go back to a
pre-migration dump. Each procedure is one `evey admin` command run inside the
container; the [command line](cli.md) page lists the other subcommands.

## Before you start

The commands read the same `EVEY_*` configuration as the server, so running
them in the server's container needs nothing else:

```sh
docker exec evey-server evey admin <subcommand>        # server running
docker compose run --rm evey admin <subcommand>        # container stopped
```

The second form boots the embedded database first, then runs the command and
exits. None of these commands starts the server.

## Give a user a new password

Use it when a user is locked out, or right after a suspected account
compromise. The new password is printed once and never logged: copy it before
closing the terminal.

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

A good result looks like this:

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

Every session of that user ends at once: the refresh tokens are deleted and
the access tokens revoked. An app that is open notices within about 30
seconds and asks to sign in again. If you lose the output, run the command
again.

## Rotate the key that signs access tokens

Use it when the key file may have leaked, or to sign every user out at once.
For a single account, **Admin -> Users -> Disable** or the password reset
above is enough.

```sh
docker exec evey-server evey admin rotate-jwt-key
```

What happens:

1. The key file (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key` by default) is
   renamed to `<path>.rotated-<UTC timestamp>`.
2. Every refresh token is deleted and every access token revoked, in one
   transaction. Open apps notice within about 30 seconds; everyone signs in
   again.
3. On its next start the server generates a fresh 2048-bit RSA key.

Restart the container afterwards (`docker compose restart evey`) so the new
key is in use. If the database could not be reached, the key is still moved
aside and the command says `key rotated, but sessions could not be ended
(restart the server, then run this again)`: do exactly that.

## Restore a pre-migration dump

Use it when an update has to be undone and the data written since can be
lost. The command feeds a `.sql.gz` dump through `gunzip | psql` into the
configured database.

!!! warning
    The restore drops and recreates each table before loading it, so the
    current rows are replaced. Take a backup of the current
    state first (**Admin -> Database Backup -> Run now**) if there is any
    chance you need it back.

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

What happens:

1. The command first reads the whole file: it must decompress cleanly and end
   with `pg_dump`'s completion line. A truncated or corrupt dump is refused
   (`refusing to restore: ...`) before anything is dropped.
2. It then waits 5 seconds before touching anything: Ctrl+C aborts.
3. `psql` applies the dump with `--single-transaction` and `ON_ERROR_STOP=1`.
   On any error the transaction is rolled back and the database is left as
   it was.
4. On success the last line is `[admin] restored backup <path>`.

Requirements:

- The file name must end in `.sql.gz`. The `.tar.gz` archives of **Admin ->
  Database Backup** are restored from the panel, not by this command.
- The target database must exist; the dump drops and recreates each table
  before loading it, so it can be restored onto a populated database, but it
  does not create the database.
- `psql` and `gunzip` must be on the PATH, which the image guarantees.

Pre-migration dumps are written automatically before each schema change,
under `EVEY_BACKUP_DIR` (`/data/backups`, so `./evey-data/backups` on the
host), named `pre-migration-<schema version>-<UTC timestamp>.sql.gz`. The
newest dump of each of the 3 most recent schema versions is kept, the others
are pruned. [Database migrations](migration-runbook.md) explains when to
restore one rather than step the schema back.

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete (whole gzip + completion marker before anything runs), 5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
