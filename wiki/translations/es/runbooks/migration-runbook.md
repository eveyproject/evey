---
source_sha: 68ae93ace6a6ac1dc58140bf8cdc28c60bd2db3d0b2ce148ec58433940070f68
reviewed: false
---
# Migraciones de la base de datos

Cómo cambia Evey el esquema de su base de datos durante una actualización, y qué
hacer cuando algo sale mal. Los comandos apuntan a la instalación estándar, en la
que la base de datos se ejecuta dentro del contenedor `evey-server`.

## Cómo se ejecutan las migraciones

Las migraciones van integradas en el binario (`go:embed`) y las aplica
`golang-migrate` al arrancar el servidor
(`internal/adapters/driven/postgres/migrate.go`, archivos
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`).
El estado es la tabla `schema_migrations`: `{version, dirty}`.

Antes de aplicar nada, el servidor escribe un volcado completo en
`/data/backups/pre-migration-<version>-<time>.sql.gz` (`EVEY_BACKUP_DIR`). Se
conserva el volcado más reciente de cada una de las 3 últimas versiones del
esquema.

Las mismas operaciones están disponibles a mano:

```sh
docker exec evey-server evey admin migrate version   # sale con 1 cuando el esquema está dirty
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # exactamente un paso
```

`up` y `down` también hacen antes un volcado; `--no-backup` lo omite.

Para ejecutar SQL contra la base de datos integrada:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Antes de una actualización

1. **Haz una copia de seguridad** en **Admin → Copia de seguridad y restauración**
   (**Ejecutar ahora**), o desde un script:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   El volcado automático previo a la migración es una segunda copia, no un
   sustituto.

2. **Con un catálogo grande**, mira el tamaño de las tablas más grandes: una
   migración que reescribe una de ellas tarda más.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Comprueba la versión actual**: `evey admin migrate version`. Si indica un
   esquema dirty, consulta primero
   [Una migración que se detuvo a medias](#a-migration-that-stopped-halfway).

## Ejecutar las migraciones

Se ejecutan al arrancar la nueva imagen:

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

El contenedor se declara sano en cuanto las migraciones están aplicadas y el
servidor responde:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## Volver atrás

`evey admin rollback` muestra la versión en ejecución, la versión del esquema, el
volcado previo a la migración más reciente y los comandos exactos que ejecutar en
el equipo. No cambia nada: el contenedor no tiene acceso a Docker.

La forma habitual de volver atrás es la imagen anterior con el volcado hecho antes
de la actualización: consulta [Volver atrás](../backups.md#going-back) en Copias
de seguridad y actualizaciones. `evey admin migrate down` revierte en cambio paso
a paso, cuando basta con el `.down.sql` de ese paso.

No edites `schema_migrations` a mano, salvo para quitar un estado dirty (abajo),
y haz antes un volcado.

Restaura el volcado previo a la migración en lugar de migrar hacia abajo cuando
la actualización eliminó datos (una columna quitada, un cambio de tipo).

## Una migración que se detuvo a medias

Si el registro muestra `Dirty database version N. Fix and force version.`, una
migración se interrumpió a medio camino. Lleva el esquema al estado que quieras y
después quita la marca:

```sql
SELECT version, dirty FROM schema_migrations;

-- Vuelve a aplicar a mano el resto del up.sql, o revierte con el down.sql,
-- y después registra la versión a la que llegaste realmente:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Reinicia el contenedor; las migraciones continúan desde ahí. Restaurar el volcado
previo a la migración es la otra salida.

## Escribir una migración

Para quienes contribuyen. Ejemplos en `internal/adapters/driven/postgres/migrations/`:

- **`000066_cascade_and_email_unique.up.sql`**: `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Toma `ACCESS EXCLUSIVE` en ambas tablas mientras dura el ADD. Barato en tablas pequeñas, doloroso en `media_items` / `watch_progress`. Patrón para tablas grandes: añadir con `NOT VALID` y después `VALIDATE CONSTRAINT` en una segunda sentencia (validar solo toma `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`**: `CREATE INDEX` simple. En tablas grandes bloquea las escrituras. `CREATE INDEX CONCURRENTLY` no puede ejecutarse dentro de una transacción de migrate.
- **`000072_syncplay_queue.up.sql`**: `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. En PG 11+ solo toca metadatos y es rápido. Aun así mantiene `ACCESS EXCLUSIVE` un momento.

Pautas:

- **ALTER largo** que reescribe la tabla (cambio de tipo, añadir un valor por defecto no constante en un PG antiguo) → espera una interrupción del servicio con un catálogo grande.
- **DROP COLUMN** → solo metadatos, pero la columna no se puede recuperar sin una copia. Nunca la elimines en la misma versión en la que se despliega una nueva versión del código: mejor en dos pasos, dejar de leer la columna, desplegar, y eliminarla en la versión siguiente.
- **FK en tablas grandes** → `ADD CONSTRAINT ... NOT VALID` y después `VALIDATE CONSTRAINT` en una migración posterior. Evita un único `ADD CONSTRAINT ... FOREIGN KEY ...` bloqueante.
- **UPDATE masivo / relleno** → nunca en una sola migración sobre una tabla grande. Entrega una migración de esquema vacía + una tarea en segundo plano.
- **Sustitución de columna (expand → migrate → contract)** → no juntes `ADD COLUMN` + relleno + `SET NOT NULL` + `DROP COLUMN` de la antigua en una sola migración. Esa forma solo funciona porque hoy Evey se ejecuta en una sola instancia, con las migraciones aplicadas de forma síncrona al arrancar: durante la ventana, el código antiguo que lee la columna eliminada y el código nuevo que lee la nueva no pueden convivir, así que un despliegue progresivo o con varias réplicas serviría errores desde la mitad que va por detrás.
  Migraciones conocidas escritas así, que se mantienen tal cual porque ya se publicaron: `000077` y `000098_playlists_profile_id.up.sql` (añade `profile_id`, rellena, fija `NOT NULL`, elimina `user_id`).
  Para una nueva, repártela en tres versiones:
  1. **expand**: añadir la columna nueva admitiendo nulos, escribir en *ambas* columnas, seguir leyendo la antigua;
  2. **migrate**: rellenar la columna nueva (tarea en segundo plano, no la migración) y después pasar las lecturas a ella;
  3. **contract**: cuando ningún código desplegado toque ya la columna antigua, `SET NOT NULL` y eliminarla.
  Cada paso se puede desplegar y revertir por separado, algo que la versión de un solo archivo no permite.

## Después de una actualización

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Después vacía la caché de transcodificación si la versión cambió cómo se
codifican los vídeos (**Admin → Caché de transcodificación**).
