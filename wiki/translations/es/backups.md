---
source_sha: 00ac3f52637e817e4e294aad7fa5cbaf5e77c3c6a343d1ffc784da076a90fe56
reviewed: true
---
# Copias de seguridad y actualizaciones { #backups-and-updates }

Dos costumbres mantienen tu servidor a salvo:

- **Hacer copias de seguridad.** Activa una copia diaria de la base de
  datos, y guarda una copia de la carpeta `evey-data` en otro sitio.
- **Actualizar.** Un solo comando descarga la versión nueva. Evey guarda
  antes la base de datos, por su cuenta, cada vez que una actualización la
  modifica.

Esta página explica las dos cosas, y después qué contiene una copia, cómo
restaurarla y cómo volver a una versión anterior.

## Programar las copias de seguridad { #schedule-backups }

1. Ve a **Administración → Copia de seguridad y restauración**, tarjeta
   **Programación**.
2. En **Expresión cron**, indica cuándo se hace. `0 3 * * *` significa todos
   los días a las 3 de la madrugada. (El formato es minuto, hora, día, mes,
   día de la semana.)
3. En **Retención (días)**, indica cuánto tiempo se guarda cada copia, por
   ejemplo `30`. Las copias más antiguas se borran después de cada
   ejecución programada.
4. Marca **Activado** y pulsa **Guardar**.

**Ejecutar ahora** hace una copia en el momento, igual que **Crear copia de
seguridad** en la parte de arriba de la página.

<figure class="evey-shot" markdown="span">
  ![Administración → Copia de seguridad y restauración con una programación diaria](images/admin-backup.webp){ loading=lazy }
  <figcaption>Una copia todos los días a las 3 de la madrugada, conservada 30 días (interfaz en inglés).</figcaption>
</figure>

Las copias se guardan en `./evey-data/backups`. Está dentro de la carpeta
que de todos modos deberías copiar a otro disco (ver
[Qué contiene una copia](#what-a-backup-contains)).

??? info "Desde la línea de comandos"

    Un script o una tarea cron en la máquina pueden hacer la misma copia:

    ```bash
    docker exec evey-server evey admin backup
    ```

    Sigue el interruptor de telemetría del panel (más abajo);
    `--include-telemetry` incluye la telemetría solo en esa ejecución.

El servidor también guarda la base de datos por su cuenta antes de que una
actualización la modifique, sea cual sea la programación (ver
[Actualizar Evey](#updating-evey)).

## Qué contiene una copia { #what-a-backup-contains }

Una copia de seguridad es una copia de la **base de datos**: cuentas,
perfiles, bibliotecas y su catálogo, historial de reproducción y puntos de
reanudación, colecciones, listas de reproducción y favoritos, ajustes,
registros de plugins, webhooks, claves API, notificaciones y registros de
auditoría.

**No** contiene:

- tus archivos de vídeo, música y fotos;
- la caché de vídeo (se vuelve a llenar sola);
- el índice de búsqueda (se reconstruye a partir de la base de datos);
- las claves guardadas en `./evey-data`: `jwt.key`, que protege todos los
  inicios de sesión, la carpeta `keys` (contraseña de la base de datos,
  clave de búsqueda) y `hls_aes.key`, la clave que cifra los segmentos de
  streaming (se crea en el primer arranque, permisos 0600);
- los archivos de los plugins instalados.

**Así que guarda también una copia de toda la carpeta `./evey-data`.** Una
copia de la base de datos restaura Evey en la misma instalación. Para
reconstruir una máquina desde cero, necesitas la carpeta.

??? info "La telemetría y el formato del archivo"

    Las mediciones de reproducción y del servidor (las que hay detrás de los
    gráficos de calidad y de carga) se dejan fuera por defecto: son lo que
    más crece y no dicen nada de tu catálogo. **Incluir telemetría en las
    copias de seguridad** (**Administración → Configuración del servidor**)
    las vuelve a incluir, en las copias manuales, programadas y desde la
    línea de comandos. Restaurar una copia siempre vacía esas tablas.

    Una copia es un archivo `.tar.gz` que contiene el volcado de la base de
    datos y una breve descripción con su suma de verificación (SHA256) y su
    versión de esquema.

## Restaurar una copia { #restore-a-backup }

**Administración → Copia de seguridad y restauración** muestra la lista de
copias. Restaura una desde la lista, o sube un archivo con **Restaurar desde
archivo**. Restaurar sustituye la base de datos actual: se pierde todo lo
que haya pasado desde esa copia.

!!! warning "Solo una copia de una versión con la misma estructura de base de datos"
    El servidor comprueba primero el archivo y la estructura de la base de
    datos (la versión de esquema). Rechaza una copia hecha por una versión
    con una estructura distinta, en lugar de cargarla. Para volver a una
    versión anterior con sus datos, ver [Volver atrás](#going-back).

Si el servidor ya no arranca, la línea de comandos puede restaurar las
copias de seguridad hechas antes de una actualización
(`pre-migration-*.sql.gz`), no las copias `.tar.gz` de este panel: ver
[Recuperación](runbooks/recovery.md).

## Actualizar Evey { #updating-evey }

En la carpeta que contiene tu archivo compose:

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

Eso es toda la actualización.

- **Primero se guardan tus datos.** Cuando la versión nueva necesita
  modificar la base de datos, primero escribe una copia en
  `./evey-data/backups/pre-migration-*.sql.gz` (se guardan las tres más
  recientes), y después hace el cambio al arrancar.
- **Puede tardar unos minutos** con un catálogo grande. El contenedor
  aparece como sano (healthy) en cuanto vuelve a responder.
- **Evey te avisa cuando hay una actualización.** Comprueba cada hora si se
  ha publicado una imagen más nueva y lo indica en el panel de
  administración. Nunca se actualiza solo.
- **Qué versión está funcionando:** `docker exec evey-server evey version`
  muestra la versión y el commit.

??? info "Fijar la versión exacta que usas"

    `latest` sigue la versión publicada más reciente y cambia con cada
    actualización. Para un servidor del que dependes, pon una etiqueta de
    versión (`0.1.0`) o una etiqueta fija `sha-<commit>` en la línea
    `image:`, para que dos descargas con un día de diferencia ejecuten el
    mismo código.

### Volver atrás { #going-back }

1. Pon la etiqueta anterior en la línea `image:` de tu archivo compose.
2. Si la versión más nueva modificó la base de datos, restaura primero su
   copia de antes de la actualización: `docker exec evey-server evey admin
   rollback` muestra los pasos exactos, con la ruta del archivo.
3. Ejecuta `docker compose up -d`.

Las etiquetas de imagen, las otras variantes y las versiones para tarjetas
gráficas se describen en
[Despliegues avanzados](operations/advanced-deployments.md#image-tags-and-variants).

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (sha-<short>, latest gated on vX.Y.Z)
schedule card controls (Enabled, Save, Run now): frontend/apps/web/src/routes/_auth/admin/backup.lazy.tsx ; i18n admin.backup.schedule.enabledLabel, save, cronHelp
-->
