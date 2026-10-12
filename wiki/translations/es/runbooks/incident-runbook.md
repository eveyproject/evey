---
source_sha: 95bca5df52c2505d699a3857e7d305e19067387a32df91af30da948dfa493ae0
reviewed: true
---
# Manual de incidentes

Qué hacer primero ante las averías más comunes de Evey, para el operador de
guardia. Busca la sección que corresponde al síntoma, haz sus comprobaciones
y aplica la solución que corresponda a lo que hayas encontrado. Todas las
secciones tienen la misma forma: **Síntoma**, **Diagnóstico**, **Solución**.

## Antes de empezar

Los comandos están pensados para la instalación estándar: un solo
contenedor, `evey-server`, con Postgres y Meilisearch dentro. Sus líneas
aparecen en el mismo registro que las del servidor (prefijo `postgres:` para
la base de datos). El `docker-compose.yml` del repositorio añade los
contenedores `evey-postgres` y `evey-caddy`; consulta
[Despliegues avanzados](../operations/advanced-deployments.md).

**Puertos.** La API HTTP escucha en `EVEY_SERVER_PORT` + 1, `9091` por
defecto; `/healthz` responde ahí. La línea `ports:` del compose es la que lo
publica.

**Shell SQL** en la base de datos integrada:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**Registros.** El mismo flujo existe dos veces: el archivo diario
`./evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` dentro
de la imagen) y `docker logs evey-server`. El que manda es el archivo: el
driver json-file de Docker rota `docker logs`, y después de una rotación el
comando puede dejar de mostrar líneas horas antes de que se parara el
contenedor. Cuando los dos no coinciden, lee el archivo. La página
**Registros y depuración** del panel de administración busca en el mismo flujo
(`GET /api/v1/admin/logs/search?q=<term>&limit=200`); los bearers, los JWT y
los valores `token=` / `password=` aparecen ahí como `[redacted]`, así que
busca un id de medio, nunca un token.

## El servidor no responde

**Síntoma**: los clientes no consiguen conectarse;
`curl http://localhost:9091/healthz` agota el tiempo de espera o es
rechazado.

