---
source_sha: 80ee38697d8d389ccbe3deabdb838a6d8be5fa0af92f9832ad8d150b574bdcb2
reviewed: true
---
# Línea de comandos { #command-line }

El programa del servidor sirve también de línea de comandos para el
operador: `evey admin <subcommand>` ejecuta una herramienta de mantenimiento
o de recuperación con la misma configuración y la misma base de datos que el
servidor, y luego termina. Esta página enumera los comandos y explica cómo
ejecutarlos en el contenedor; los procedimientos del día a día (contraseña
perdida, cambio de clave, restauración) están en [Recuperación](recovery.md).

## Ejecutar un comando { #running-a-command }

En la instalación estándar, los comandos se ejecutan dentro del contenedor,
que ya tiene la contraseña de la base de datos y la configuración `EVEY_*`:

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # un shell de administración: evey admin ..., man evey
```

En una instalación sin Docker, el programa se llama directamente
(`evey admin reset-password --user alice@example.com`); lee las mismas
variables `EVEY_*` que el servidor (`EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_PASSWORD`, `EVEY_DB_NAME`, `EVEY_BACKUP_DIR`...). La mayoría de los
comandos necesitan la base de datos; ninguno arranca el servidor, y la
mayoría se pueden ejecutar sin riesgo con el servidor parado.

| Llamada | Qué hace |
| --- | --- |
| `evey` | Arranca el servidor. |
| `evey admin <subcommand>` | Ejecuta una herramienta del operador y termina. `evey admin` solo muestra la lista y termina con el código 1; `evey admin <subcommand> --help` detalla uno. |
| `evey version` | Versión, commit, fecha de compilación. No necesita configuración ni base de datos, así que responde incluso desde un contenedor que no consigue arrancar. |
| `evey help` | El resumen de los comandos. |
| `evey man [--text]` | El manual: en roff para `man`, en texto plano con `--text`. |
| `evey completion bash` | El script de autocompletado para bash. |

Cualquier otro primer argumento se rechaza (código de salida 1) en lugar de
arrancar el servidor sin avisar.

## Los comandos { #the-commands }

La lista de abajo es el propio texto `--help` del programa, agrupado como lo
muestra `evey admin`. `evey admin` sin subcomando siempre da la lista
actual.

### Recuperación { #recovery }

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

`reset-password` y `rotate-jwt-key` cierran sesiones los dos: las del usuario
en el primer caso, las de todo el mundo en el segundo. Las apps abiertas lo
notan en unos 30 segundos y piden volver a iniciar sesión. Los procedimientos
completos están en [Recuperación](recovery.md).

### Esquema { #schema }

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

Se hace un `pg_dump` antes de la migración para `up` y `down` cuando
`EVEY_BACKUP_DIR` está definido (`/data/backups` en la imagen Docker);
`--no-backup` lo omite, para equipos sin `pg_dump`. Un fallo a mitad de
`up` deja `schema_migrations.dirty = true`: consulta
[Migraciones de la base de datos](migration-runbook.md). En el entorno de
desarrollo, el Makefile ofrece estos tres como `make migrate-up`,
`make migrate-down` y `make migrate-version`, ejecutados en el contenedor de
desarrollo `evey-server`.

### Mantenimiento de los medios { #media-maintenance }

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

El mismo motor funciona detrás de **Administración → Salud de medios**, que
es la forma más fácil de reparar una biblioteca: consulta
[Salud de medios](media-health.md).

### Codificación por título { #per-title-encoding }

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### Diagnóstico { #diagnostics }

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

### Capacidad { #capacity }

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

### Fuentes del autocompletado { #completion-sources }

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## Autocompletado y manual { #tab-completion-and-the-manual }

El programa lleva su propio autocompletado para bash y su propio manual, así
que los dos coinciden siempre con los subcomandos que incluye:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

Para zsh: primero `autoload -U +X bashcompinit && bashcompinit`, y luego
carga el mismo script. El autocompletado cubre los subcomandos, sus opciones
y los valores de las opciones (`--mode`, `--backend`, `--scenario`), los
correos de los usuarios después de `--user`, las carpetas raíz de biblioteca
que conoce el servidor después de `--root` (además de cualquier carpeta),
solo archivos de vídeo para los argumentos de archivo de `ladder-bench`,
`ladder-probe` y `route-bench`, y los volcados `.sql.gz` para
`restore-backup`.

La imagen Docker instala el autocompletado al construirse, y su shell de
administración (`docker exec -it evey-server bash`) responde a `man evey`
mostrando `evey man --text` página a página, porque la imagen no incluye el
programa `man`.

## Lo que no está en `evey admin` { #what-is-not-in-evey-admin }

Hay tres programas independientes junto al servidor en el código fuente. Son
herramientas de desarrollo y de pruebas, no comandos para el operador, y la
imagen Docker no los incluye:

| Programa | Para qué sirve |
| --- | --- |
| `cmd/intro-backfill` | Volver a lanzar la detección de intros en una parte del catálogo después de un cambio en el proceso. Es un cliente de la API con un token bearer, no funciona dentro del servidor. |
| `cmd/intro-diag` | Explicar por qué la detección de intros ha encontrado, o no, una intro en una serie, a partir de las miniaturas de la barra de avance guardadas en disco. |
| `cmd/trickplay-verify` | Comprobar que cada miniatura de la barra de avance muestra el fotograma de su marca de tiempo, volviendo a extraer el fotograma de origen con ffmpeg. |

## Procedimientos relacionados { #related-runbooks }

- [Recuperación](recovery.md): restablecer una contraseña, cambiar la clave JWT, restaurar una copia de seguridad.
- [Migraciones de la base de datos](migration-runbook.md): desplegar el esquema, volver atrás, recuperar un estado «dirty».
- [Manual de incidentes](incident-runbook.md): qué hacer primero cuando el servidor no funciona como debe.

<!-- sources
dispatch and top-level commands: cmd/evey/cli.go - runCLI (version, help, completion, man, unknown command refused), cmd/evey/main.go
subcommand table, groups, --help text: cmd/evey/admin_commands.go - adminCommands, adminGroups, printAdminUsage, adminUsage (exit 1 without subcommand) ; drift test cmd/evey/admin_cli_test.go
sessions ended: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL internal/app/user/token_epoch.go
backup and restore: cmd/evey/admin_backup.go - adminBackup ; cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete
container: Dockerfile (EVEY_DB_PASSWORD_FILE, completion at build time) ; internal/config/config.go (EVEY_BACKUP_DIR default /data/backups) ; docker/evey-shell.sh - man()
makefile: Makefile migrate-up / migrate-down / migrate-version
standalone programs: cmd/intro-backfill, cmd/intro-diag, cmd/trickplay-verify
-->
