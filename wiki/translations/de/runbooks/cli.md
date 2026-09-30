---
source_sha: fc91b3be124757e49009dbe56dd9647724bd582d2bc356702367e4982e757455
reviewed: false
---
# Evey-CLI - `evey admin`

Das Serverprogramm von Evey dient zugleich als Betreiber-CLI. Wird `evey`
ohne Argumente aufgerufen, startet es den HTTP/gRPC-Server; mit `admin` als
erstem Argument wechselt es in einen einmaligen Betreibermodus und beendet
sich, sobald der Befehl fertig ist.

Alle Befehle lesen ihre Konfiguration aus denselben `EVEY_*`-Umgebungsvariablen
wie der Server selbst (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...).

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

Jedes andere erste Argument wird abgelehnt (Exit 1), statt stillschweigend den
Server zu starten. Die Go-Tabelle in `cmd/evey/admin_commands.go` ist die
einzige Beschreibung jedes Unterbefehls: Nutzungsliste, `--help`, Handbuch,
Vervollständigungsskript und Container-Shell werden von
`cmd/evey/admin_cli_test.go` dagegen geprüft.

## Kurzreferenz

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key (kills ALL sessions).
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required; not the panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line (feeds `--user` completion).
evey admin list-libraries                    Print the library root paths, one per line (feeds `--root` completion).
```

### Medienwartung

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

### Diagnose und Kapazität

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (owned by the server UID?),
                                             cache dirs, ffmpeg/mkvgo and disk. --repair applies only
                                             the safe fixes (create + chown missing writable dirs).
evey admin rollback                          Read-only advisor: prints the running version, schema,
                                             newest pre-migration backup and the exact HOST commands
                                             to roll back. The container has no Docker access, by
                                             design, so it advises rather than acts.
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
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
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights ...] [--fps ...] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the admission cost
                                             model. Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights ...] [--crf ...] [--out rows.csv] <file>...
                                             Ladder complexity features on a few scenes per title,
                                             encoded at fixed CRFs and scored. Never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Per-title probe timing and early-stop rate on the files.
```

### Demo-Instanz

```
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

> Die vollständige, stets aktuelle Liste liefert `evey admin` ohne
> Unterbefehl: Es gibt jeden Unterbefehl aus, den das Programm enthält.

## Aufruf

### Im Container (Standardinstallation)

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # eine Admin-Shell: evey admin ..., man evey
```

Das Image setzt die Passwortdatei der Datenbank, sodass die Befehle die
eingebettete Datenbank ohne zusätzliche Umgebung erreichen.

### Auf einer Installation ohne Docker

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

Das installierte Programm enthält bereits jeden Unterbefehl.

### Auf dem Entwicklungsstack

Das Makefile kapselt die Migrationsbefehle, die aus dem Quellbaum im
Entwicklungscontainer `evey-server` laufen:

```sh
make migrate-version
make migrate-up
make migrate-down
```

## Tab-Vervollständigung und Handbuch

Das Programm bringt seine eigene Bash-Vervollständigung und sein eigenes
Handbuch mit, sodass beide immer zu den mitgelieferten Unterbefehlen passen:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh-Benutzer: zuerst `autoload -U +X bashcompinit && bashcompinit`, dann dasselbe
Skript einlesen. Die Vervollständigung deckt die Unterbefehle, ihre Optionen und
Optionswerte ab (`--mode`, `--backend`, `--scenario`), Benutzer-E-Mails nach
`--user`, die dem Server bekannten Bibliothekswurzeln nach `--root` (plus jedes
Verzeichnis), nur Mediendateien für die Dateiargumente von `ladder-bench`,
`ladder-probe` und `route-bench`, und `.sql.gz`-Dumps für `restore-backup`.

Das Docker-Image installiert die Vervollständigung beim Bauen, und seine
Admin-Shell (`docker exec -it evey-server bash`) beantwortet `man evey`, indem
sie `evey man --text` seitenweise anzeigt, da das Image kein `man`-Programm
mitbringt.

## Unterbefehle im Detail

### `migrate`

- `up` wendet jede ausstehende Migration der Reihe nach an. Ein `pg_dump` vor
  der Migration wird automatisch erstellt, wenn `EVEY_BACKUP_DIR` gesetzt ist
  (Standard im Docker-Image: `/data/backups`). Ein Fehler mitten in Up
  hinterlässt `schema_migrations.dirty = true`; siehe `migration-runbook.md`.
- `down` macht genau eine Migration rückgängig. Gezielt, kein „alles abreißen" -
  es gibt kein `down --all`. Erstellt ebenfalls vorher einen `pg_dump`.
- `version` gibt die aktuelle Schemaversion aus. Liefert Exit-Code 1 mit einer
  Warnung `DIRTY`, wenn eine frühere Migration auf halbem Weg gescheitert ist
  (also einige, aber nicht alle `Up`-Anweisungen gelaufen sind).
- `--no-backup` überspringt den pg_dump in beide Richtungen. Nützlich auf Hosts
  ohne installierten `postgres-client` (typischer WSL-Entwicklungsweg, auf dem
  das Makefile die Option bereits übergibt).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

Siehe `docs/runbooks/recovery.md` für das vollständige Vorgehen, die
Voraussetzungen und die Nachbedingungen.

## Was *nicht* in `evey admin` steckt

Drei eigenständige Programme liegen neben `cmd/evey`, weil sie keine
Betreiberbefehle im Sinne von Wiederherstellung oder Schema sind:

| Programm | Zweck | Warum eigenständig |
|---|---|---|
| `cmd/intro-backfill` | Die Intro-Erkennung für einen Teil des Katalogs nach einer Änderung der Pipeline auffrischen | **API-Client** - spricht mit `/api/v1/admin/media/{id}/detect-segments` mit einem Bearer-Token, nicht im Prozess. |
| `cmd/intro-diag` | Diagnostizieren, warum die pHash-Intro-Erkennung bei einer bestimmten Serie getroffen hat (oder nicht) | **Entwickler-Debugwerkzeug** - liest Trickplay-Sprites von der Festplatte und gibt eine Hamming-Trefferkarte Bild für Bild aus. Optionaler Durchlauf mit Audio-Kreuzkorrelation. |
| `cmd/trickplay-verify` | Prüfen, dass jede Trickplay-Kachel wirklich das Bild zum gespeicherten Zeitpunkt zeigt | **QA/CI-Werkzeug** - extrahiert die Quellbilder mit ffmpeg neu und vergleicht ihren aHash mit der Kachel. |

Sie werden als eigene Programme ausgeliefert, weil keines davon die gesamte
Serverkonfiguration braucht - es sind gezielte, einmalige Werkzeuge, kein Teil
der Wiederherstellung.

## Verwandte Runbooks

- `migration-runbook.md` - Schema ausrollen, zurücknehmen, Zustand „dirty" beheben.
- `recovery.md` - Passwort zurücksetzen, JWT-Schlüssel rotieren, Sicherung wiederherstellen, Suchindex neu aufbauen.
- `incident-runbook.md` - erste Reaktion, wenn der Server sich seltsam verhält.
