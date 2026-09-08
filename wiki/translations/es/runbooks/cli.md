---
source_sha: 82f98fcae7d7d2028b57419f257f377ba2928df3c32c0a66a00c30682eda65c1
---

# CLI de Evey - `evey admin`

El binario del servidor de Evey hace también de CLI del operador. Invocado como
`evey` sin argumentos, arranca el servidor HTTP y gRPC; con `admin` como primer
argumento, pasa a modo de operador de una sola pasada y sale en cuanto termina
la orden.

Todas las órdenes leen la configuración de las mismas variables `EVEY_*` que el
propio servidor (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, etc.).

## Referencia rápida

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

### Mantenimiento de los medios

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

> La lista completa y siempre al día es `evey admin` sin ningún subcomando: la
> imprime él. 2026-07-23; `reindex-no-cue`, la reparación del índice de
> posicionamiento que hay detrás de las incidencias recurrentes de reanudación
> lenta, estaba entre las que faltaban.

## Cómo invocarlo

### Desde el host, con la pila en marcha (Docker de desarrollo o producción)

El Makefile envuelve las operaciones más habituales en llamadas al intérprete
del contenedor, para que funcionen sin cargar el `.env` y sin necesidad de tener
pg_dump en el host:

```sh
make migrate-version
make migrate-up
make migrate-down
```

Por debajo hacen `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'`,
así que `evey-server` tiene que estar levantado; si no lo está, arránquelo antes con `make dev`.

### Entrando directamente en el contenedor en marcha

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### En una instalación sin contenedor (sin Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

El binario ya instalado trae todos los subcomandos dentro.

## Los subcomandos en detalle

### `migrate`

- `up` aplica en orden todas las migraciones pendientes. Se hace un `pg_dump`
  previo de forma automática cuando `EVEY_BACKUP_DIR` está definida (en la
  imagen Docker, por defecto `/data/backups`). Un fallo a mitad del Up deja
  `schema_migrations.dirty = true`; véase `migration-runbook.md`.
- `down` retrocede exactamente una migración. Es quirúrgico, no un «tíralo todo
  abajo»: no existe ningún `down --all`. También hace un `pg_dump` previo.
- `version` imprime la versión actual del esquema. Devuelve el código de salida
  1 con un aviso `DIRTY` cuando una migración anterior se quedó a medias, es
  decir cuando se ejecutaron algunas sentencias del `Up` pero no todas.
- `--no-backup` se salta el pg_dump en cualquiera de los dos sentidos. Sirve en
  hosts sin `postgres-client` instalado (el caso típico del desarrollo en WSL,
  donde el Makefile ya pasa la opción).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

El procedimiento completo, con sus requisitos previos y sus poscondiciones,
está en `docs/runbooks/recovery.md`.

## Lo que *no* está en `evey admin`

Junto a `cmd/evey` viven tres binarios independientes, porque no son órdenes de
operador en el sentido de la recuperación o del esquema:

| Binario | Para qué sirve | Por qué va aparte |
|---|---|---|
| `cmd/intro-backfill` | Rehacer la detección de intros sobre una parte del catálogo después de cambiar la cadena | **Es un cliente de la API**: habla con `/api/v1/admin/media/{id}/detect-segments` usando un token Bearer, no dentro del proceso. |
| `cmd/intro-diag` | Averiguar por qué la detección de intros por pHash acertó (o no) en una serie concreta | **Herramienta de depuración** para quien desarrolla: lee del disco los sprites de trickplay e imprime un mapa de coincidencia de Hamming fotograma a fotograma. Con una pasada opcional de correlación cruzada del audio. |
| `cmd/trickplay-verify` | Comprobar que cada casilla de trickplay muestra de verdad el fotograma de la marca de tiempo anotada | **Herramienta de QA y de CI**: vuelve a extraer los fotogramas de la fuente con ffmpeg y les calcula el aHash para compararlos con la casilla. |

Van como binarios aparte porque ninguno necesita todo el cableado de
configuración del servidor: son herramientas acotadas, de un solo uso, y no
forman parte de la superficie de recuperación.

## Guías relacionadas

- `migration-runbook.md` - despliegue del esquema, vuelta atrás y recuperación del estado `dirty`.
- `recovery.md` - restablecer una contraseña, rotar la clave JWT, restaurar una copia y reindexar la búsqueda.
- `incident-runbook.md` - primera respuesta cuando el servidor se porta mal.
