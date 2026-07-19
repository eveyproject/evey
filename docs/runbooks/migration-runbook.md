# Migration Runbook

How Evey runs schema migrations and how to operate on them safely.

## How migrations run

Migrations are **embedded** (`go:embed`) and executed automatically on server startup from `internal/adapters/driven/postgres/migrate.go` using `golang-migrate/migrate/v4`. Source: `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.

Migrations apply automatically when `evey-server` boots. There is **also** a CLI for manual control: `evey admin migrate up | down | version` (implemented in `cmd/evey/admin.go`; `version` prints the current schema version and exits 1 if the DB is dirty). The `Makefile` `migrate-up` / `migrate-down` targets wrap it via `./cmd/evey admin migrate`. A pre-migration `pg_dump` is taken automatically when `EVEY_BACKUP_DIR` is set (pass `--no-backup` to skip).

State is tracked in the `schema_migrations` table (managed by golang-migrate): `{version, dirty}`.

## Pre-migration checklist

1. **Backup the database.** Evey has a built-in backup endpoint (see `docs/backups.md`):
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Or run `pg_dump` directly against the container:
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   Verify the dump size is non-zero and `gunzip -t` passes.

2. **Check table sizes** for any migration touching large tables (media_items, watch_progress, tracks, hls_sessions):
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Check active sessions / locks** before a DDL-heavy migration:
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   Kill long-running or idle-in-transaction sessions (`SELECT pg_terminate_backend(pid)`) before running a migration that takes strong locks.

4. **Check current migration version**:
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   If `dirty = true`, follow the Recovery section before doing anything else.

## Running a migration

Migrations run on server start. To apply pending migrations:

```sh
docker compose pull evey       # if deploying a new image
docker compose up -d evey
docker logs -f evey-server     # watch the startup log
```

Successful run: no migrate errors, server reaches `/healthz` 200. Verify:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## Rollback

There is no one-shot `migrate-down` target wired up. To roll back manually you must either:

- **Redeploy the previous image** whose embedded migrations stop at version N-1, or
- Apply the `.down.sql` by hand:
  ```sh
  docker exec -i evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
    < internal/adapters/driven/postgres/migrations/000NNN_<name>.down.sql
  docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
    -c "UPDATE schema_migrations SET version = <N-1>, dirty = false;"
  ```

Always restore from the pre-migration backup if data-destructive changes shipped (DROP COLUMN, type changes) and the backup is fresher than the production traffic divergence.

## Recovery - dirty state

If startup logs show `Dirty database version N. Fix and force version.` the last migration crashed halfway. Fix the schema manually to match the state you want, then clear the flag:

```sql
-- Inspect: what does schema_migrations currently report?
SELECT version, dirty FROM schema_migrations;

-- Either re-apply the up.sql statements by hand, or revert via down.sql.
-- Then clear the dirty bit at the version you actually reached:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Restart `evey-server`. golang-migrate will continue from there.

## Special cases

Recent migrations for reference (see `internal/adapters/driven/postgres/migrations/`):

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Takes `ACCESS EXCLUSIVE` on both tables for the duration of the ADD. Cheap on small tables, painful on `media_items` / `watch_progress`. Pattern for large tables: add with `NOT VALID`, then `VALIDATE CONSTRAINT` in a second statement (validate only takes `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - plain `CREATE INDEX`. On large tables this locks writes. Consider `CREATE INDEX CONCURRENTLY` manually during a maintenance window (cannot run inside a migrate transaction - apply out-of-band then record the version).
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. On PG 11+ this is metadata-only and fast. Still holds `ACCESS EXCLUSIVE` briefly.

Guidelines:

- **Long ALTER** that rewrites the table (type change, adding a non-constant default on old PG) → pre-lock check mandatory, expect downtime.
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

## Post-migration checks

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Smoke test a few endpoints the migration affects (`/api/v1/...`).
