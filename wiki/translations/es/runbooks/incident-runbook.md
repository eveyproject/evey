---
source_sha: baf759b03f8e14d6048f850d6927ea2371fa5533ad8148f6f86faf5364ba5f66
---

# Guía de incidencias

Manual de primera respuesta para las caídas más habituales de Evey. Cada sección sigue el mismo esquema: **síntoma → diagnóstico → arreglo**.

Contenedores: `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (producción), `evey-web` y `evey-mailcatcher` (override de desarrollo). Una instalación por defecto tiene **solo `evey-server`**: Postgres y Meilisearch corren dentro de él, así que sustituya el `docker exec evey-postgres psql -U ...` que aparece más abajo por `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`, y sus registros son las líneas `postgres:` y las de Meili dentro de `docker logs evey-server`.

Puerto HTTP por defecto: `9091` (`EVEY_HTTP_PORT`). Extremo de salud: `/healthz`.

**Registros.** Hay dos copias del mismo flujo: el archivo diario `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` dentro de la imagen) y `docker logs evey-server`. El archivo es el que manda: el controlador json-file rota lo que ve `docker logs`, y tras una rotación la orden puede detenerse horas antes que el contenedor; el `stop_detail` de una prueba de carga o el veredicto de una calibración parecen entonces no estar, cuando lo único que ha pasado es que se han ido con la rotación. Cuando ambos discrepen, lea el archivo o, como root, los archivos en bruto `/var/lib/docker/containers/<id>/*-json.log*`. Las órdenes de una línea con `docker logs` que siguen dan por hecho una ventana reciente y sin rotar.

## 1. No se llega al servidor

**Síntoma**: los clientes no consiguen conectarse; `curl http://localhost:9091/healthz` agota el tiempo o devuelve conexión rechazada.

**Diagnóstico**:
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # ¿está el puerto escuchando de verdad?
curl -v http://localhost:9091/healthz
```

**Causas frecuentes y arreglo**:
- El contenedor ha salido → busque el pánico en `docker logs evey-server`, corríjalo y haga `docker compose up -d evey`.
- El healthcheck falla porque la base de datos o Meili están caídos → véanse las secciones 4 y 5.
- Choque de puertos en el host → cambie `EVEY_HTTP_PORT` o libere el puerto.
- El Caddy de delante no encamina → `docker logs evey-caddy`, compruebe `EVEY_DOMAIN` y que el healthcheck de `evey` esté en verde (Caddy depende de él).

## 2. Todos los inicios de sesión devuelven 401

**Síntoma**: todo el mundo, incluidos los administradores que entraron ayer, recibe un 401 de `/api/v1/auth/*`.

**Diagnóstico**:
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Busque primero la línea de la epoch.** Cada petición autenticada vuelve a
comprobar la epoch del token del usuario contra la base de datos (ADR-0019), y
esa comprobación falla en CERRADO. Así que una base de datos lenta, bloqueada o
inalcanzable hace que todas las peticiones del servidor devuelvan el mismo 401
que devuelve un token caducado, lo que se lee exactamente como un problema de
JWT sin serlo. El servidor lo dice con todas las letras:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Si esa línea está, deje de leer esta sección y vaya a la §1 (base de datos).
`/healthz` puede seguir en verde aquí: solo hace un ping a la conexión, no
ejecuta la consulta de la epoch. El contador de Prometheus es
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Causas frecuentes y arreglo**:
- **Falta la clave JWT o ha cambiado**: `EVEY_JWT_PRIVATE_KEY_PATH` (por defecto `/data/jwt.key`) no se puede leer, o se ha rotado. Todos los tokens firmados con la clave antigua son ya inválidos y no hay más remedio que reponer la clave anterior desde una copia, o hacer que todo el mundo vuelva a iniciar sesión. Confirme que el volumen `evey-data` está montado correctamente en `/data`.
- **El límite de frecuencia salta en cada petición**: mire `EVEY_RATELIMIT_LOGIN_MAX` (por defecto `10` por IP y minuto) y `EVEY_RATELIMIT_GLOBAL_MAX` (por defecto `300`). Un proxy inverso que reenvíe todo el tráfico como una sola IP lo dispara enseguida: defina `EVEY_TRUSTED_PROXIES` y reinicie. El override de desarrollo ya pone `1000` y `10000`.
- Desfase de reloj entre el servidor y los clientes → `date -u` en el host.

## 3. La transcodificación falla o los vídeos no se reproducen transcodificados

**Síntoma**: la reproducción directa funciona y falla en cuanto hace falta transcodificar. El reproductor da error y en los registros del servidor aparecen códigos de salida de ffmpeg.

**Diagnóstico**:
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Causas frecuentes y arreglo**:
- **Falta el binario de ffmpeg** (una imagen propia que no lo trae): recompile a partir del `Dockerfile` oficial.
- **El directorio de caché no se puede escribir**: `EVEY_TRANSCODING_CACHE_DIR` vale `/data/transcode` por defecto en el código, pero el compose lo asigna a `/cache/transcode` con el volumen nombrado `evey-cache`. `EVEY_TRICKPLAY_CACHE_DIR` vale `/cache/trickplay` por defecto. Compruebe que se puede escribir:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Caché llena** → véase la sección 6.
- **Aceleración por hardware mal configurada**: ponga `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false` para forzar el software, reinicie y confirme que la regresión viene del hardware antes de volver a activarla.
- **Demasiadas transcodificaciones a la vez**: `EVEY_TRANSCODING_MAX_CONCURRENT` (por defecto `2`). Súbalo con prudencia si la máquina tiene núcleos.

### 3c. «Se queda almacenando en búfer / se ha cortado» y los registros no dicen por qué

**Síntoma**: un espectador informa de paradas o cortes en un título transcodificado, y los registros del servidor muestran `hls-vod: segment timeout` con `err:"context canceled"`, lo que significa que quien ha colgado es el CLIENTE, no que al servidor se le haya agotado el tiempo.

**Diagnóstico**: el servidor ya anota todas las decisiones que ha tomado. Empiece por ahí, que es gratis:

```sh
# Por qué una petición de segmento reutilizó un codificador o lanzó uno nuevo, y
# lo que le costó al reproductor. Solo se registra cuando cuesta algo: una
# espera de más de un segmento, o un lanzamiento. `over_client_patience: true`
# es la prueba del delito.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Los codificadores en sí: lanzamientos, muertes y su motivo.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Esas líneas dicen lo que el servidor *decidió*. Cuando necesite lo que ffmpeg
*consiguió* de verdad segundo a segundo, es decir la posición real de la
frontera a lo largo del tiempo, active el registro de progreso por codificador:

```sh
# ACTIVAR. Se aplica al SIGUIENTE codificador; no hace falta reiniciar.
# $TOKEN es un JWT de administrador.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduzca el problema y lea después una sesión: out_time = contenido
# producido hasta ahora, speed = cuánto se adelantó el codificador al tiempo real.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# DESACTIVAR cuando haya terminado.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Déjelo apagado fuera de una investigación.** Es un archivo por codificador,
unos pocos MB por sesión, escrito en cada reproducción de la máquina: disco de
verdad y escrituras de verdad en un NAS flojo. El nombre del archivo lleva el
hash de la decisión, el peldaño (`-vN-`) y el segmento inicial (`-sN-`), así que
casa con el registro JSON de más arriba; un relanzamiento en la misma posición
crea su propio archivo en lugar de sobrescribir el anterior, y eso es lo que
hace visible el ir y venir de codificadores.

**Causas frecuentes y arreglo**:
- **Posicionamientos cortos hacia delante, uno tras otro**: cada uno puede
  lanzar un codificador nuevo, y el tope por hash (`MaxSessionsPerHash`, 2 por
  defecto) desaloja al menos usado recientemente. Cuente con ver
  `session killed reason:lru_cap` en el registro. Con moderación, es normal.
- **El cliente tiene menos paciencia que el servidor**: `clientSegmentPatience`
  (6 s) es la medida con la que se dimensiona la ventana de alcance del
  posicionamiento; AndroidX Media3 abandona un segmento a los 8 s escasos,
  mientras que el `WaitBudgetDur` del servidor es de 60 s. Una línea
  `reach decision` con `over_client_patience: true` significa que el servidor
  estaba a punto de pedir una espera que ningún reproductor aguantaría.

### 3b. Los 410 de `stale_rendition` / el reproductor vuelve a pedir su maestro a mitad de reproducción

**Síntoma**: el reproductor se para un momento y se recupera; los registros
muestran
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
y peticiones respondidas con `410 Gone` y `X-Evey-Error: stale_rendition`.

**Qué significa**: esto es *una protección disparándose*, no el fallo en sí.
Cada URI del manifiesto lleva el hash de la decisión con la que se construyó
(`fp=`). Un 410 quiere decir que un reproductor ha presentado una URL de una
sesión cuya escalera ABR se ha remodelado desde entonces; servirla le daría al
descodificador bytes de otra versión (la familia de incidencias de la «pantalla
arcoíris», ADR 0022 §1b). El rechazo le obliga a pedir de nuevo el maestro, y
así queda limpio.

**Diagnóstico**: el WARN lleva `sealed_fp` y `current_fp`, más el id del medio.
- Una **ráfaga justo después de un despliegue, un reinicio o una recalibración
  desde el panel** es lo esperable: había sesiones vivas que sobrevivieron a un
  cambio de capacidad. No hay nada que hacer; los reproductores se resincronizan
  en un sondeo.
- Un **goteo constante durante el funcionamiento normal** significa que algo
  sigue remodelando escaleras por debajo de flujos vivos: busque una capacidad
  que oscila (`docker logs evey-server | grep -iE 'calibrat|capability'`), un
  modelador de admisión revuelto por la carga o una anulación de tope de CMCD
  que va y viene.
- Los reproductores que **entran en bucle de 410 sin recuperarse** tienen un
  maestro en caché (una compilación antigua de la aplicación, un proxy que
  cachea `.m3u8` con demasiado celo). Compruebe que el proxy no cachea los
  manifiestos (`Cache-Control: no-cache` tiene que pasar de largo).

## 4. Conexión a la base de datos rechazada o con errores

**Síntoma**: en los registros del servidor aparecen `dial tcp ... connect: connection refused`, `pq: ...`, o las consultas agotan el tiempo.

**Diagnóstico**:
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**Causas frecuentes y arreglo**:
- El contenedor de Postgres está caído → `docker compose up -d postgres`, y vigile en los registros si hay corrupción o un OOM.
- Credenciales incorrectas → el entorno del servidor tiene que coincidir con el `EVEY_DB_USER`, el `EVEY_DB_PASSWORD` y el `EVEY_DB_NAME` que usa el servicio `postgres`. Por defecto: `evey` / `evey` / `evey`.
- Pool agotado (`too many connections` o esperas largas) → mire `EVEY_DB_MAX_OPEN_CONNS` (por defecto `50`). Mate las sesiones atascadas:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- Disco lleno en `postgres-data/` → `df -h`, libere espacio y reinicie.

## 5. Meilisearch caído

**Síntoma**: la búsqueda devuelve resultados degradados y los registros muestran `meilisearch unavailable, using SQL search fallback`.

**Comportamiento**: Evey tiene un repliegue resistente y la búsqueda sigue funcionando por SQL (`cmd/evey/wire.go` conecta un adaptador de búsqueda resistente que registra `meilisearch unavailable, using SQL search fallback` cuando no se llega a Meili). El usuario no tiene que hacer nada para seguir en marcha; lo único que empeora es la calidad de la ordenación.

**Nota sobre el modo de despliegue**: el `docker-compose.yml` por defecto ejecuta Meilisearch **integrado dentro del contenedor `evey-server`** (`EVEY_EMBEDDED_MEILI: "true"`). Un contenedor `evey-meilisearch` aparte solo existe en el override de desarrollo (`docker-compose.override.yml`). En modo integrado, «Meilisearch caído» significa que el proceso evey-server no está sano: no hay ningún contenedor aparte que reiniciar.

**Diagnóstico y arreglo (integrado, el caso por defecto)**:
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # puerto reenviado desde dentro de evey-server
docker logs --tail=200 evey-server | grep -i meili
```
- El servidor no está sano → `docker compose restart evey-server`.
- Índice corrupto → pare evey-server, haga copia de `./evey-data/meili-data/`, bórrelo, reinicie y lance una reindexación completa desde el panel de administración.

**Diagnóstico y arreglo (solo con el override de desarrollo, contenedor aparte)**:
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- El contenedor está caído → `docker compose up -d meilisearch`.
- `MEILI_MASTER_KEY` no coincide → los registros del servidor muestran fallos de autenticación. Iguale el entorno en los dos servicios y reinicie.
- Índice corrupto → pare Meili, haga copia de `./meili-data/`, bórrelo, reinicie y lance una reindexación completa desde el panel de administración.

## 6. Disco de la caché de transcodificación lleno

**Síntoma**: las transcodificaciones fallan con `no space left on device`; el healthcheck del contenedor oscila.

**Diagnóstico**:
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # el nombre que retuerce compose puede variar
```

**Arreglo**:
- Purga sin riesgo, la caché se regenera:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Eso se lleva también `photos/` (las variantes redimensionadas de las fotos) y
  `track-art/` (la carátula incrustada en las etiquetas de una pista, extraída
  una vez y redimensionada), que cuelgan de ahí; las dos se rehacen en la
  siguiente petición. El botón «Limpiar caché» del panel de administración hace
  esa misma purga.
- El trickplay cuesta más de regenerar; púrguelo solo si no hay más remedio:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- Baje `EVEY_TRANSCODING_CACHE_MAX_GB` (por defecto `50`) si el volumen del host es más pequeño que eso.

## 7. Una extensión se ha caído

**Síntoma**: una extensión deja de responder; los registros del servidor muestran pánicos del ejecutor de extensiones.

**Diagnóstico**:
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Busque: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Arreglo**:
- Pruebe primero un reinicio quirúrgico, sin parar el contenedor: `POST /api/v1/admin/plugins/<id>/enable`. Funciona incluso cuando la extensión ya está `enabled` pero atascada en estado `Error`: la guarda de `StartPlugin` la relanza en cuanto el estado no es `Running`. Prefiéralo a reiniciar el servidor entero, sobre todo si no tiene acceso SSH a la máquina a mano.
- Si el reinicio quirúrgico no lo resuelve, desactive desde el panel de administración la extensión culpable.
- Si el inestable es el propio servidor, reinicie: `docker compose restart evey`.
- Inspeccione los archivos de la extensión en `./evey-data/plugins/` (host) == `/data/plugins` (contenedor). Quite el zip o el directorio roto y reinicie.
- Mande la traza del pánico y el id de la extensión a quien la desarrolla.

## 8. Una emisión se ha congelado, se ha quedado muda o el móvil no para de girar

Todo lo que sigue se puede leer desde la página de Registros del panel de
administración (o con
`GET /api/v1/admin/logs/search?q=<term>&limit=200`): no hace falta ningún
intérprete de órdenes en la máquina. Las horas son UTC. Cada baliza es una línea
`player-client: playback event reported` con un `type=` y una cadena `details=`.

1. **¿Estaba vivo el receptor, y hasta cuándo?** Busque `cast_receiver_alive`.
   El receptor manda una cada 30 s mientras reproduce; `details` lleva
   `buf=<behind>/<ahead>/<ranges>` (segundos almacenados por detrás y por
   delante del cabezal de reproducción) y `pos_sec`. Una reproducción sana
   muestra decenas de segundos en `ahead`. **La hora de la última baliza es el
   momento en que el receptor murió o lo mataron**: si no hay nada después, el
   televisor ya no está ejecutando el receptor, diga lo que diga el móvil.
2. **¿Por qué se paró?** Busque `cast_receiver_finished`: `reason=INTERRUPTED`
   es un LOAD nuevo que sustituye al que estaba en marcha (busque un
   `StartStream` unos segundos antes); `reason=ERROR` lleva el error del
   reproductor. Ningún `finished` después del último `alive` = el proceso del
   receptor desapareció (casi siempre, un Chromecast HD al que se mató por
   memoria con un maestro de copia de más de 20 Mbps).
3. **¿Qué vio el móvil?** Busque `cast_sender_stall`: el emisor manda
   `state=playing frozen=<n>s pos=<p>` cuando el SDK de Cast sigue diciendo
   PLAYING pero la posición lleva 20 s sin moverse, y `state=idle reason=<r>`
   cuando el receptor ha informado de un motivo de inactividad. Esas líneas se
   registran como WARN, así que el filtro de avisos de la página de Registros
   las saca a la luz. Es la versión del emisor de ese mismo minuto: compare su
   `pos` con el último `pos_sec` del receptor.
4. **¿Se encadenó la sesión?** Busque `refresh rejected`: un token de refresco
   «not found» cierra la sesión de la aplicación y la vuelve a abrir (con las
   credenciales guardadas), lo que reemite el flujo; la línea dice ahora el
   `jti` del token, su `issued` y su `age`, que es lo que distingue una copia
   caducada de una falsificada. Busque `StartStream` para ver las reemisiones;
   el emisor ya no vuelve a cargar el receptor con una URL reemitida para el
   mismo título.
5. **¿Era el flujo demasiado grande para el aparato?** Busque
   `advertises a fallback rung` (la tasa de bits de pico del maestro de copia) y
   `capped master served`: una sesión que empezó con un maestro de copia de más
   de 20 Mbps y a los pocos segundos cayó a un peldaño de 1080p limitado es el
   rescate de suelo haciendo su trabajo; un nuevo LOAD que arranca otra vez con
   el maestro de copia es justo el patrón que mata a un Chromecast HD.
6. **¿Hay inanición en el servidor?** Busque `on-demand resource refused`: una
   ráfaga de rechazos con `context canceled` en segmentos consecutivos es un
   barrido por la barra de posición (cada petición cancelada por la siguiente),
   no un fallo del servidor; las que sí lo son son las líneas `served` con
   duraciones largas.

Si los pasos 1 a 3 muestran el receptor vivo y el emisor callado, el problema
está en la sesión de Cast del móvil (el SDK, la red hasta el televisor); si las
que se cortan antes son las balizas del receptor, hay que mirar en el televisor
(memoria, códec, tasa de bits del flujo).

## Cortar una sesión o un aparato comprometidos

Síntoma: el administrador ve un aparato inesperado en **Reproduciendo ahora** (panel de administración), o un usuario informa de un aparato desconocido en su lista de sesiones.

1. Identifique el `session_id`:
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. Córtela desde la interfaz de administración: **Admin → Actividad → Reproduciendo ahora → ⋯ → Detener sesión**, o por gRPC con `AdminSessionService/KillSession` y el `session_id`. La sesión termina 5 s después, para que al reproductor le dé tiempo a recibir la orden de parada.
3. Si la sesión no responde, borre a la fuerza la sesión HLS (el directorio es `<cacheDir>/<mediaItemID>`, por defecto `/cache/transcode/<mediaItemID>`):
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. Si además hay que revocar el token de refresco (por sospecha de contraseña comprometida):
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   Todos los aparatos de ese usuario tendrán que volver a iniciar sesión. Para un cierre completo, restablezca la contraseña con `evey admin reset-password --user <username>`.

## Desatascar un análisis de biblioteca colgado

Síntoma: un análisis de biblioteca lleva horas «en curso» pero en los registros no avanza ningún archivo.

1. Consulte de forma indirecta el mapa `scan_service.scanning`: si el proceso está colgado, la CPU está ociosa y en los registros recientes no aparece ningún `scan_service`:
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. El bloqueo del análisis vive **en memoria** (no hay ninguna fila en la base de datos), así que un reinicio del servidor lo suelta:
   ```sh
   docker compose restart evey-server
   ```
3. Si hay un archivo concreto que provoca pánicos una y otra vez (mire las trazas de pila), póngalo en cuarentena sacándolo de la ruta de la biblioteca y vuelva a analizar.
4. Si se han agotado las vigilancias de inotify (el límite del núcleo), súbalo:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Chuleta general de diagnóstico

```sh
# Estado de todo
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Seguir los registros de todos los contenedores
docker compose logs --tail=100 -f

# Volcado de la configuración del servidor
docker exec evey-server env | grep ^EVEY_ | sort

# Tamaño y bloqueos de Postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Disco del host
df -h . ./postgres-data ./meili-data ./evey-data
```