**Diagnóstico**:

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # ¿escucha el puerto?
curl -v http://localhost:9091/healthz
```

**Solución**:

- El contenedor se ha parado: el panic está al final de
  `docker logs evey-server`. Corrige la causa y después
  `docker compose up -d evey`.
- La comprobación de salud está en rojo porque la base de datos o el motor de
  búsqueda no funcionan: consulta
  [Errores de la base de datos](#database-errors) y
  [La búsqueda funciona peor](#search-is-degraded).
- Conflicto de puertos en el equipo: cambia el lado del equipo en la línea
  `ports:` del compose, o libera el puerto.
- Caddy, delante, no enruta (organización del repositorio):
  `docker logs evey-caddy`, comprueba `EVEY_DOMAIN`, y que la comprobación de
  salud de `evey` esté en verde, porque Caddy la espera.

## Todos los inicios de sesión devuelven 401

**Síntoma**: todos los usuarios, administradores incluidos, reciben 401 de
`/api/v1/auth/*`.

**Diagnóstico**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

Busca primero la línea de la época (epoch). Cada petición autenticada vuelve
a comprobar la época del token del usuario en la base de datos, y esa
comprobación falla en cerrado: una base de datos lenta, bloqueada o
inalcanzable hace que todas las peticiones devuelvan el mismo 401 que un
token caducado, lo que parece un problema de JWT y no lo es. El servidor lo
dice:

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

Si esa línea aparece, ve a [Errores de la base de datos](#database-errors).
Mientras tanto, `/healthz` puede seguir en verde: comprueba la conexión, no
ejecuta la consulta de la época. El contador de Prometheus es
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Solución**:

- **Una clave JWT que falta o que ha cambiado rara vez es la causa.** Un
  archivo de clave ilegible (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key`)
  impide arrancar al servidor; si falta, se genera otra. Los tokens de acceso
  firmados con la clave antigua se rechazan entonces, pero las apps los
  renuevan con su token de renovación, que se comprueba en la base de datos y
  no con la clave: los usuarios siguen con la sesión iniciada. Comprueba que
  el volumen `./evey-data` está montado en `/data`; sin él, también falta la
  base de datos.
- **El limitador de peticiones salta en cada petición**:
  `EVEY_RATELIMIT_LOGIN_MAX` (`10` por IP y por minuto) y
  `EVEY_RATELIMIT_GLOBAL_MAX` (`300`). Un proxy inverso que lo reenvía todo
  como una sola dirección lo hace saltar enseguida: añade el proxy a
  **Proxies de confianza** en **Administración -> Acceso remoto** (se aplica
  sin reiniciar; `EVEY_TRUSTED_PROXIES` es el valor de reserva). El
  `docker-compose.override.yml` del repositorio sube los dos a `1000` /
  `10000` para el desarrollo.
- **Desfase de reloj** entre el servidor y los clientes: `date -u` en el
  equipo.

## La transcodificación falla

**Síntoma**: la reproducción directa funciona, pero la reproducción que
necesita transcodificar falla; el reproductor da error y el registro del
servidor muestra códigos de salida de ffmpeg.

**Diagnóstico**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**Solución**:

- **Falta ffmpeg** (una imagen personalizada): vuelve a construirla a partir
  del `Dockerfile` oficial.
- **No se puede escribir en la carpeta de la caché**: la imagen define
  `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode` (el volumen `evey-cache`) y
  `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay`; una entrada `environment:` del
  compose los sustituye. Fuera de la imagen, los valores por defecto de Go
  son `/data/transcode` y `/cache/trickplay`.
- **Caché llena**: consulta [El disco de la caché está lleno](#the-cache-disk-is-full).
- **La aceleración por hardware se porta mal**: pon **Aceleración por
  hardware** en **Desactivada (solo software)** en **Administración ->
  Transcodificación -> Calibración de codificación** (se aplica en el acto y
  vuelve a calibrar) para que todas las transcodificaciones se hagan con el
  procesador; vuelve a **Auto (mejor disponible)** cuando el hardware esté
  arreglado.
- **«El servidor está ocupado transcodificando otros flujos»**: la capacidad
  en directo se mide, no se fija; consulta [Repartir la
  capacidad](../playback/transcoding.md#sharing-the-capacity).
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**Máximo simultáneo** en
  **Administración -> Caché de transcodificación**) solo cubre las tareas de
  transcodificación fuera de la reproducción en directo.

### Se queda cargando o se corta y el registro no dice por qué

**Síntoma**: un espectador avisa de parones en un título transcodificado; el
registro muestra `hls-vod: segment timeout` con `err:"context canceled"`, lo
que significa que el cliente ha colgado, no que el servidor haya agotado su
tiempo.

**Diagnóstico**: empieza por lo que el servidor ya ha registrado, no cuesta
nada:

```sh
# Por qué una petición de segmento ha reutilizado un codificador o ha lanzado
# uno, y lo que le ha costado al reproductor. Solo se registra cuando cuesta
# algo. `over_client_patience: true` es la prueba definitiva.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Los propios codificadores: lanzamientos, cierres y su motivo.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Esas líneas dicen lo que ha decidido el servidor. Para ver lo que ha
conseguido ffmpeg segundo a segundo, activa el registro de progreso de cada
codificador (`$TOKEN` es un JWT de administrador):

```sh
# ACTIVAR. Se aplica al siguiente codificador, sin reiniciar.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce el problema y lee una sesión: out_time = contenido producido hasta
# ahora, speed = cuánto se ha adelantado el codificador al tiempo real.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# DESACTIVAR al terminar.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note
    Deja el registro de progreso desactivado fuera de una investigación: un
    archivo por codificador, unos pocos MB por sesión, para cada
    reproducción en la máquina. El nombre del archivo lleva el hash de la
    decisión, el escalón (`-vN-`) y el segmento de inicio (`-sN-`), así que
    coincide con el registro JSON; un codificador que se vuelve a lanzar en
    la misma posición tiene su propio archivo, y eso es lo que hace visible
    que los codificadores se relanzan sin parar.

**Solución**:

- **Saltos cortos hacia delante repetidos**: cada uno puede lanzar un
  codificador nuevo, y el límite por hash (`MaxSessionsPerHash`, 2 por
  defecto, 1 con el perfil de poca memoria) echa al que lleva más tiempo sin
  usarse: `session killed reason:lru_cap` en el registro. Normal si no pasa
  demasiado.
- **El cliente tiene menos paciencia que el servidor**: la ventana para
  alcanzar un salto se calcula con `clientSegmentPatience` (6 s), mientras
  que el propio margen de espera del servidor (`WaitBudgetDur`) es de 60 s.
  Una línea `reach decision` con `over_client_patience: true` significa que
  el servidor estaba a punto de pedir una espera que ningún reproductor
  aguanta.

### Errores 410 `stale_rendition`: el reproductor vuelve a pedir su lista maestra en plena reproducción

**Síntoma**: el reproductor se para un momento y luego se recupera; el
registro muestra
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
y peticiones respondidas con `410 Gone` y `X-Evey-Error: stale_rendition`.

Es una protección que salta, no la avería en sí. Cada URI del manifiesto
lleva el hash de la decisión con la que se construyó (`fp=`). Un 410 significa
que un reproductor ha presentado una URL de una sesión cuya escala ABR ha
cambiado de forma desde entonces; servirla le daría al descodificador bytes
de otra calidad. El rechazo obliga a volver a pedir la lista maestra limpia.

**Diagnóstico**: el WARN lleva `sealed_fp`, `current_fp` y el id del medio.

- Una **ráfaga justo después de un despliegue, un reinicio o una
  recalibración** es normal: hay sesiones en curso que han sobrevivido a un
  cambio de capacidades. Los reproductores se ponen al día en una consulta.
- Un **goteo constante durante el funcionamiento normal** significa que algo
  sigue cambiando las escalas mientras hay flujos en curso: busca
  capacidades que cambian una y otra vez (`docker logs
  evey-server | grep -iE 'calibrat|capability'`), un regulador de admisión
  que cambia sin parar bajo carga, o un límite CMCD que oscila.
- Los reproductores que **se quedan en bucle con 410** tienen una lista
  maestra en caché (una versión antigua de la app, un proxy que guarda en
  caché los `.m3u8`). El proxy tiene que dejar pasar
  `Cache-Control: no-cache`.

## Errores de la base de datos

**Síntoma**: el registro contiene `dial tcp ... connect: connection refused`,
`pq: ...`, o las consultas agotan su tiempo.

**Diagnóstico**:

```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**Solución**:

- **Disco lleno** en `./evey-data` (el clúster está en
  `./evey-data/postgres`): libera espacio, y después
  `docker compose restart evey`.
- **Reserva de conexiones agotada** (`too many connections`, esperas
  largas): `EVEY_DB_MAX_OPEN_CONNS` (por defecto `25`). Cierra las sesiones
  bloqueadas:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Contenedor de Postgres aparte** (organización del repositorio):
  `docker ps --filter name=evey-postgres`,
  `docker logs --tail=200 evey-postgres`; los `EVEY_DB_USER` /
  `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` del servidor tienen que coincidir con
  el servicio `postgres`.

## La búsqueda funciona peor

**Síntoma**: la búsqueda da peores resultados; el registro muestra
`meilisearch unavailable, using SQL search fallback`.

La búsqueda sigue funcionando por SQL mientras no se puede llegar a
Meilisearch; solo baja la calidad del orden de los resultados, nadie se queda
sin servicio. Meilisearch funciona dentro de `evey-server`
(`EVEY_EMBEDDED_MEILI`, activado por defecto) en la interfaz local del
contenedor, puerto 7700.

**Diagnóstico**:

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**Solución**:

- No está sano: `docker compose restart evey`.
- Índice corrupto: para el contenedor, aparta `./evey-data/meili`, vuelve a
  arrancar, y después reconstruye el índice a partir de la base de datos:
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  La reconstrucción es un comando, no una acción del panel.

## El disco de la caché está lleno

**Síntoma**: las transcodificaciones fallan con `no space left on device`;
la comprobación de salud cambia de estado una y otra vez.

**Diagnóstico**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # el prefijo del compose puede cambiar
```

**Solución**:

- Vacía la caché de transcodificación, se puede regenerar:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Esto también borra `photos/` (las versiones redimensionadas de las fotos)
  y `track-art/` (las portadas extraídas de las etiquetas de audio) que hay
  dentro; las dos se reconstruyen en la siguiente petición. **Borrar todo**
  en **Administración -> Caché de transcodificación** vacía la misma caché
  desde el panel.
- Las miniaturas de la barra de avance (trickplay) cuestan más de regenerar;
  bórralas solo si hace falta:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Baja el **Presupuesto de caché de transcodificación (GB)** en
  **Administración -> Configuración del servidor -> Configuración del
  servidor** (por defecto `10`) si el volumen del equipo va justo. Sea cual
  sea el presupuesto, la limpieza automática mantiene 2 GiB libres en el
  volumen de la caché.

## Un plugin se ha bloqueado

**Síntoma**: un plugin deja de responder; el registro muestra panics del
ejecutor de plugins.

**Diagnóstico**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# busca: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Solución**:

1. Vuelve a lanzar el plugin sin tocar el servidor: **Activar** otra vez en
   **Administración -> Plugins**, o `POST /api/v1/admin/plugins/<id>/enable`.
   Funciona cuando el plugin está activado pero atascado en `Error`: el
   arranque solo rechaza un plugin que ya está `Running` o `Initializing`.
2. Si con eso no se arregla, **Desactivar** el plugin desde el panel.
3. Si el propio servidor está inestable: `docker compose restart evey`.
4. Los archivos de los plugins están en `./evey-data/plugins/` en el equipo
   (`/data/plugins` en el contenedor). Borra la carpeta del plugin roto y
   reinicia.
5. Envía la traza del panic y el id del plugin a su autor.

## Un envío se ha congelado, se ha quedado sin sonido, o el teléfono no deja de cargar

Todo lo de aquí se puede leer en **Administración -> Registros y depuración**; no
hace falta un shell en la máquina. Las horas están en UTC. Cada señal es una
línea `player-client: playback event reported` con `type=` y una cadena
`details=`.

1. **¿Estaba vivo el receptor, y hasta cuándo?** Busca `cast_receiver_alive`.
   El receptor envía una cada 30 s mientras reproduce; `details` lleva
   `buf=<behind>/<ahead>/<ranges>` (los segundos en búfer alrededor de la
   posición de lectura) y `pos_sec`. Una reproducción sana muestra `ahead`
   en decenas de segundos. La hora de la última señal es cuando el receptor
   murió o lo mataron: si no hay nada después, la tele ya no está ejecutando
   el receptor, diga lo que diga el teléfono.
2. **¿Por qué se paró?** Busca `cast_receiver_finished`:
   `reason=INTERRUPTED` es un LOAD nuevo que sustituye al que estaba en
   marcha (busca un `StartStream` unos segundos antes), `reason=ERROR` lleva
   el error del reproductor. Si no hay ningún `finished` después del último
   `alive`, el proceso del receptor ha desaparecido; casi siempre es un
   Chromecast HD que se ha cerrado por falta de memoria con una lista maestra
   de copia de más de 20 Mbps.
3. **¿Qué ha visto el teléfono?** Busca `cast_sender_stall`: el emisor envía
   `state=playing frozen=<n>s pos=<p>` cuando el SDK de Cast sigue diciendo
   PLAYING pero la posición no se ha movido en 20 s, y
   `state=idle reason=<r>` cuando el receptor ha indicado un motivo de
   inactividad. Son líneas WARN, así que el filtro de avisos de la página
   las muestra. Compara su `pos` con el último `pos_sec` del receptor.
4. **¿Ha habido una reacción en cadena en la sesión?** Busca
   `refresh rejected`: un token de renovación `not found` cierra la sesión de
   la app y la vuelve a abrir (con las credenciales guardadas), lo que vuelve
   a lanzar el flujo; la línea lleva el `jti`, `issued` y `age` del token,
   que permiten distinguir una copia antigua de una falsificada. Busca
   `StartStream` para ver los relanzamientos.
5. **¿Era el flujo demasiado pesado para el dispositivo?** Busca
   `advertises a fallback rung` (el bitrate máximo de la lista maestra de
   copia) y `plan fitted to the client's line` (la medida `maxKbps` del
   receptor o su rescate `abr=floor`; la lista maestra transcodificada que
   viene después lleva el motivo `client_line_exceeded` o
   `abr_floor_requested`). Una sesión que empezó con una lista maestra de
   copia de más de 20 Mbps y bajó a un escalón 1080p ajustado al cabo de
   unos segundos es el rescate haciendo su trabajo; un nuevo LOAD que vuelve
   a empezar con la lista maestra de copia es lo que mata a un Chromecast
   HD.
6. **¿Le ha faltado algo al servidor?** Busca
   `on-demand resource refused`: una ráfaga de rechazos `context canceled`
   en segmentos seguidos es alguien moviéndose por la barra de avance (cada
   petición la cancela la siguiente), no un fallo del servidor; las líneas
   `served` con duraciones largas sí lo son.

Si los pasos 1 a 3 muestran el receptor vivo y el emisor callado, el problema
está en la sesión de Cast del teléfono (el SDK, la red hasta la tele). Si las
señales del receptor se paran primero, mira la tele: la memoria, el códec,
el bitrate del flujo.

## Detener una sesión o un dispositivo comprometido

**Síntoma**: un dispositivo inesperado en **Administración -> Reproduciendo
ahora**, o un usuario avisa de un dispositivo desconocido en su lista de
sesiones.

1. Identifica la sesión:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Detenla desde **Administración -> Reproduciendo ahora**: el icono de
   parar de la sesión (**Detener sesión**), y después **Detener** para
   confirmar; o `AdminSessionService/KillSession` por gRPC con el id de la
   sesión. El reproductor recibe primero una orden de parar y el flujo se
   termina 5 segundos después. Esto solo termina la reproducción; el usuario
   sigue con la sesión iniciada.
3. Si el flujo sigue funcionando de todos modos, reinicia el contenedor:
   todos los codificadores se paran con él.
   ```sh
   docker compose restart evey
   ```
4. Si la propia cuenta está comprometida, **Administración -> Usuarios ->
   Desactivar**: los tokens de renovación se borran y los tokens de acceso se
   revocan en el acto, así que se cierra la sesión en todos los
   dispositivos. Pon una contraseña nueva (`evey admin reset-password
   --user <email>`, consulta [Recuperación](recovery.md#give-a-user-a-new-password)),
   y después vuelve a **Activar** la cuenta.

## Un escaneo de la biblioteca se ha atascado

**Síntoma**: un escaneo aparece en curso durante horas sin ningún avance de
archivos en el registro.

1. Comprueba si el escaneo sigue escribiendo algo en el registro; un escaneo
   colgado muestra el procesador sin actividad y ninguna línea `scan:`
   reciente:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. El bloqueo del escaneo está en memoria, así que un reinicio lo quita:
   ```sh
   docker compose restart evey
   ```
3. Si un archivo provoca panics una y otra vez (las trazas lo nombran),
   sácalo de la carpeta de la biblioteca y vuelve a escanear.
4. Si el vigilante de archivos registra `watcher: inotify failed for path,
   will use periodic scan`, se ha agotado el límite de vigilancias del
   núcleo:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Chuleta

```sh
# Estado de todo
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Seguir todos los contenedores
docker compose logs --tail=100 -f

# Configuración efectiva
docker exec evey-server env | grep ^EVEY_ | sort

# Tamaño de la base de datos
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Disco del equipo
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/en/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/en/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/en/common.yaml - enable, disable
-->
