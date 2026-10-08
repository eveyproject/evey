# Command line

The server binary doubles as the operator's command line: `evey admin
<subcommand>` runs one maintenance or recovery tool against the same
configuration and database as the server, then exits. This page lists the
commands and how to run them in the container; the day-to-day procedures
(lost password, key rotation, restore) are in [Recovery](recovery.md).

## Running a command

In the standard installation, the commands run inside the container, which
already carries the database password and the `EVEY_*` configuration:

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # an admin shell: evey admin ..., man evey
```

On an install without Docker, the binary is called directly
(`evey admin reset-password --user alice@example.com`); it reads the same
`EVEY_*` variables as the server (`EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_PASSWORD`, `EVEY_DB_NAME`, `EVEY_BACKUP_DIR`...). Most commands need
the database; none starts the server, and most are safe to run while it is
stopped.

| Invocation | What it does |
| --- | --- |
| `evey` | Starts the server. |
| `evey admin <subcommand>` | Runs one operator tool and exits. `evey admin` alone prints the list and exits 1; `evey admin <subcommand> --help` details one. |
| `evey version` | Version, commit, build date. Needs no configuration or database, so it answers from a container that will not start. |
| `evey help` | The command summary. |
| `evey man [--text]` | The manual: roff for `man`, plain text with `--text`. |
| `evey completion bash` | The bash completion script. |

Any other first argument is refused (exit 1) rather than silently booting the
server.

## The commands

The list below is the binary's own `--help` text, grouped as `evey admin`
prints it. `evey admin` with no subcommand is always the current list.

### Recovery

```
evey admin reset-password --user <email>     Generate a new password for the user, print it once.
                                             Every session of that user is ended at the same time.
evey admin rotate-jwt-key                    Regenerate the JWT signing key. Invalidates ALL active
                                             sessions: the key is moved aside (kept as
                                             <key>.rotated-<timestamp>), the next server boot
                                             generates a new one, and every session is ended in the
                                             database at once.
evey admin restore-backup <path-to-dump.sql.gz>
                                             Restore the database from a pg_dump.gz file, in a single
                                             transaction. Existing rows are OVERWRITTEN; waits 5 s for
                                             a Ctrl-C before it starts. The .sql.gz suffix is required
                                             (not the panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]
                                             Rebuild the Meilisearch index from the database (no
                                             metadata fetch). --purge clears the index first, which
                                             drops orphan documents. If Meilisearch is down, search
                                             is served by the Postgres fallback: the command reports
                                             that and does nothing.
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

`reset-password` and `rotate-jwt-key` both end sessions: the user's for the
first, everyone's for the second. Open apps notice within about 30 seconds and
ask to sign in again. The full procedures are in [Recovery](recovery.md).

### Schema

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

A pre-migration `pg_dump` is taken for `up` and `down` when `EVEY_BACKUP_DIR`
is set (`/data/backups` in the Docker image); `--no-backup` skips it, for hosts
without `pg_dump`. A failure mid-`up` leaves `schema_migrations.dirty = true`:
see [Migration runbook](migration-runbook.md). On the development stack, the
Makefile wraps these three as `make migrate-up`, `make migrate-down` and
`make migrate-version`, run in the `evey-server` dev container.

### Media maintenance

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--mode auto|inplace|rewrite] [--deep-verify] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one, fixing slow resume. DRY-RUN BY DEFAULT: pass
                                             --apply to rewrite. Each rewrite is validated and
                                             cross-checked and keeps a .mkvgobak backup; --limit N for
                                             a cautious first run; --keep-backup=false drops the
                                             backups and enforces --deep-verify instead. --mode auto
                                             takes the surgical in-place patch when the file layout
                                             allows it and rewrites otherwise; inplace patches only
                                             (no backup); rewrite always rebuilds and swaps.
                                             --resync repairs files with a CORRUPTED region the strict
                                             pass refuses: it skips the damage, keeps the rest and
                                             reports the lost bytes. LONG: rewrites each file in full
                                             (hours on a large library); shows a progress bar.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness: a
                                             missing Cues index, cue points on a non-video track, cue
                                             times matching no keyframe, video tracks without a
                                             DefaultDuration. A file with errors falls back to the
                                             ffmpeg pipeline. --errors-only skips the warnings.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
```

