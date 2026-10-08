# Database migrations

How Evey changes its database schema during an update, what to check before
and after, and the way out when a migration stops halfway. For operators of
the standard install, where the database runs inside the `evey-server`
container; contributors writing a migration will find the rules at the end.

## How migrations run

The migrations are embedded in the binary and applied by `golang-migrate`
when the server starts. The state is the `schema_migrations` table, one row
`{version, dirty}`. Before applying anything, the server writes a full dump
to `EVEY_BACKUP_DIR` (`/data/backups`, so `./evey-data/backups` on the host)
named `pre-migration-<version>-<UTC timestamp>.sql.gz`, and keeps the newest
dump of each of the 3 most recent schema versions.

The same operations exist by hand:

```sh
docker exec evey-server evey admin migrate version   # exit 1 when the schema is dirty
docker exec evey-server evey admin migrate up        # apply every pending migration
docker exec evey-server evey admin migrate down      # roll back exactly one step
```

`up` and `down` take a dump first; `--no-backup` skips it. To open a SQL
shell on the embedded database:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Update safely

1. **Take a backup** in **Admin -> Database Backup** (**Run now**), or from
   a script:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   The automatic pre-migration dump is a second copy, not a replacement.

2. **Check the current version**: `docker exec evey-server evey admin migrate
   version` prints `schema version: N`. If it prints `DIRTY`, go to
   [A migration that stopped halfway](#a-migration-that-stopped-halfway)
   before updating.

3. **On a large catalogue**, look at the biggest tables: a migration that
   rewrites one of them takes longer, and the container healthcheck allows
   300 seconds of boot for that.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **Start the new image** and watch the log until the migrations are
   applied and the server answers:
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   A good result: a `migrations: pre-migration backup written` line when
   there was something to apply, then the server banner, then
   `curl -sf http://localhost:9091/healthz` answers.

5. **Afterwards**, confirm the version and look for errors:
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   Clear the transcoding cache (**Admin -> Transcode Cache -> Clear All**)
   if the release changed how videos are encoded.

## Go back to the previous version

`docker exec evey-server evey admin rollback` prints the running version,
the schema version, the newest pre-migration dump and the exact commands to
run on the host. It changes nothing: the container has no access to Docker.

The usual way back is the previous image plus the dump taken before the
update: see [Going back](../backups.md#going-back). Restore the dump rather
than migrating down when the update dropped data (a removed column, a type
change). `evey admin migrate down` steps back one migration at a time
instead, when that step's `.down.sql` is enough.

!!! warning
    Do not edit `schema_migrations` by hand except to clear a dirty state
    (below), and take a dump first.

## A migration that stopped halfway

When the log shows `Dirty database version N. Fix and force version.`, a
migration crashed partway and `golang-migrate` refuses to continue. Bring the
schema to the state you want, then clear the flag:

```sql
SELECT version, dirty FROM schema_migrations;

-- Re-apply the rest of the up.sql by hand, or revert with the down.sql,
-- then record the version you actually reached:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Restart the container; the migrations continue from there. Restoring the
pre-migration dump ([Recovery](recovery.md#restore-a-pre-migration-dump)) is
the other way out.

## Writing a migration

For contributors. The files live in
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.
Examples already shipped:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Takes `ACCESS EXCLUSIVE` on
  both tables for the duration of the ADD. Cheap on small tables, painful on
  `media_items` / `watch_progress`.
- **`000067_sort_title_index.up.sql`**,
  **`000068_watch_progress_composite_index.up.sql`**,
  **`000069_quick_connect_shortcode_index.up.sql`** - plain `CREATE INDEX`,
  which locks writes on large tables. `CREATE INDEX CONCURRENTLY` cannot run
  inside a migrate transaction.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT
  NULL DEFAULT '[]'`. Metadata-only on PG 11+, but still holds `ACCESS
  EXCLUSIVE` briefly.

Guidelines:

- **Long ALTER** that rewrites the table (type change, non-constant default on
  an old PG) means downtime on a large catalogue.
- **DROP COLUMN** is metadata-only, but the column cannot come back without
  a backup. Never drop it in the release that stops reading it: stop
  reading, deploy, drop in the next release.
- **FK on big tables**: `ADD CONSTRAINT ... NOT VALID`, then `VALIDATE
  CONSTRAINT` in a follow-up migration (validate only takes `SHARE UPDATE
  EXCLUSIVE`).
- **Batch UPDATE / backfill**: never in a single migration on a big table.
  Ship a no-op schema migration plus a background job.
- **Column replacement (expand, migrate, contract)**: do not fold `ADD
  COLUMN` + backfill + `SET NOT NULL` + `DROP COLUMN` into one file. That
  shape only works because Evey runs single-instance, with migrations applied
  synchronously at boot; a rolling deploy would serve errors from whichever
  half is behind. `000077` and `000098_playlists_profile_id.up.sql` were
  written this way and stay as they are because they have shipped. For a new
  one, split across three releases:
  1. **expand** - add the new column nullable, write both columns, keep
     reading the old one;
  2. **migrate** - backfill the new column (background job, not the
     migration), then switch reads to it;
  3. **contract** - once no deployed code touches the old column, `SET NOT
     NULL` and drop it.

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/en/admin.yaml - sidebar.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
