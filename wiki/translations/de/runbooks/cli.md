---
source_sha: 80ee38697d8d389ccbe3deabdb838a6d8be5fa0af92f9832ad8d150b574bdcb2
reviewed: true
---
# Kommandozeile { #command-line }

Das Programm des Servers ist zugleich die Kommandozeile für den Betreiber:
`evey admin <subcommand>` führt ein Wartungs- oder Wiederherstellungswerkzeug
mit derselben Konfiguration und derselben Datenbank wie der Server aus und
beendet sich dann. Diese Seite listet die Befehle auf und zeigt, wie Sie sie
im Container ausführen. Die Abläufe für den Alltag (Passwort vergessen,
Schlüssel wechseln, Wiederherstellen) stehen unter
[Wiederherstellung](recovery.md).

## Einen Befehl ausführen { #running-a-command }

In der Standardinstallation laufen die Befehle im Container. Dort sind das
Datenbankpasswort und die `EVEY_*`-Konfiguration schon vorhanden:

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # eine Admin-Shell: evey admin ..., man evey
```

Bei einer Installation ohne Docker rufen Sie das Programm direkt auf
(`evey admin reset-password --user alice@example.com`). Es liest dieselben
`EVEY_*`-Variablen wie der Server (`EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_PASSWORD`, `EVEY_DB_NAME`, `EVEY_BACKUP_DIR`...). Die meisten Befehle
brauchen die Datenbank. Keiner startet den Server, und die meisten können Sie
gefahrlos ausführen, während er gestoppt ist.

| Aufruf | Was er tut |
| --- | --- |
| `evey` | Startet den Server. |
| `evey admin <subcommand>` | Führt ein Werkzeug für den Betreiber aus und beendet sich. `evey admin` allein gibt die Liste aus und endet mit Code 1; `evey admin <subcommand> --help` erklärt einen Befehl genauer. |
| `evey version` | Version, Commit, Build-Datum. Braucht weder Konfiguration noch Datenbank und antwortet daher auch aus einem Container, der nicht startet. |
| `evey help` | Die Übersicht der Befehle. |
| `evey man [--text]` | Das Handbuch: roff für `man`, einfacher Text mit `--text`. |
| `evey completion bash` | Das Skript für die Vervollständigung in bash. |

Jedes andere erste Argument wird abgelehnt (Code 1), statt stillschweigend
den Server zu starten.

## Die Befehle { #the-commands }

Die Liste unten ist der `--help`-Text des Programms selbst, so gruppiert, wie
`evey admin` ihn ausgibt. `evey admin` ohne Unterbefehl zeigt immer die
aktuelle Liste.

### Wiederherstellung { #recovery }

```
evey admin reset-password --user <email>     Generate a new password for the user, print it once.
                                             Every session of that user is ended at the same time.
evey admin rotate-jwt-key                    Regenerate the JWT signing key. Invalidates ALL active
                                             sessions: the key is moved aside (kept as
                                             <key>.rotated-<timestamp>), the next server boot
                                             generates a new one, and every session is ended in the
                                             database at once.
evey admin backup [--include-telemetry]
                                             Create a database backup in the backup directory, the
                                             same .tar.gz as the panel and the scheduler. Telemetry
                                             follows the panel switch (off by default);
                                             --include-telemetry includes it for this run.
