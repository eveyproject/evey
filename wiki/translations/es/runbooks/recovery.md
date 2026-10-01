---
source_sha: 64f12b2cc77ed7069bf33f581c2d91e3b13b6ff04cc4a3bc62db72c3607b46fe
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
- Todas las sesiones del usuario terminan: se eliminan sus tokens de
  actualización y se revocan sus tokens de acceso. Una aplicación abierta lo
  nota en unos 30 segundos y pide volver a iniciar sesión.
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
- Todas las sesiones terminan de inmediato, sin esperar al reinicio: el comando
  elimina todos los tokens de actualización y revoca todos los tokens de acceso
  en la base de datos. Las aplicaciones abiertas lo notan en unos 30 segundos y
  todos vuelven a iniciar sesión.
- Si no se puede contactar con la base de datos, la clave se aparta igualmente y
  el comando lo indica: vuelve a ejecutarlo cuando la base responda.

Cuándo usarlo:
- Sospecha de fuga de la clave: nadie puede ya emitir tokens de acceso válidos
  con la clave antigua.
- Cierre masivo de sesiones. Para una sola cuenta basta con desactivarla en
  **Admin → Usuarios**, o con `reset-password`.

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
