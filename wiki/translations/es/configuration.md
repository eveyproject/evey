---
source_sha: 4346505ac20853c385d4b6641af12c128cdd868479ccef57a064e5b9e0901fce
reviewed: false
---
# Referencia de configuración

Dos sitios, una regla. **Lo que debe existir antes de que arranque el servidor**
(secretos, almacenamiento, la dirección pública, datos del proxy inverso) va en el
archivo compose. **Lo que un administrador ajusta después** (idiomas, límites de
transcodificación, aceleración por hardware, programaciones, límites de plugins)
vive en el panel de administración, se guarda en la base de datos, sobrevive a
una actualización del contenedor y en su mayoría se aplica sin reiniciar. Para un
ajuste que existe en ambos sitios, una variable de entorno solo inicializa el
primer arranque: prevalece el valor guardado en el panel.

Las variables, por tema:

| Página | Cubre |
| --- | --- |
| [Servidor y red](environment/server.md) | Dirección pública, puertos, proxy inverso, acceso remoto, límites de peticiones, usuario del contenedor |
| [Base de datos y búsqueda](environment/database-search.md) | Postgres y Meilisearch integrados o externos |
| [Cuentas e inicio de sesión](environment/accounts.md) | Duración de las sesiones, clave de firma, inicio de sesión único |
| [Bibliotecas y metadatos](environment/libraries.md) | Escaneo, claves de metadatos, detección de intros |
| [Transcodificación y hardware](environment/transcoding.md) | Tarjeta gráfica, capacidad, límites de calidad, caché, clip de calibración |
| [Entrega de la reproducción y descargas](environment/playback-delivery.md) | Interruptores del empaquetado de flujos, descargas preparadas, botones de salto del reproductor |
| [Tareas en segundo plano](environment/background.md) | Pasadas en segundo plano y su ritmo |
| [Plugins y sandbox](environment/plugins.md) | Modo de sandbox, usuario aparte, límites de recursos, firmas |
| [Correo y notificaciones](environment/notifications.md) | SMTP, claves de push del navegador |
| [Copias, registros y supervisión](environment/logs-monitoring.md) | Copias de seguridad, registro, retención del historial, métricas y trazas |

## Secretos

El Postgres y el Meilisearch integrados generan sus secretos en el primer
arranque, en `./evey-data/keys` (`postgres.password`, `meili.key`, modo 0600). El
servidor los lee mediante `EVEY_DB_PASSWORD_FILE` y `MEILI_MASTER_KEY_FILE`,
definidas en la imagen, y por eso los comandos `evey admin` no necesitan
contraseña. Define `EVEY_DB_PASSWORD` o `MEILI_MASTER_KEY` solo para una base de
datos o un motor de búsqueda externos; un valor explícito prevalece sobre el
archivo.

Viene integrada una clave de TMDB. `EVEY_TMDB_API_KEY` inicializa la tuya; la
clave definida en **Admin → Configuración del servidor → Claves API** prevalece
sobre ella.

## Variables que no necesitas

Líneas que a menudo se arrastran de configuraciones antiguas, y por qué no cambian
nada:


| Línea | Por qué es innecesaria |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | La imagen ya las define como `/data/logs`, `/cache/transcode`, `/data/trickplay`. Repetirlas no cambia nada. |
| `EVEY_FORWARDED_PROTO` | Obsoleta: se deduce del esquema de `EVEY_SERVER_BASE_URL`. Definirla solo registra un aviso de obsolescencia. |
| `EVEY_KEYFRAME_BACKFILL=on` | Cualquier valor distinto de `off` la activa, incluida su ausencia. |
| `LIBVA_DRIVER_NAME` | libva elige el controlador a partir del id PCI de la GPU. Fuérzalo solo si tu tarjeta necesita un controlador que libva no elige (compruébalo con `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` elige el confinamiento más fuerte que admite tu equipo. Defínelo de forma explícita solo cuando quieras una garantía de *fallo en cerrado*: `landlock` se niega a ejecutar un plugin que no puede confinar, mientras que `auto` se degrada con un aviso. |
| un `group_add` para `/dev/dri/card0` | Evey solo abre `renderD128`. Compruébalo con `ls -ln /dev/dri` y añade solo ese grupo. |


## Propiedad de los archivos (PUID y PGID)

**Por defecto: automática.** Cuando `PUID`/`PGID` no están definidos, el
entrypoint mira al arrancar el propietario de tus montajes `/media*` y ejecuta el
servidor como ese usuario, para que las funciones que *escriben en* tu biblioteca
(la reparación del índice de posicionamiento reconstruye archivos en el sitio)
funcionen de serie, sin `chown` ni configuración. El registro de arranque te dice
qué se eligió:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Los montajes de medios que pertenecen a **usuarios distintos** se tratan lo mejor
posible: el servidor adopta el usuario propietario del mayor número de montajes y
se une a los grupos de los demás montajes como grupos suplementarios, así que las
reparaciones en esos montajes funcionan cuando sus archivos tienen permiso de
escritura para el grupo (el aviso de arranque lista qué montajes pueden seguir
rechazando escrituras). La detección automática solo se aparta del todo (vuelve a
`1000:1000` con un aviso) cuando todos los montajes de medios pertenecen a root o
al uid reservado `1001`: cambia el propietario de tus medios a un usuario normal,
o define `PUID`/`PGID` de forma explícita:

```env title=".env"
PUID=1026   # ejecutar el servidor con este id de usuario  (equipo: `id -u <media-owner>`)
PGID=100    # ...y este id de grupo
```

Los valores explícitos siempre prevalecen sobre la detección.

Cómo funciona: el contenedor arranca como root el tiempo justo para reasignar su
usuario interno (y devolver la propiedad de los directorios de estado de Evey,
`/data` y `/cache`; **nunca** tus montajes de medios), y después abandona los
privilegios. Los grupos suplementarios concedidos con `group_add:` en el compose
(por ejemplo el grupo render de tu equipo para la transcodificación por hardware
con `/dev/dri`) se conservan tras el cambio. El proceso del servidor nunca se
ejecuta como root.

Alternativa sin root: si incluso ese root transitorio te resulta inaceptable,
define en su lugar `user: "UID:GID"` en el servicio de tu archivo compose. El
entrypoint detecta el arranque sin root y se salta la reasignación (incluida la
detección del propietario de los medios): mantener `./evey-data` con permiso de
escritura para ese UID pasa entonces a ser cosa tuya.

Notas:
- `PUID=1001` se rechaza: es el usuario interno `evey-plugin`, que se mantiene
  aparte para que los plugins nunca puedan compartir la identidad del servidor.
- Los medios se montan con lectura y escritura por defecto para que las
  reparaciones funcionen de forma nativa; añade `:ro` al volumen de medios si
  prefieres una biblioteca de solo lectura (las reparaciones fallarán entonces
  con un error claro).

## Variables que el servidor no lee

Algunas variables aparecen en los archivos compose sin que el servidor las lea:

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR`: las usa el propio
  `docker-compose.yml` del repositorio (puerto publicado, dominio del Caddy
  incluido, ruta de los medios), no Evey. `EVEY_VERSION` es ahí la etiqueta de la
  imagen; el servidor solo la muestra en `/healthz` cuando su binario no lleva
  versión.
- `EVEY_WEB_DIR` (`/srv`, donde están los archivos de la interfaz web), y
  `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES`: corresponden a la disposición de la imagen. Cambiarlas
  no está soportado.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER`: las definen el entrypoint y el
  lanzador de plugins para su propio uso.
