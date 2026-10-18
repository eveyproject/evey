---
source_sha: bcd1fe92a6c6837ce30d66edf387d4539752382b0a9f6038a14c15967da4eb32
reviewed: true
---
# Referencia de configuración { #configuration-reference }

Dónde está cada ajuste de un servidor Evey, qué valor tiene prioridad cuando un ajuste
existe a la vez en el panel de administración y en el entorno, y para qué
hace falta reiniciar. Las variables en sí están listadas por tema en las diez
páginas enlazadas más abajo; esta página es para el administrador que edita
el archivo compose o que se pregunta por qué una variable parece ignorada.

## Dos lugares, una regla { #two-places-one-rule }

**Lo que tiene que existir antes de que arranque el servidor** (secretos,
rutas de almacenamiento, la dirección pública, datos del proxy inverso) va en
el archivo compose como variables de entorno. **Lo que un administrador
ajusta después** (idiomas, límites de transcodificación, aceleración por
hardware, horarios, límites de los plugins) está en el panel de
administración, se guarda en la base de datos, sobrevive a una actualización
del contenedor y casi siempre se aplica sin reiniciar.

Para un ajuste que existe en los dos lugares, la prioridad es siempre la
misma:

1. **Tiene prioridad el valor guardado en el panel.** En cada arranque, el servidor lee
   primero el entorno y después sustituye cada campo por el valor guardado en
   la base de datos, si lo hay. Una variable nunca se impone a un valor
   guardado.
2. **La variable se aplica mientras no haya nada guardado.** El panel la
   muestra como valor por defecto actual; **Restablecer valor predeterminado**
   borra el valor guardado, y la variable vuelve a aplicarse a partir del
   siguiente arranque.
3. **El valor por defecto integrado** se aplica cuando no hay ninguno de los
   dos.

Los secretos son la excepción: el panel nunca muestra una contraseña ni una
clave que haya guardado, y una variable como `EVEY_TMDB_API_KEY` se copia en
la base de datos la primera vez que se usa, para que el panel pueda indicar
que hay una clave propia en uso.

## Qué necesita un reinicio { #what-needs-a-restart }

- Un cambio en una **variable de entorno** siempre exige recrear el
  contenedor (`docker compose up -d`): el proceso lee su entorno una sola
  vez.
- Un cambio en el **panel** se aplica en directo para la mayoría de los
  ajustes (nombre del servidor, nivel de registro, límites de peticiones,
  aceleración por hardware, SMTP, inicio de sesión único, acceso remoto,
  límites de los plugins, plazos de conservación en la siguiente limpieza
  diaria).
- El panel marca los ajustes que se **leen al arrancar** con *(se aplica al
  reiniciar)*: la vigilancia de bibliotecas y el intervalo de escaneo, el
  presupuesto de la caché de transcodificación, la tarjeta **Límites
  avanzados** de Transcodificación, los proveedores de intros y cada fila de
  **Entradas de configuración**. Guardar conserva tu elección, reiniciar la
  aplica.

Cada tabla de las páginas temáticas tiene una columna **Panel**: la ruta
hasta el campo (empezando por **Administración**) y si se aplica en directo o
al reiniciar, o `ninguno` cuando la variable no tiene equivalente en el
panel.

| Página | Contenido |
| --- | --- |
| [Servidor y red](environment/server.md) | Dirección pública, puertos, proxy inverso, acceso remoto, límites de peticiones, usuario del contenedor |
| [Base de datos y búsqueda](environment/database-search.md) | Postgres y Meilisearch integrados o externos |
| [Cuentas e inicio de sesión](environment/accounts.md) | Duración de las sesiones, clave de firma, inicio de sesión único |
| [Bibliotecas y metadatos](environment/libraries.md) | Escaneo, claves de metadatos, detección de intros |
| [Transcodificación y hardware](environment/transcoding.md) | Tarjeta gráfica, capacidad, límites de calidad, caché, clip de calibración |
| [Entrega de la reproducción y descargas](environment/playback-delivery.md) | Interruptores del empaquetado del flujo, descargas preparadas, botones para saltar del reproductor |
| [Trabajo en segundo plano](environment/background.md) | Pasadas en segundo plano y su ritmo |
| [Plugins y sandbox](environment/plugins.md) | Modo de sandbox, usuario aparte, límites de recursos, firmas |
| [Correo y notificaciones](environment/notifications.md) | SMTP, claves de las notificaciones push del navegador |
| [Copias de seguridad, registros y supervisión](environment/logs-monitoring.md) | Copias de seguridad, registro, conservación del historial, métricas y trazas |

