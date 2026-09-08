# Evey CLI - `evey admin`

The Evey server binary doubles as the operator CLI. When `evey` is invoked
without arguments, it boots the HTTP/gRPC server; with `admin` as the first
argument, it switches into one-shot operator mode and exits when the
command completes.

All commands read configuration from the same `EVEY_*` env vars as the
server itself (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...).

## Quick reference

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key (kills ALL sessions).
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line.
```

### Media maintenance

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one - the fix for slow seek and slow resume.
                                             DRY-RUN BY DEFAULT: pass --apply to rewrite. Each
                                             rewrite is validated, cross-checked, and keeps a
                                             .mkvgobak backup. --limit N for a cautious first run.
                                             --resync also repairs files with a corrupted region the
                                             strict pass refuses (skips the damage, keeps the rest,
                                             reports the lost bytes).
                                             LONG: rewrites each file in full, hours on a large
                                             library. Shows a progress bar while applying.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness
                                             (Cues index, keyframe cues, DefaultDuration) - what the
                                             on-demand HLS rung and seeking rely on.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
evey admin reclassify <media-id>             Force-recompute the encoding complexity class.
evey admin reclassify --all                  Wipe and re-queue every classified item.
```

### Diagnostics and capacity

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (owned by the server UID?),
                                             cache dirs, ffmpeg/mkvgo and disk. --repair applies only
                                             the safe fixes (create + chown missing writable dirs).
evey admin rollback                          Read-only advisor: prints the running version, schema,
                                             newest pre-migration backup and the exact HOST commands
                                             to roll back. The container has no Docker access, by
                                             design, so it advises rather than acts.
evey admin load-test [minimal|standard|max] [--scenario <s>] [--max-viewers N] [--duration S]
                                             Needs a LIVE server (it is an HTTP client of the local
                                             gateway). Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count with TTFF and
                                             segment latencies. Scenarios: transcode, natural, mkvgo,
                                             light (80/20), balanced (50/50, default), heavy (20/80),
                                             mkvgo+N. --max-viewers caps the ramp (0 = server default
                                             12). Short sanity run:
                                             `load-test --scenario balanced --max-viewers 4`.
                                             `admission_full` is the ceiling verdict (exit 0), not an
                                             error.
```

> The full, always-current list is `evey admin` with no subcommand - it prints
> 2026-07-23; `reindex-no-cue`, the seek-index repair behind the recurring slow-
> resume incidents, was among the missing ones.

## How to invoke

### From the host with the stack running (Docker dev / production)

The Makefile wraps the most common ops in container-shell calls so they
work without sourcing `.env` and without pg_dump on the host:

```sh
make migrate-version
make migrate-up
make migrate-down
```

These shell out to `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'`,
so `evey-server` must be up - start it with `make dev` first if it isn't.

### Direct shell into the running container

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### On a bare-metal install (no Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

The pre-installed binary already has every subcommand built in.

## Subcommand details

### `migrate`

- `up` applies every pending migration in order. A pre-migration `pg_dump`
  is taken automatically when `EVEY_BACKUP_DIR` is set (default in the
  Docker image: `/data/backups`). Failure mid-Up leaves
  `schema_migrations.dirty = true`; see `migration-runbook.md`.
- `down` rolls back exactly one migration. Surgical, not "tear it all
  down" - there's no `down --all`. Also takes a pre-rollback `pg_dump`.
- `version` prints the current schema version. Returns exit code 1 with a
  `DIRTY` warning when a previous migration failed halfway (i.e. some
  but not all `Up` statements ran).
- `--no-backup` skips the pg_dump for either direction. Useful on hosts
  without `postgres-client` installed (typical WSL dev path where the
  Makefile already passes the flag).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

See `docs/runbooks/recovery.md` for the full procedure, prerequisites,
and post-conditions.

## What's *not* in `evey admin`

Three standalone binaries live next to `cmd/evey` because they're not
operator commands in the recovery / schema sense:

| Binary | Purpose | Why standalone |
|---|---|---|
| `cmd/intro-backfill` | Refresh intro detection on a slice of the catalog after a pipeline change | **API client** - talks to `/api/v1/admin/media/{id}/detect-segments` with a bearer token, not in-process. |
| `cmd/intro-diag` | Diagnose why pHash intro detection matched (or didn't) on a specific series | **Developer debug** tool - reads trickplay sprites off disk, prints a frame-by-frame Hamming match map. Optional audio cross-correlation pass. |
| `cmd/trickplay-verify` | Verify that each trickplay tile actually shows the frame at the recorded timestamp | **QA/CI** tool - re-extracts source frames via ffmpeg and aHashes them against the tile. |

They're shipped as separate binaries because none of them needs the full
server config plumbing - they're scoped, run-once tools, not part of the
recovery surface.

## Related runbooks

- `migration-runbook.md` - schema deploy, rollback, dirty-state recovery.
- `recovery.md` - password reset, JWT key rotation, backup restore, search reindex.
- `incident-runbook.md` - first-response when the server is misbehaving.
