---
source_sha: 15f4243e2aa41d1a370efb30c5607eb485799cf11a0be191ea0b7489684f4485
reviewed: true
---
# Recuperación

Abre esta página cuando ya nadie puede iniciar sesión, cuando la clave que
firma los tokens de acceso puede haberse filtrado, o cuando la base de datos
tiene que volver a una copia anterior a una migración. Cada procedimiento es
un comando `evey admin` que se ejecuta dentro del contenedor; la página
[Línea de comandos](cli.md) enumera los demás subcomandos.

## Antes de empezar

Los comandos leen la misma configuración `EVEY_*` que el servidor, así que
ejecutarlos en el contenedor del servidor no necesita nada más:

```sh
docker exec evey-server evey admin <subcommand>        # servidor en marcha
docker compose run --rm evey admin <subcommand>        # contenedor parado
```

La segunda forma arranca primero la base de datos integrada, después ejecuta
el comando y termina. Ninguno de estos comandos arranca el servidor.

## Dar una contraseña nueva a un usuario

Úsalo cuando un usuario no puede entrar en su cuenta, o justo después de
sospechar que una cuenta ha sido comprometida. La contraseña nueva se muestra
una sola vez y nunca se guarda en el registro: cópiala antes de cerrar el
terminal.

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

Así se ve un resultado correcto:

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

Todas las sesiones de ese usuario se cierran a la vez: los tokens de
renovación se borran y los tokens de acceso se revocan. Una app que esté
abierta lo nota en unos 30 segundos y pide volver a iniciar sesión. Si
pierdes lo que ha mostrado el comando, vuelve a ejecutarlo.

## Cambiar la clave que firma los tokens de acceso

Úsalo cuando el archivo de la clave puede haberse filtrado, o para cerrar la
sesión de todos los usuarios a la vez. Para una sola cuenta, basta con
**Administración → Usuarios → Desactivar** o con cambiar la contraseña como
se explica arriba.

```sh
docker exec evey-server evey admin rotate-jwt-key
```

Lo que pasa:

1. El archivo de la clave (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key` por
   defecto) se renombra a `<path>.rotated-<UTC timestamp>`.
2. Todos los tokens de renovación se borran y todos los tokens de acceso se
   revocan, en una sola transacción. Las apps abiertas lo notan en unos 30
   segundos; todo el mundo vuelve a iniciar sesión.
3. En su siguiente arranque, el servidor genera una clave RSA nueva de 2048
   bits.

Reinicia el contenedor después (`docker compose restart evey`) para que se
use la clave nueva. Si no se ha podido llegar a la base de datos, la clave se
aparta igualmente y el comando dice `key rotated, but sessions could not be
ended (restart the server, then run this again)`: haz exactamente eso.

## Restaurar una copia anterior a una migración

Úsalo cuando hay que deshacer una actualización y se pueden perder los datos
escritos desde entonces. El comando pasa una copia `.sql.gz` por
`gunzip | psql` hacia la base de datos configurada.

!!! warning
    La restauración sobrescribe las filas actuales. Haz antes una copia de
    seguridad del estado actual (**Administración → Copia de seguridad y
    restauración → Ejecutar ahora**) si hay alguna posibilidad de que lo
    necesites.

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

Lo que pasa:

1. El comando lee primero el archivo entero: tiene que descomprimirse sin
   errores y terminar con la línea de cierre de `pg_dump`. Una copia cortada
   o corrupta se rechaza (`refusing to restore: ...`) antes de borrar nada.
2. Después espera 5 segundos antes de tocar nada: Ctrl+C lo cancela.
3. `psql` aplica la copia con `--single-transaction` y `ON_ERROR_STOP=1`. Ante
   cualquier error, la transacción se deshace y la base de datos se queda
   como estaba.
4. Si todo va bien, la última línea es `[admin] restored backup <path>`.

Requisitos:

- El nombre del archivo tiene que terminar en `.sql.gz`. Los archivos
  `.tar.gz` de **Administración → Copia de seguridad y restauración** se
  restauran desde el panel, no con este comando.
- La base de datos de destino tiene que existir; la copia borra y vuelve a
  crear las tablas, no la base de datos.
- `psql` y `gunzip` tienen que estar en el PATH, cosa que garantiza la
  imagen.

Las copias anteriores a una migración se escriben solas antes de cada cambio
del esquema, en `EVEY_BACKUP_DIR` (`/data/backups`, es decir,
`./evey-data/backups` en el equipo), con el nombre
`pre-migration-<schema version>-<UTC timestamp>.sql.gz`. Se guarda la copia
más reciente de cada una de las 3 últimas versiones del esquema, y las demás
se borran. [Migraciones de la base de datos](migration-runbook.md) explica
cuándo restaurar una en lugar de hacer retroceder el esquema.

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete (whole gzip + completion marker before anything runs), 5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
