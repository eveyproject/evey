---
source_sha: ee8527dd07b1c7262f2f353e0f33f6004d9ae87c801d3274d9c9a31c109637be
reviewed: false
---
# Manual de recuperación

Procedimientos de CLI para los casos en que ya no puedes iniciar sesión, se
sospecha que la clave JWT está comprometida o hay que devolver la base de datos a
una copia previa a una migración.

Todos los comandos se ejecutan como `evey admin <subcommand>` y leen la
configuración de las mismas variables `EVEY_*` que el servidor. **No** arrancan
el servidor: la mayoría se pueden ejecutar sin riesgo con el servidor parado.

## reset-password

Genera una contraseña nueva para un usuario existente. La nueva contraseña se
muestra una sola vez en la salida estándar: cópiala enseguida.

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
- Se actualiza el `updated_at` del usuario, pero no cambia ningún otro estado.
- Las sesiones existentes siguen siendo válidas hasta que caducan de forma
  natural. Para cerrar la sesión del usuario en todas partes, desactiva la cuenta
  en **Admin → Usuarios** (**Desactivar**) y vuelve a activarla: desactivarla
  borra sus tokens de actualización e invalida sus tokens de acceso de inmediato.
- La contraseña en claro nunca se registra. Si pierdes la salida, tienes que
  volver a ejecutar el comando.

## rotate-jwt-key

Sustituye la clave RSA que firma los tokens de acceso.

```sh
evey admin rotate-jwt-key
```

Comportamiento:
- El archivo de clave actual (ruta = `EVEY_JWT_PRIVATE_KEY_PATH`, por defecto
  `/data/jwt.key`) se renombra a `<path>.rotated-<UTC-timestamp>`.
- En el siguiente arranque del servidor, `LoadOrGenerateRSAKey` genera una clave
  RSA nueva de 2048 bits.
- Todos los tokens de acceso emitidos antes fallan la verificación de firma.
- Los tokens de actualización se comprueban contra el hash guardado en la base de
  datos, no contra la clave, así que siguen siendo válidos: una aplicación cuyo
  token de acceso se rechaza lo renueva y sigue funcionando. Para terminar también
  todas las sesiones, vacía la tabla:
  ```sql
  DELETE FROM refresh_tokens;
  ```

Cuándo usarlo:
- Sospecha de fuga de la clave: nadie puede ya emitir tokens de acceso válidos
  con la clave antigua.
- Cierre masivo de sesiones: junto con el `DELETE` de arriba. Para una sola
  cuenta, basta con desactivarla en **Admin → Usuarios**.

## restore-backup

Aplica un `pg_dump.gz` producido por el mecanismo de copia previa a la migración
(o de cualquier otra fuente) a la base de datos configurada. **Sobrescribe los
datos existentes.** El archivo debe terminar en `.sql.gz`: las copias `.tar.gz`
de **Admin → Copia de seguridad y restauración** se restauran desde el panel, no
con este comando.

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Procedimiento:
1. El comando espera 5 segundos antes de hacer nada: Ctrl+C lo cancela.
2. El archivo pasa por `gunzip | psql` contra la base de datos configurada, con
   `--single-transaction` y `ON_ERROR_STOP=1`.
3. Ante cualquier error, psql revierte la transacción: la base de datos queda en
   su estado anterior.

Requisitos:
- `psql` y `gunzip` deben estar en el PATH (ambos vienen en la imagen de
  producción junto a `pg_dump`).
- La base de datos de destino debe existir; el volcado solo contiene DROP/CREATE
  para las tablas, no para la propia base de datos.
- El `EVEY_DB_USER` configurado debe ser propietario de la base de datos de
  destino.

Las copias previas a una migración se generan automáticamente antes de cada
`migrate.Up()` y se guardan en `EVEY_BACKUP_DIR` (por defecto `/data/backups`).
Se conserva el volcado más reciente de cada una de las 3 últimas versiones del
esquema; los demás se eliminan.
