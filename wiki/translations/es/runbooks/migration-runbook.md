---
source_sha: e42888fc562d4048501a959c02c07cd4a45986308799bd171bf193880d6a12ee
reviewed: true
---
# Migraciones de la base de datos { #database-migrations }

Cómo cambia Evey el esquema de su base de datos durante una actualización,
qué comprobar antes y después, y qué hacer cuando una migración se queda a
medias. Para quien administra la instalación estándar, donde la base de datos
funciona dentro del contenedor `evey-server`; quien contribuya escribiendo
una migración encontrará las reglas al final.

## Cómo se aplican las migraciones { #how-migrations-run }

Las migraciones van dentro del programa y las aplica `golang-migrate` cuando
arranca el servidor. El estado está en la tabla `schema_migrations`, una fila
`{version, dirty}`. Antes de aplicar nada, el servidor escribe una copia
completa en `EVEY_BACKUP_DIR` (`/data/backups`, es decir,
`./evey-data/backups` en el equipo) con el nombre
`pre-migration-<version>-<UTC timestamp>.sql.gz`, y guarda la copia más
reciente de cada una de las 3 últimas versiones del esquema.

Las mismas operaciones existen a mano:

```sh
docker exec evey-server evey admin migrate version   # código de salida 1 cuando el esquema está «dirty»
docker exec evey-server evey admin migrate up        # aplica todas las migraciones pendientes
docker exec evey-server evey admin migrate down      # deshace exactamente un paso
```

`up` y `down` hacen antes una copia; `--no-backup` la omite. Para abrir
un shell SQL en la base de datos integrada:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Actualizar sin riesgo { #update-safely }

1. **Haz una copia de seguridad** en **Administración → Copia de seguridad y
   restauración** (**Ejecutar ahora**), o desde un script:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   La copia automática anterior a la migración es una segunda copia, no
   sustituye a esta.

2. **Comprueba la versión actual**: `docker exec evey-server evey admin migrate
   version` muestra `schema version: N`. Si muestra `DIRTY`, ve a
   [Una migración que se ha quedado a medias](#a-migration-that-stopped-halfway)
   antes de actualizar.

3. **Con un catálogo grande**, mira las tablas más grandes: una migración que
   reescribe una de ellas tarda más, y la comprobación de salud del
   contenedor deja 300 segundos de arranque para eso.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **Arranca la imagen nueva** y sigue el registro hasta que las migraciones
   estén aplicadas y el servidor responda:
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   Un resultado correcto: una línea `migrations: pre-migration backup written`
   cuando había algo que aplicar, después el banner del servidor, y después
   `curl -sf http://localhost:9091/healthz` responde.

5. **Después**, confirma la versión y busca errores:
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   Vacía la caché de transcodificación (**Administración → Caché de
   transcodificación → Borrar todo**) si la versión ha cambiado la forma de
   codificar los vídeos.

## Volver a la versión anterior { #go-back-to-the-previous-version }

`docker exec evey-server evey admin rollback` muestra la versión en marcha,
la versión del esquema, la copia más reciente anterior a una migración y los
comandos exactos que hay que ejecutar en el equipo. No cambia nada: el
contenedor no tiene acceso a Docker.

La forma habitual de volver atrás es la imagen anterior más la copia hecha
antes de la actualización: consulta [Volver atrás](../backups.md#going-back).
Restaura la copia en lugar de deshacer la migración cuando la actualización
ha borrado datos (una columna eliminada, un cambio de tipo).
`evey admin migrate down`, en cambio, retrocede una migración cada vez,
cuando el `.down.sql` de ese paso basta.

!!! warning
    No modifiques `schema_migrations` a mano salvo para quitar un estado
    «dirty» (más abajo), y haz antes una copia.

## Una migración que se ha quedado a medias { #a-migration-that-stopped-halfway }

Cuando el registro muestra `Dirty database version N. Fix and force version.`,
una migración se ha interrumpido a mitad y `golang-migrate` se niega a
seguir. Lleva el esquema al estado que quieras y después quita la marca:

```sql
SELECT version, dirty FROM schema_migrations;

-- Vuelve a aplicar a mano el resto del up.sql, o deshazlo con el down.sql,
-- y después anota la versión a la que has llegado de verdad:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Reinicia el contenedor; las migraciones siguen desde ahí. Restaurar la copia
anterior a la migración ([Recuperación](recovery.md#restore-a-pre-migration-dump))
es la otra opción.

## Escribir una migración { #writing-a-migration }

Para quien contribuye. Los archivos están en
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.
Ejemplos ya publicados:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Toma `ACCESS EXCLUSIVE` en
  las dos tablas mientras dura el ADD. Rápido en tablas pequeñas, costoso en
  `media_items` / `watch_progress`.
- **`000067_sort_title_index.up.sql`**,
  **`000068_watch_progress_composite_index.up.sql`**,
  **`000069_quick_connect_shortcode_index.up.sql`** - un `CREATE INDEX`
  simple, que bloquea las escrituras en las tablas grandes.
  `CREATE INDEX CONCURRENTLY` no puede ejecutarse dentro de una transacción
  de migración.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT
  NULL DEFAULT '[]'`. Solo toca los metadatos en PG 11 o posterior, pero
  aun así mantiene `ACCESS EXCLUSIVE` un momento.

Pautas:

- **Un ALTER largo** que reescribe la tabla (cambio de tipo, valor por
  defecto no constante en un PG antiguo) significa una interrupción del
  servicio con un catálogo grande.
- **DROP COLUMN** solo toca los metadatos, pero la columna no puede volver
  sin una copia de seguridad. Nunca la borres en la versión que deja de
  leerla: deja de leerla, despliega, y bórrala en la versión siguiente.
- **Claves foráneas en tablas grandes**: `ADD CONSTRAINT ... NOT VALID`, y
  después `VALIDATE CONSTRAINT` en una migración posterior (la validación
  solo toma `SHARE UPDATE EXCLUSIVE`).
- **UPDATE masivo / relleno de datos**: nunca en una sola migración sobre una
  tabla grande. Publica una migración de esquema que no hace nada más una
  tarea en segundo plano.
- **Sustituir una columna (ampliar, migrar, reducir)**: no metas `ADD
  COLUMN` + relleno + `SET NOT NULL` + `DROP COLUMN` en un solo archivo. Esa
  forma solo funciona porque Evey funciona con una sola instancia, con las
  migraciones aplicadas de forma síncrona al arrancar; un despliegue
  escalonado devolvería errores desde la mitad de las instancias que
  todavía no esté actualizada. `000077`
  y `000098_playlists_profile_id.up.sql` se escribieron así y se quedan como
  están porque ya se han publicado. Para una nueva, repártela en tres
  versiones:
  1. **ampliar** - añade la columna nueva admitiendo NULL, escribe en las dos
     columnas, sigue leyendo la antigua;
  2. **migrar** - rellena la columna nueva (con una tarea en segundo plano,
     no con la migración), y después pasa a leer de ella;
  3. **reducir** - cuando ningún código desplegado toque ya la columna
     antigua, `SET NOT NULL` y bórrala.

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/en/admin.yaml - nav.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
