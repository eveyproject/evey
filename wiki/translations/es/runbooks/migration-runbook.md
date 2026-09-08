---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
---

# Guía de migraciones

Cómo ejecuta Evey las migraciones de esquema y cómo operar sobre ellas sin riesgo.

## Cómo se ejecutan las migraciones

Las migraciones van **incrustadas** en el binario (`go:embed`) y se ejecutan solas al arrancar el servidor, desde `internal/adapters/driven/postgres/migrate.go`, con `golang-migrate/migrate/v4`. Fuente: `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.

Las migraciones se aplican solas cuando arranca `evey-server`. **Además** hay una CLI para el control manual: `evey admin migrate up | down | version` (está en `cmd/evey/admin.go`; `version` imprime la versión actual del esquema y sale con 1 si la base de datos está marcada `dirty`). Las reglas `migrate-up` y `migrate-down` del `Makefile` la envuelven a través de `./cmd/evey admin migrate`. Se hace un `pg_dump` previo a la migración de forma automática cuando `EVEY_BACKUP_DIR` está definida (pase `--no-backup` para saltárselo).

El estado se guarda en la tabla `schema_migrations`, que gestiona golang-migrate: `{version, dirty}`.

## Lista de comprobación antes de migrar

1. **Copie la base de datos.** Evey trae un extremo de copia de seguridad (véase `docs/backups.md`):
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   O lance `pg_dump` directamente contra el contenedor:
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   Compruebe que el volcado no está vacío y que pasa `gunzip -t`.

2. **Mire el tamaño de las tablas** si la migración toca alguna grande (media_items, watch_progress, tracks, hls_sessions):
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Mire las sesiones y los bloqueos activos** antes de una migración con mucho DDL:
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   Mate las sesiones largas o en `idle in transaction` (`SELECT pg_terminate_backend(pid)`) antes de lanzar una migración que tome bloqueos fuertes.

4. **Mire la versión de migración actual**:
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   Si `dirty = true`, siga la sección de recuperación antes de hacer ninguna otra cosa.

## Lanzar una migración

Las migraciones se ejecutan al arrancar el servidor. Para aplicar las pendientes:

```sh
docker compose pull evey       # si va a desplegar una imagen nueva
docker compose up -d evey
docker logs -f evey-server     # siga el registro de arranque
```

Ha ido bien si no hay errores de migración y el servidor responde 200 en `/healthz`. Compruébelo:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## Vuelta atrás

Retroceda un paso con la regla ya preparada:

```sh
make migrate-down     # envuelve `evey admin migrate down`, retrocede EXACTAMENTE un paso
make migrate-version  # confirme dónde ha aterrizado
```

Repítala una vez por paso para seguir retrocediendo. Se hace un `pg_dump` previo
a la migración de forma automática cuando `EVEY_BACKUP_DIR` está definida.

La otra vía es **volver a desplegar la imagen anterior**, cuyas migraciones
incrustadas se detienen en la versión N-1.

> No edite `schema_migrations` a mano. Una revisión anterior de esta guía
> afirmaba que no existía ninguna regla `migrate-down` y mandaba al operador
> aplicar el `.down.sql` con `psql` y después hacer a mano un
> `UPDATE schema_migrations SET version = ...`, en plena incidencia. Se
> contradecía con la sección «Cómo se ejecutan las migraciones», nueve líneas
> más arriba. La regla ha existido siempre (`Makefile:314`). Recurra a la
> cirugía manual solo si la base de datos está marcada `dirty` y
> `migrate down` se niega a ejecutarse, y haga un volcado antes.

Restaure siempre desde la copia previa a la migración si se han desplegado cambios que destruyen datos (DROP COLUMN, cambios de tipo) y esa copia es más reciente que el punto en el que el tráfico de producción se separó.

## Recuperación: estado `dirty`

Si el registro de arranque dice `Dirty database version N. Fix and force version.`, la última migración se cayó a medias. Arregle el esquema a mano hasta dejarlo en el estado que quiera y limpie después la marca:

```sql
-- Inspección: ¿qué dice ahora mismo schema_migrations?
SELECT version, dirty FROM schema_migrations;

-- O bien vuelva a aplicar a mano las sentencias del up.sql, o bien
-- deshaga con el down.sql. Después limpie el bit dirty en la versión
-- a la que haya llegado de verdad:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Reinicie `evey-server`. golang-migrate continuará desde ahí.

## Casos particulares

Migraciones recientes como referencia (véase `internal/adapters/driven/postgres/migrations/`):

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Toma un `ACCESS EXCLUSIVE` sobre las dos tablas mientras dura el ADD. Barato en tablas pequeñas, doloroso en `media_items` y `watch_progress`. El patrón para tablas grandes: añadir con `NOT VALID` y hacer después un `VALIDATE CONSTRAINT` en una segunda sentencia (validar solo toma `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - un `CREATE INDEX` pelado. En tablas grandes bloquea las escrituras. Plantéese un `CREATE INDEX CONCURRENTLY` a mano durante una ventana de mantenimiento (no puede ejecutarse dentro de la transacción de migrate: aplíquelo fuera de banda y después anote la versión).
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. En PG 11 y posteriores esto es solo metadatos y va rápido. Aun así toma un `ACCESS EXCLUSIVE` un instante.

Pautas:

- **Un ALTER largo** que reescriba la tabla (cambio de tipo, añadir un valor por defecto no constante en un PG antiguo) → comprobación de bloqueos obligatoria, y cuente con una parada del servicio.
- **DROP COLUMN** → son solo metadatos, pero la columna no se puede recuperar sin una copia. No la elimine nunca en la misma entrega en la que se despliega una versión nueva del código: mejor en dos pasos, primero dejar de leer la columna, desplegar, y eliminarla en la entrega siguiente.
- **Una clave ajena sobre tablas grandes** → `ADD CONSTRAINT ... NOT VALID` y un `VALIDATE CONSTRAINT` en una migración posterior. Evite un `ADD CONSTRAINT ... FOREIGN KEY ...` único y bloqueante.
- **UPDATE por lotes o relleno** → nunca dentro de una sola migración sobre una tabla grande. Entregue una migración de esquema que no haga nada más una tarea en segundo plano.
- **Sustitución de una columna (expand → migrate → contract)** → no meta en una sola migración el `ADD COLUMN`, el relleno, el `SET NOT NULL` y el `DROP COLUMN` de la antigua. Esa forma solo funciona porque hoy Evey corre en una sola instancia y las migraciones se aplican de forma síncrona al arrancar: durante esa ventana no pueden convivir el código viejo leyendo la columna eliminada y el nuevo leyendo la nueva, así que un despliegue progresivo o con varias réplicas serviría errores desde la mitad que fuera por detrás.
  Hay migraciones escritas así, que se dejan como están porque ya se entregaron: `000077` y `000098_playlists_profile_id.up.sql` (añade `profile_id`, rellena, pone `NOT NULL` y elimina `user_id`).
  Para una nueva, repártala en tres entregas:
  1. **expandir** - añada la columna nueva como anulable, escriba en *las dos* columnas y siga leyendo la antigua;
  2. **migrar** - rellene la columna nueva (con una tarea en segundo plano, no con la migración) y pase después las lecturas a ella;
  3. **contraer** - cuando ningún código desplegado toque ya la columna antigua, ponga `NOT NULL` y elimínela.
  Cada paso se despliega y se deshace por separado, cosa que la versión de un solo archivo no permite.

## Comprobaciones después de migrar

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Pruebe por encima unos cuantos extremos afectados por la migración (`/api/v1/...`).