evey admin restore-backup <path-to-dump.sql.gz>
                                             Restore the database from a pg_dump.gz file, in a single
                                             transaction. The whole file is checked first: a truncated
                                             or corrupt dump is refused before anything is dropped.
                                             The dump drops and recreates each table before loading
                                             it, so it can be restored onto a populated database;
                                             waits 5 s for a Ctrl-C
                                             before it starts. The .sql.gz suffix is required (not the
                                             panel's .tar.gz backups).
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

`reset-password` und `rotate-jwt-key` beenden beide Sitzungen: der erste die
des betroffenen Nutzers, der zweite die aller Nutzer. Offene Apps merken das
innerhalb von etwa 30 Sekunden und bitten um eine neue Anmeldung. Die
vollständigen Abläufe stehen unter [Wiederherstellung](recovery.md).

### Schema { #schema }

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

Vor `up` und `down` wird ein `pg_dump` angelegt, wenn `EVEY_BACKUP_DIR`
gesetzt ist (`/data/backups` im Docker-Image); `--no-backup` lässt ihn weg,
für Rechner ohne `pg_dump`. Scheitert `up` mittendrin, bleibt
`schema_migrations.dirty = true` stehen: siehe
[Datenbankmigrationen](migration-runbook.md). Auf dem Entwicklungsstack
verpackt das Makefile diese drei Befehle als `make migrate-up`,
`make migrate-down` und `make migrate-version`; sie laufen im
Entwicklungscontainer `evey-server`.

### Medienwartung { #media-maintenance }

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

Dieselbe Technik steckt hinter **Verwaltung → Medienzustand**. Das ist der
einfachere Weg, eine Bibliothek zu reparieren: siehe
[Medienzustand](media-health.md).

### Kodierung pro Titel { #per-title-encoding }

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### Diagnose { #diagnostics }

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
                                             Minutes of full load; never while serving. More flags
                                             (--device --routes --classes --duration --workdir
                                             --capability): the full list is in
                                             `evey admin route-bench --help`.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...
                                             Encode a few scenes of each title at fixed CRFs and
                                             score encode cost against the ladder's complexity
                                             features. Runs ffmpeg for minutes; never while serving.
                                             More flags (--preset --ref-height --ref-crf --cache
                                             --encoders --device): the full list is in
                                             `evey admin ladder-bench --help`.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Run the per-title probe on the given files: probe time,
                                             early-stop rate and points. Reads and encodes only.
```

### Kapazität { #capacity }

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

### Quellen für die Vervollständigung { #completion-sources }

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## Tab-Vervollständigung und Handbuch { #tab-completion-and-the-manual }

Das Programm bringt seine eigene bash-Vervollständigung und sein eigenes
Handbuch mit. Beide passen daher immer zu den Unterbefehlen, die es enthält:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

Wer zsh nutzt, führt zuerst `autoload -U +X bashcompinit && bashcompinit` aus
und lädt dann dasselbe Skript. Die Vervollständigung kennt die Unterbefehle,
ihre Optionen und deren Werte (`--mode`, `--backend`, `--scenario`), die
E-Mail-Adressen der Nutzer nach `--user`, die Stammordner der Bibliotheken,
die der Server kennt, nach `--root` (plus jeden Ordner), nur Mediendateien für
die Dateiargumente von `ladder-bench`, `ladder-probe` und `route-bench`, und
`.sql.gz`-Dumps für `restore-backup`.

Das Docker-Image installiert die Vervollständigung schon beim Bauen. Seine
Admin-Shell (`docker exec -it evey-server bash`) beantwortet `man evey`, indem
sie `evey man --text` seitenweise anzeigt, denn das Image enthält kein
`man`-Programm.

## Was nicht in `evey admin` steckt { #what-is-not-in-evey-admin }

Drei eigenständige Programme liegen im Quellcode neben dem Server. Sie sind
Werkzeuge für Entwicklung und Tests, keine Befehle für den Betreiber, und das
Docker-Image enthält sie nicht:

| Programm | Zweck |
| --- | --- |
| `cmd/intro-backfill` | Die Intro-Erkennung für einen Teil des Katalogs auffrischen, nachdem sich die Verarbeitung geändert hat. Ein API-Client mit Bearer-Token, nicht im Prozess des Servers. |
| `cmd/intro-diag` | Erklären, warum die Intro-Erkennung bei einer Serie ein Intro gefunden hat oder nicht, anhand der Trickplay-Sprites auf der Festplatte. |
| `cmd/trickplay-verify` | Prüfen, dass jede Trickplay-Kachel das Bild zum gespeicherten Zeitpunkt zeigt, indem das Bild der Quelle mit ffmpeg erneut extrahiert wird. |

## Verwandte Runbooks { #related-runbooks }

- [Wiederherstellung](recovery.md): Passwort zurücksetzen, JWT-Schlüssel wechseln, Sicherung wiederherstellen.
- [Datenbankmigrationen](migration-runbook.md): Schema ausrollen, zurückrollen, einen „dirty“-Zustand beheben.
- [Störungshandbuch](incident-runbook.md): die ersten Schritte, wenn beim Server Probleme auftreten.

<!-- sources
dispatch and top-level commands: cmd/evey/cli.go - runCLI (version, help, completion, man, unknown command refused), cmd/evey/main.go
subcommand table, groups, --help text: cmd/evey/admin_commands.go - adminCommands, adminGroups, printAdminUsage, adminUsage (exit 1 without subcommand) ; drift test cmd/evey/admin_cli_test.go
sessions ended: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL internal/app/user/token_epoch.go
backup and restore: cmd/evey/admin_backup.go - adminBackup ; cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete
container: Dockerfile (EVEY_DB_PASSWORD_FILE, completion at build time) ; internal/config/config.go (EVEY_BACKUP_DIR default /data/backups) ; docker/evey-shell.sh - man()
makefile: Makefile migrate-up / migrate-down / migrate-version
standalone programs: cmd/intro-backfill, cmd/intro-diag, cmd/trickplay-verify
-->