## Cómo se leen los valores { #how-values-are-read }

- Los booleanos aceptan `1`, `t`, `true`, `0`, `f`, `false`. Las duraciones
  necesitan una unidad: `30s`, `5m`, `2h` (un `30` solo se ignora). Las
  listas van separadas por comas.
- Un valor que el servidor no sabe interpretar se **ignora con un aviso** en
  el registro (`environment variable ignored, value could not be parsed`) y
  se aplica el valor por defecto. Revisa las primeras líneas del registro
  después de editar el archivo compose.
- `EVEY_DB_PASSWORD` y `MEILI_MASTER_KEY` también pueden venir de un archivo
  indicado por la misma variable con el sufijo `_FILE`; un valor explícito
  tiene prioridad sobre el archivo.

## Secretos { #secrets }

El Postgres y el Meilisearch integrados generan sus secretos en el primer
arranque, en `./evey-data/keys` (`postgres.password`, `meili.key`, modo
0600). El servidor los lee a través de `EVEY_DB_PASSWORD_FILE` y
`MEILI_MASTER_KEY_FILE`, definidas en la imagen, y por eso los comandos
`evey admin` no necesitan contraseña. Define `EVEY_DB_PASSWORD` o
`MEILI_MASTER_KEY` solo para una base de datos o un motor de búsqueda
externos.

Evey trae una clave TMDB integrada. `EVEY_TMDB_API_KEY` define la tuya como valor inicial; la
clave definida en **Administración → Configuración del servidor → Claves
API** tiene prioridad sobre ella.

## Variables que no necesitas { #variables-you-do-not-need }

Líneas que a menudo se conservan de instalaciones antiguas, y por qué no
cambian nada:

