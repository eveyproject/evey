---
source_sha: 75f950590ae90c9a9a77cfcea37f93cbbe8eb70a0dbea1df56d154a205d2fc12
---

# Guía de recuperación

Procedimientos de línea de órdenes para cuando ya no puede iniciar sesión,
sospecha que la clave JWT se ha filtrado o hay que devolver la base de datos a
una copia previa a la migración.

Todas las órdenes se ejecutan como `evey admin <subcommand>` y leen la
configuración de las mismas variables `EVEY_*` que el servidor. **No** arrancan
el servidor: casi todas se pueden ejecutar con el servidor parado.

## reset-password

Genera una contraseña nueva para un usuario que ya existe. La contraseña se
imprime una sola vez por stdout: cópiela en el acto.

```sh
evey admin reset-password --user admin@example.com
```

Salida:
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

Notas:
- Se actualiza el `updated_at` del usuario, y nada más.
- Las sesiones abiertas siguen siendo válidas hasta que caduquen solas. Si
  además quiere cerrar la sesión de ese usuario en todas partes, ejecute
  después `rotate-jwt-key`.
- La contraseña en claro no se registra en ningún sitio. Si pierde la salida,
  vuelva a ejecutar la orden.

## rotate-jwt-key

Invalida a la fuerza todas las sesiones activas, sustituyendo la clave RSA de
firma.

```sh
evey admin rotate-jwt-key
```

Qué hace:
- Renombra el archivo de clave actual (la ruta es
  `EVEY_JWT_PRIVATE_KEY_PATH`, por defecto `/data/jwt.key`) a
  `<path>.rotated-<UTC-timestamp>`.
- En el siguiente arranque del servidor, `LoadOrGenerateRSAKey` genera una
  clave RSA de 2048 bits nueva.
- Todos los tokens de acceso emitidos hasta entonces fallan la verificación de
  firma → todo el mundo queda con la sesión cerrada.
- Los tokens de refresco son JWT firmados con esa misma clave, así que también
  mueren: no hay que limpiar nada aparte.

Cuándo usarla:
- Sospecha de filtración de la clave.
- Cierre masivo de sesiones (respuesta a un incidente, salida de un
  colaborador externo).

## restore-backup

Aplica sobre la base de datos configurada un `pg_dump.gz` producido por el
mecanismo de copia previa a la migración, o por cualquier otro medio.
**Sobrescribe los datos existentes.**

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Procedimiento:
1. La orden espera 5 segundos antes de tocar nada: Ctrl+C aborta.
2. El archivo se pasa por `gunzip | psql` contra la base de datos configurada,
   con `--single-transaction` y `ON_ERROR_STOP=1`.
3. Ante cualquier error, psql revierte la transacción y la base de datos se
   queda como estaba.

Requisitos:
- `psql` y `gunzip` tienen que estar en el PATH (los dos vienen en la imagen de
  producción, junto a `pg_dump`).
- La base de datos de destino tiene que existir: el volcado solo trae los
  DROP/CREATE de las tablas, no de la base de datos.
- El `EVEY_DB_USER` configurado tiene que ser el propietario de la base de datos
  de destino.

Las copias previas a la migración se hacen solas antes de cada `migrate.Up()` y
viven bajo `EVEY_BACKUP_DIR` (por defecto `/data/backups`). Se conservan las 3
más recientes; las anteriores se van podando.
