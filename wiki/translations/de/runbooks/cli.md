---
source_sha: 82f98fcae7d7d2028b57419f257f377ba2928df3c32c0a66a00c30682eda65c1
---
# Evey CLI - `evey admin`

Die Server-Binärdatei von Evey ist zugleich die Kommandozeile für Betreiber.
Rufen Sie `evey` ohne Argumente auf, startet der HTTP- und gRPC-Server; steht
`admin` als erstes Argument da, wechselt sie in einen einmaligen Betreibermodus
und endet, sobald der Befehl durch ist.

Alle Befehle lesen ihre Konfiguration aus denselben `EVEY_*`-Umgebungsvariablen
wie der Server selbst (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...).

## Kurzreferenz

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

### Wartung der Medien

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

> Die vollständige, stets aktuelle Liste liefert `evey admin` ohne Unterbefehl -
> sie gibt 2026-07-23 aus; `reindex-no-cue`, die Reparatur des Suchindex hinter
> den wiederkehrenden Störungen mit langsamem Fortsetzen, war eine der
> fehlenden.

## Wie Sie sie aufrufen

### Vom Host aus, bei laufendem Stack (Docker in der Entwicklung oder im Produktivbetrieb)

Das Makefile verpackt die gängigsten Handgriffe in Aufrufe der Container-Shell,
damit sie ohne ein gesourctes `.env` und ohne pg_dump auf dem Host
funktionieren:

```sh
make migrate-version
make migrate-up
make migrate-down
```

Diese rufen `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'` auf,
`evey-server` muss also laufen - starten Sie ihn andernfalls zuerst mit `make dev`.

### Unmittelbar in den laufenden Container hinein

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### Bei einer Installation ohne Container (ohne Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

In der vorinstallierten Binärdatei stecken bereits alle Unterbefehle.

## Die Unterbefehle im Einzelnen

### `migrate`

- `up` wendet jede ausstehende Migration der Reihe nach an. Ist
  `EVEY_BACKUP_DIR` gesetzt, entsteht davor automatisch ein `pg_dump` (im
  Docker-Image standardmäßig `/data/backups`). Scheitert Up auf halbem Weg,
  bleibt `schema_migrations.dirty = true` zurück; siehe
  `migration-runbook.md`.
- `down` rollt genau eine Migration zurück. Chirurgisch, nicht „alles
  einreißen“ - ein `down --all` gibt es nicht. Auch hier entsteht vorher ein
  `pg_dump`.
- `version` gibt die aktuelle Schemaversion aus. Der Rückgabewert ist 1, dazu
  eine `DIRTY`-Warnung, wenn eine frühere Migration auf halbem Weg gescheitert
  ist (also einige, aber nicht alle `Up`-Anweisungen durchliefen).
- `--no-backup` überspringt das pg_dump in beide Richtungen. Nützlich auf
  Rechnern ohne installiertes `postgres-client` (der typische Fall bei der
  Entwicklung unter WSL, wo das Makefile den Schalter bereits mitgibt).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

Das vollständige Vorgehen samt Voraussetzungen und Nachbedingungen steht in
`docs/runbooks/recovery.md`.

## Was *nicht* in `evey admin` steckt

Drei eigenständige Binärdateien liegen neben `cmd/evey`, weil sie keine
Betreiberbefehle im Sinne von Wiederherstellung oder Schema sind:

| Binärdatei | Wozu | Warum eigenständig |
|---|---|---|
| `cmd/intro-backfill` | Die Intro-Erkennung nach einer Änderung an der Pipeline für einen Ausschnitt des Katalogs auffrischen | **API-Client** - spricht mit `/api/v1/admin/media/{id}/detect-segments` und einem Bearer-Token, nicht im selben Prozess. |
| `cmd/intro-diag` | Nachvollziehen, warum die Intro-Erkennung per pHash bei einer bestimmten Serie angeschlagen hat oder eben nicht | Ein Werkzeug zum **Debuggen in der Entwicklung** - liest die Trickplay-Sprites von der Platte und gibt Bild für Bild eine Karte der Hamming-Treffer aus. Optional zusätzlich ein Durchgang mit Kreuzkorrelation des Tons. |
| `cmd/trickplay-verify` | Nachprüfen, dass jede Trickplay-Kachel wirklich das Bild zum eingetragenen Zeitstempel zeigt | Ein Werkzeug für **QA und CI** - holt die Bilder über ffmpeg erneut aus der Quelle, bildet ihren aHash und vergleicht ihn mit der Kachel. |

Sie werden als eigene Binärdateien ausgeliefert, weil keine davon die ganze
Verdrahtung der Serverkonfiguration braucht - es sind eng gefasste Werkzeuge für
den einmaligen Lauf, kein Teil der Oberfläche für die Wiederherstellung.

## Verwandte Runbooks

- `migration-runbook.md` - Schema ausrollen, zurückrollen, aus dem Zustand `dirty` herausfinden.
- `recovery.md` - Passwort zurücksetzen, JWT-Schlüssel wechseln, Sicherung zurückspielen, Suche neu indizieren.
- `incident-runbook.md` - die erste Reaktion, wenn sich der Server danebenbenimmt.