The same engine runs behind **Admin → Media Health**, which is the easier way
to repair a library: see [Media health](media-health.md).

### Per-title encoding

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### Diagnostics

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (can the server write each
                                             library root?), cache dirs, ffmpeg/mkvgo, trusted
                                             proxies, remote access and disk. One line per check
                                             (ok / WARN / FAIL) with a remediation hint; exits 1 when
                                             a check fails. --repair applies only the safe fixes
                                             (create and chown the missing writable dirs).
evey admin rollback                          Read-only advisor: running version, schema, newest
                                             pre-migration backup and the exact HOST commands to roll
                                             back. The container has no Docker access, by design, so
                                             it advises rather than acts.
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights 1080,2160] [--fps 24,30,50,60] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the cost model.
                                             Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...
                                             Encode a few scenes of each title at fixed CRFs and
                                             score encode cost against the ladder's complexity
                                             features. Runs ffmpeg for minutes; never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Run the per-title probe on the given files: probe time,
                                             early-stop rate and points. Reads and encodes only.
```

### Capacity

```
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
                                             Needs a LIVE server: it is an HTTP client of the local
                                             gateway. Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count and latencies.
                                             Scenarios: transcode, natural, mkvgo, light (80/20),
                                             balanced (50/50, default), heavy (20/80), mkvgo+N.
                                             Modes: ramp (default), flash, seek, endurance. Presets:
                                             minimal (4 viewers), standard (12), max (32); an explicit
                                             --max-viewers wins. --cold-cache keeps evicting the
                                             library from the page cache so the run measures the
                                             storage, not the RAM. admission_full is the ceiling
                                             verdict (exit 0), not an error. Short sanity run:
                                             load-test --scenario balanced --max-viewers 4
```

### Completion sources

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## Tab-completion and the manual

The binary carries its own bash completion and manual, so both always match
the subcommands it ships:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh users: `autoload -U +X bashcompinit && bashcompinit` first, then source the
same script. Completion covers the subcommands, their flags and flag values
(`--mode`, `--backend`, `--scenario`), user emails after `--user`, the library
roots the server knows after `--root` (plus any directory), media files only
for the file arguments of `ladder-bench`, `ladder-probe` and `route-bench`, and
`.sql.gz` dumps for `restore-backup`.

The Docker image installs the completion at build time, and its admin shell
(`docker exec -it evey-server bash`) answers `man evey` by paging
`evey man --text`, since the image ships no `man` binary.

## What is not in `evey admin`

Three standalone programs live next to the server in the source tree. They
are developer and QA tools, not operator commands, and the Docker image does
not ship them:

| Program | Purpose |
| --- | --- |
| `cmd/intro-backfill` | Refresh intro detection on a slice of the catalogue after a pipeline change. An API client with a bearer token, not in-process. |
| `cmd/intro-diag` | Explain why intro detection matched, or did not, on one series, from the trickplay sprites on disk. |
| `cmd/trickplay-verify` | Check that each trickplay tile shows the frame at its recorded timestamp, by re-extracting the source frame with ffmpeg. |

## Related runbooks

- [Recovery](recovery.md): password reset, JWT key rotation, backup restore.
- [Migration runbook](migration-runbook.md): schema deploy, rollback, dirty-state recovery.
- [Incident runbook](incident-runbook.md): first response when the server misbehaves.

<!-- sources
dispatch and top-level commands: cmd/evey/cli.go - runCLI (version, help, completion, man, unknown command refused), cmd/evey/main.go
subcommand table, groups, --help text: cmd/evey/admin_commands.go - adminCommands, adminGroups, printAdminUsage, adminUsage (exit 1 without subcommand) ; drift test cmd/evey/admin_cli_test.go
sessions ended: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL internal/app/user/token_epoch.go
container: Dockerfile (EVEY_DB_PASSWORD_FILE, EVEY_BACKUP_DIR=/data/backups, completion at build time) ; docker/evey-shell.sh - man()
makefile: Makefile migrate-up / migrate-down / migrate-version
standalone programs: cmd/intro-backfill, cmd/intro-diag, cmd/trickplay-verify
-->