| Línea | Por qué no hace falta |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | La imagen ya las define como `/data/logs`, `/cache/transcode`, `/data/trickplay`. Repetirlas no cambia nada. |
| `EVEY_FORWARDED_PROTO` | Obsoleta: se deduce del esquema de `EVEY_SERVER_BASE_URL`. Definirla solo escribe un aviso de obsolescencia en el registro. |
| `EVEY_KEYFRAME_BACKFILL=on` | Cualquier valor distinto de `off` la activa, también si no está definida. |
| `LIBVA_DRIVER_NAME` | libva elige el controlador a partir del id PCI de la GPU. Fuérzalo solo si tu tarjeta necesita un controlador que libva no elige (compruébalo con `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` elige el confinamiento más fuerte que admite tu equipo. Defínela explícitamente solo si quieres la garantía de *cierre por defecto*: `landlock` se niega a ejecutar un plugin que no puede confinar, mientras que `auto` baja de nivel con un aviso. |
| un `group_add` para `/dev/dri/card0` | Evey solo abre `renderD128`. Compruébalo con `ls -ln /dev/dri` y añade solo ese grupo. |

## Propietario de los archivos (PUID y PGID) { #file-ownership-puid-and-pgid }

**Por defecto: automático.** Cuando `PUID`/`PGID` no están definidos, el
punto de entrada mira al arrancar quién es el propietario de tus montajes
`/media*` y ejecuta el servidor con ese usuario. Así, las funciones que
*escriben en* tu biblioteca (la reparación del índice de posicionamiento
reconstruye los archivos en su sitio) funcionan desde el principio, sin
`chown` ni configuración. El registro de arranque te dice qué eligió:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Solo cuenta un `/media*` realmente montado: el `/media` vacío de la propia
imagen (toda imagen Debian trae uno) se ignora, así que una biblioteca guardada
en otro sitio, por ejemplo bajo `/data`, arranca sin ningún aviso de propiedad.

Los montajes de medios que pertenecen a **usuarios distintos** se gestionan
lo mejor posible: el servidor adopta el usuario propietario de más montajes y
se une a los grupos de los demás montajes como grupos adicionales, así que
las reparaciones en esos montajes funcionan cuando sus archivos tienen
permiso de escritura para el grupo (el aviso de arranque indica qué montajes
podrían seguir rechazando la escritura). La detección automática solo se
desactiva del todo (vuelve a `1000:1000` con un aviso) cuando todos los montajes
de medios pertenecen a root o al uid reservado `1001`: cambia el propietario
de tus medios a un usuario normal con chown, o define `PUID`/`PGID`
explícitamente, bajo `environment:` en el archivo compose:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      PUID: "1026"   # ejecuta el servidor con este id de usuario  (en el equipo: `id -u <media-owner>`)
      PGID: "100"    # ...y este id de grupo
```

Los valores explícitos siempre tienen prioridad sobre la detección.

Cómo funciona: el contenedor arranca como root solo el tiempo justo para
reasignar su usuario interno (y devolver al usuario la propiedad de las
carpetas de estado de Evey, `/data` y `/cache`, **nunca** tus montajes de
medios), y después renuncia a los privilegios. Los grupos adicionales dados
con `group_add:` en el compose (por ejemplo el grupo render de tu equipo para
la transcodificación por hardware con `/dev/dri`) se conservan después del
cambio. El proceso del servidor nunca se ejecuta como root.

Alternativa sin nada de root: si ni siquiera ese uso temporal de root te
parece aceptable, define `user: "UID:GID"` en el servicio de tu archivo compose. El
punto de entrada detecta el arranque sin root y se salta la reasignación
(incluida la detección del propietario de los medios); entonces tienes que
mantener tú `./evey-data` con permiso de escritura para ese UID.

!!! note
    `PUID=1001` se rechaza: es el usuario interno `evey-plugin`, que se
    mantiene aparte para que los plugins nunca compartan la identidad del
    servidor. Los medios se montan en lectura y escritura por defecto para
    que las reparaciones funcionen directamente; añade `:ro` al volumen de
    medios si prefieres una biblioteca de solo lectura (entonces las
    reparaciones fallan con un error claro).

## Variables que el servidor no lee { #variables-the-server-does-not-read }

Algunas variables aparecen en archivos compose sin que el servidor las lea:

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR`: las usa el propio
  `docker-compose.yml` del repositorio (puerto publicado, dominio del Caddy
  incluido, ruta de los medios), no Evey. `EVEY_VERSION` es ahí la etiqueta
  de la imagen; el servidor solo la muestra en `/healthz` cuando su binario
  no lleva versión.
- `EVEY_WEB_DIR` (`/srv`, donde están los archivos de la interfaz web), y
  `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES`: corresponden a la organización de la imagen.
  Cambiarlas no está soportado.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER`: las definen el punto de
  entrada y el lanzador de plugins para su propio uso.
- `EVEY_BANNER=off` solo oculta el cartel de arranque en el registro;
  `NO_COLOR` le quita los colores.

<!-- sources
precedence: cmd/evey/main.go - loadOverride (DB row wins when non-empty) ; cmd/evey/wire_helpers.go - resolveFlag, resolveAPIKeyWithBuiltin (env seeded into server_config)
reset to default: internal/adapters/driving/http/settings_handler.go - reset / resetSideEffect / requiresRestart
parsing: internal/config/config.go - envBool, envDuration, envStrSlice, warnBadEnv, envSecret (_FILE)
restart: internal/adapters/driving/http/settings_handler.go (requiresRestart) ; i18n admin.transcoding.advancedRestartHint ; admin.serverConfig.appliedOnRestart ; admin.serverConfig.introProvidersRestart
variables: internal/config/config.go ; docker/entrypoint.sh ; Dockerfile ; docker-compose.yml ; cmd/evey/banner.go
-->
