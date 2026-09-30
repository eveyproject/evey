# Database migrations

How Evey changes its database schema during an update, and what to do when it
goes wrong. The commands target the standard installation, where the database
runs inside the `evey-server` container.

## How migrations run

The migrations are embedded in the binary (`go:embed`) and applied by
`golang-migrate` when the server starts
(`internal/adapters/driven/postgres/migrate.go`, files
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`).
The state is the `schema_migrations` table: `{version, dirty}`.

Before applying anything, the server writes a full dump to
`/data/backups/pre-migration-<version>-<time>.sql.gz` (`EVEY_BACKUP_DIR`). The
newest dump of each of the 3 most recent schema versions is kept.

The same operations are available by hand:

```sh
docker exec evey-server evey admin migrate version   # exit 1 when the schema is dirty
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # exactly one step
```

`up` and `down` also take a dump first; `--no-backup` skips it.

To run SQL against the embedded database:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Before an update

1. **Take a backup** in **Admin → Database Backup** (**Run now**), or from a
   script:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   The automatic pre-migration dump is a second copy, not a replacement.

2. **For a large catalogue**, look at the size of the biggest tables: a
   migration that rewrites one of them takes longer.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Check the current version**: `evey admin migrate version`. If it reports
   a dirty schema, see [A migration that stopped halfway](#a-migration-that-stopped-halfway)
   first.

## Running the migrations

They run when the new image starts:

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

The container reports healthy once the migrations are applied and the server
answers:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## Going back

`evey admin rollback` prints the running version, the schema version, the
newest pre-migration dump and the exact commands to run on the host. It
changes nothing: the container has no access to Docker.

The usual way back is the previous image with the dump taken before the
update: see [Going back](../backups.md#going-back) in Backups and updates.
`evey admin migrate down` rolls back one step at a time instead, when the
`.down.sql` of that step is enough.

Do not edit `schema_migrations` by hand, except to clear a dirty state (below),
and take a dump first.

Restore the pre-migration dump rather than migrating down when the update
dropped data (a removed column, a type change).

## A migration that stopped halfway

If the log shows `Dirty database version N. Fix and force version.`, a
migration crashed partway. Bring the schema to the state you want, then clear
the flag:

```sql
SELECT version, dirty FROM schema_migrations;

-- Re-apply the rest of the up.sql by hand, or revert with the down.sql,
-- then record the version you actually reached:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Restart the container; the migrations continue from there. Restoring the
pre-migration dump is the other way out.

## Writing a migration

For contributors. Examples in `internal/adapters/driven/postgres/migrations/`:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Takes `ACCESS EXCLUSIVE` on both tables for the duration of the ADD. Cheap on small tables, painful on `media_items` / `watch_progress`. Pattern for large tables: add with `NOT VALID`, then `VALIDATE CONSTRAINT` in a second statement (validate only takes `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - plain `CREATE INDEX`. On large tables this locks writes. `CREATE INDEX CONCURRENTLY` cannot run inside a migrate transaction.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. On PG 11+ this is metadata-only and fast. Still holds `ACCESS EXCLUSIVE` briefly.

Guidelines:

- **Long ALTER** that rewrites the table (type change, adding a non-constant default on old PG) → expect downtime on a large catalogue.
- **DROP COLUMN** → only metadata, but the column cannot be restored without a backup. Never drop the same release a new version of the code is rolled out - prefer a two-step: stop reading the column, deploy, then drop in the next release.
- **FK on big tables** → `ADD CONSTRAINT ... NOT VALID` then `VALIDATE CONSTRAINT` in a follow-up migration. Avoid a single blocking `ADD CONSTRAINT ... FOREIGN KEY ...`.
- **Batch UPDATE / backfill** → never in a single migration on a big table. Ship a no-op schema migration + a background job.
- **Column replacement (expand → migrate → contract)** → do not fold `ADD COLUMN` + backfill + `SET NOT NULL` + `DROP COLUMN` of the old one into a single migration. That shape only works because Evey runs single-instance today, with migrations applied synchronously at boot: during the window, old code reading the dropped column and new code reading the new one cannot coexist, so a rolling or multi-replica deploy would serve errors from whichever half is behind.
  Known migrations written this way, kept as-is because they have already shipped: `000077` and `000098_playlists_profile_id.up.sql` (adds `profile_id`, backfills, sets `NOT NULL`, drops `user_id`).
  For a new one, split across three releases:
  1. **expand** - add the new column nullable, write to *both* columns, keep reading the old one;
  2. **migrate** - backfill the new column (background job, not the migration), then switch reads to it;
  3. **contract** - once no deployed code touches the old column, `SET NOT NULL` and drop it.
  Each step is independently deployable and reversible, which the one-file version is not.

## After an update

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Then clear the transcoding cache if the release changed how videos are encoded
(**Admin → Transcode Cache**).

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go ; internal/adapters/driven/postgres/migrate_backup.go - backupFilePrefix, pruneOldBackups
cli: cmd/evey/admin_commands.go (migrate, rollback) ; cmd/evey/admin_ops.go - adminRollback
embedded database: docker/entrypoint.sh (pg_user, pg_db, postgres.password, EVEY_DB_PASSWORD_FILE)
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
-->
