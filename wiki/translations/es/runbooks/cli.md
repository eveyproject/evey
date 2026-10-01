---
source_sha: f17930061339f3eb3099a82e050ac2879e82832832be4739cf28de6bb5bcbec3
reviewed: false
---
# CLI de Evey - `evey admin`

El binario del servidor Evey sirve también como CLI de operación. Cuando se invoca
`evey` sin argumentos, arranca el servidor HTTP/gRPC; con `admin` como primer
argumento, pasa a un modo de operación de un solo uso y termina cuando el comando
acaba.

Todos los comandos leen la configuración de las mismas variables `EVEY_*` que el
propio servidor (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
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

Cualquier otro primer argumento se rechaza (código de salida 1) en lugar de
arrancar el servidor en silencio. La tabla Go de `cmd/evey/admin_commands.go` es
la descripción única de cada subcomando: la lista de uso, `--help`, el manual, el
script de autocompletado y el shell del contenedor se comprueban contra ella en
`cmd/evey/admin_cli_test.go`.

## Referencia rápida

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key (access tokens only, see recovery.md).
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required; not the panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line (feeds `--user` completion).
evey admin list-libraries                    Print the library root paths, one per line (feeds `--root` completion).
```

### Mantenimiento de medios

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

### Diagnóstico y capacidad

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

### Instancia de demostración

```
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

> La lista completa y siempre actualizada es `evey admin` sin subcomando: muestra
> todos los subcomandos que lleva el binario.

## Cómo invocarla

### En el contenedor (instalación estándar)

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # un shell de administración: evey admin ..., man evey
```

La imagen define el archivo con la contraseña de la base de datos, así que los
comandos llegan a la base de datos integrada sin ningún entorno adicional.

### En una instalación sin contenedor (sin Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

El binario preinstalado ya incluye todos los subcomandos.

### En el entorno de desarrollo

El Makefile envuelve los comandos de migración, que se ejecutan desde el árbol de
fuentes en el contenedor de desarrollo `evey-server`:

```sh
make migrate-version
make migrate-up
make migrate-down
```

## Autocompletado y manual

El binario lleva su propio autocompletado de bash y su manual, así que ambos
coinciden siempre con los subcomandos que incluye:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

Usuarios de zsh: primero `autoload -U +X bashcompinit && bashcompinit`, y después
cargar el mismo script. El autocompletado cubre los subcomandos, sus opciones y
los valores de opciones (`--mode`, `--backend`, `--scenario`), los correos de
usuario tras `--user`, las raíces de biblioteca que conoce el servidor tras
`--root` (además de cualquier directorio), solo archivos multimedia para los
argumentos de archivo de `ladder-bench`, `ladder-probe` y `route-bench`, y los
volcados `.sql.gz` para `restore-backup`.

La imagen Docker instala el autocompletado al construirse y su shell de
administración (`docker exec -it evey-server bash`) responde a `man evey`
paginando `evey man --text`, ya que la imagen no incluye el binario `man`.

## Detalle de los subcomandos

### `migrate`

- `up` aplica en orden todas las migraciones pendientes. Se hace un `pg_dump`
  previo a la migración automáticamente cuando `EVEY_BACKUP_DIR` está definido
  (por defecto en la imagen Docker: `/data/backups`). Un fallo a mitad de Up deja
  `schema_migrations.dirty = true`; consulta `migration-runbook.md`.
- `down` revierte exactamente una migración. Es quirúrgico, no "derribarlo
  todo": no existe `down --all`. También hace un `pg_dump` previo a la reversión.
- `version` muestra la versión actual del esquema. Devuelve el código de salida 1
  con un aviso `DIRTY` cuando una migración anterior falló a medias (es decir,
  se ejecutaron algunas sentencias `Up`, pero no todas).
- `--no-backup` omite el pg_dump en ambos sentidos. Útil en equipos sin
  `postgres-client` instalado (el caso típico de desarrollo en WSL, donde el
  Makefile ya pasa la opción).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

Consulta `docs/runbooks/recovery.md` para el procedimiento completo, los
requisitos previos y las condiciones posteriores.

## Lo que *no* está en `evey admin`

Tres binarios independientes viven junto a `cmd/evey` porque no son comandos de
operación en el sentido de recuperación o esquema:

| Binario | Propósito | Por qué independiente |
|---|---|---|
| `cmd/intro-backfill` | Actualizar la detección de intros en una parte del catálogo tras un cambio en la canalización | **Cliente de API**: habla con `/api/v1/admin/media/{id}/detect-segments` con un token bearer, no dentro del proceso. |
| `cmd/intro-diag` | Diagnosticar por qué la detección de intros por pHash coincidió (o no) en una serie concreta | Herramienta de **depuración para desarrolladores**: lee del disco los sprites de trickplay e imprime un mapa de coincidencias de Hamming fotograma a fotograma. Pasada opcional de correlación cruzada de audio. |
| `cmd/trickplay-verify` | Comprobar que cada mosaico de trickplay muestra de verdad el fotograma de la marca de tiempo registrada | Herramienta de **QA/CI**: vuelve a extraer los fotogramas de origen con ffmpeg y compara su aHash con el mosaico. |

Se distribuyen como binarios aparte porque ninguno necesita toda la configuración
del servidor: son herramientas acotadas, de un solo uso, y no forman parte de la
superficie de recuperación.

## Manuales relacionados

- `migration-runbook.md`: despliegue del esquema, reversión, recuperación de un estado dirty.
- `recovery.md`: restablecer contraseña, rotar la clave JWT, restaurar una copia, reindexar la búsqueda.
- `incident-runbook.md`: primera respuesta cuando el servidor se comporta mal.
