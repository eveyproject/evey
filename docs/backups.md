# Backups and updates

A scheduled backup of the database, kept for as long as you choose, and an
update that is a `docker compose pull` away. This page is for the
administrator: what a backup holds and what it does not, how to restore one,
and how to update or go back without losing data.

## What a backup contains

A backup is a copy of the **database**: accounts, profiles, libraries and
their catalogue, watch history and resume points, collections, playlists and
favourites, settings, plugin records, webhooks, API keys, notifications and
audit logs. It is a `.tar.gz` holding the SQL dump and a manifest with the
dump's SHA256 and the schema version.

It does **not** contain:

- your media files;
- the transcoding cache (regenerated on demand);
- the search index (rebuilt from the database);
- the keys kept in `./evey-data`: `jwt.key`, which signs sessions, and the
  `keys` folder (database password, search key);
- the installed plugin files.

Playback and server metrics (the telemetry behind the quality and load
charts) are left out by default: they grow fastest and say nothing about your
catalogue. **Admin → Server Config → Include telemetry in backups** puts them
back in; the switch applies to manual, scheduled and command-line backups.
Restoring a backup always empties those tables.

A database backup therefore restores Evey's state on the same installation.
To be able to rebuild a machine, keep a copy of the whole `./evey-data` folder
as well: the database backups are inside it, in `./evey-data/backups`.

## Schedule backups

**Admin → Database Backup → Schedule**: a **Cron expression** (for example
`0 3 * * *`, every day at 3 AM) and a **Retention (days)**. Backups older than
the retention are deleted after each scheduled run. **Run now** makes one
immediately, as does **Create Backup** at the top of the page. From a script
or a cron job on the host, the command line makes the same backup:

```bash
docker exec evey-server evey admin backup
```

It follows the telemetry switch of the panel; `--include-telemetry` includes
the telemetry for that run only.

<figure class="evey-shot" markdown="span">
  ![Admin → Database Backup with a daily schedule](images/admin-backup.webp){ loading=lazy }
  <figcaption>A backup every day at 3 AM, kept 30 days.</figcaption>
</figure>

The server also dumps the database on its own before applying a migration
during an update, whatever the schedule (see [Updating Evey](#updating-evey)).

## Restore a backup

**Admin → Database Backup** lists the backups and restores one, or restores
from a file you upload (**Restore from File**). Restoring replaces the current
database.

!!! warning "Same schema only"
    The server checks the checksum and the schema version first, and refuses
    a backup taken by a version with a different schema instead of loading it.
    To go back to an older version with its data, see
    [Going back](#going-back).

When the server does not start any more, the command line restores the dumps
taken before a migration (`pre-migration-*.sql.gz`), not the `.tar.gz` backups
of this panel: see [Recovery](runbooks/recovery.md).

## Updating Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

That is the whole update. When the new version needs to change the database,
it first writes a dump to `./evey-data/backups/pre-migration-*.sql.gz` (the
three most recent are kept), then applies the migrations while starting; a
large catalogue can take a few minutes, and the container reports healthy once
it serves again. Evey asks the registry every hour whether a newer image is
published and says so in the admin panel; it never updates itself.

**Pin what you run.** During the beta there is no numbered release and no
`latest` tag; the branch tag `beta-0.0.1` moves with every change. For a server
you rely on, put an immutable `sha-<commit>` tag on the `image:` line, so that
two pulls a day apart run the same code. Once releases exist, `latest` will
follow the most recent stable one, and a release tag (`0.2.0`) will pin it.

**Which version is running.** `docker exec evey-server evey version` prints the
version and the commit of the running binary.

### Going back

Put the previous tag on the `image:` line and run `docker compose up -d`. If
the newer version had migrated the database, restore its pre-migration dump
first: `docker exec evey-server evey admin rollback` prints the exact steps
with the dump's path.

Image tags, runtime variants and GPU flavours are described in
[Advanced deployments](operations/advanced-deployments.md#image-tags-and-variants).

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (beta-0.0.1 + sha-<short>, latest gated on vX.Y.Z)
-->
