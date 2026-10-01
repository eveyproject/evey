---
source_sha: 183e65df2f01ed4e879613548dd30f3675ca6c82fa34bb3521c12c68938b83be
reviewed: false
---
# Manual de incidentes

Guía de primera respuesta para las caídas más comunes de Evey. Cada sección: **síntoma → diagnóstico → solución**.

Los comandos apuntan a la instalación estándar: un contenedor, `evey-server`, con Postgres y Meilisearch ejecutándose dentro; sus registros son las líneas `postgres:` y de Meilisearch de `docker logs evey-server`. La disposición del `docker-compose.yml` del repositorio añade los contenedores `evey-postgres` y `evey-caddy` ([Despliegues avanzados](../operations/advanced-deployments.md)).

Para ejecutar SQL contra la base de datos integrada:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

Puerto HTTP por defecto: `9091` (`EVEY_HTTP_PORT`). Endpoint de salud: `/healthz`.

**Registros.** Dos copias del mismo flujo: el archivo diario `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` dentro de la imagen) y `docker logs evey-server`. El archivo es la referencia: el controlador json-file rota `docker logs`, y tras una rotación el comando puede terminar horas antes que el contenedor; el `stop_detail` de una prueba de carga o el veredicto de una calibración parecen entonces ausentes cuando solo se han ido con la rotación. Cuando no coinciden, lee el archivo o, como root, los archivos brutos `/var/lib/docker/containers/<id>/*-json.log*`. Los comandos `docker logs` de abajo suponen una ventana reciente, sin rotar.

## 1. El servidor no responde

**Síntoma**: los clientes no consiguen conectar; `curl http://localhost:9091/healthz` agota el tiempo o devuelve connection refused.

**Diagnóstico**:

```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # ¿escucha realmente el puerto?
curl -v http://localhost:9091/healthz
```

**Causas habituales + solución**:
- El contenedor terminó → `docker logs evey-server` para ver el panic, corregir, `docker compose up -d evey`.
- El healthcheck falla porque la base de datos o Meili están caídos → consulta las secciones 4 y 5.
- Conflicto de puerto en el equipo → cambia `EVEY_HTTP_PORT` o libera el puerto.
- El Caddy de delante no enruta → `docker logs evey-caddy`, comprueba `EVEY_DOMAIN` y que el healthcheck de `evey` está en verde (Caddy depende de él).

## 2. Todos los inicios de sesión devuelven 401

**Síntoma**: todos los usuarios, incluidos los administradores que iniciaron sesión ayer, reciben 401 de `/api/v1/auth/*`.

**Diagnóstico**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Busca primero la línea del epoch.** Cada petición autenticada vuelve a
comprobar el epoch del token del usuario contra la base de datos (ADR-0019), y
esa comprobación falla en CERRADO. Así, una base de datos lenta, bloqueada o
inaccesible hace que cada petición del servidor devuelva el mismo 401 que un
token caducado, lo que parece exactamente un problema de JWT y no lo es. El
servidor lo dice de forma explícita:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Si esa línea aparece, deja de leer esta sección y ve al §4 (base de datos).
`/healthz` puede seguir en verde aquí: solo hace ping a la conexión, no ejecuta
la consulta del epoch. El contador de Prometheus es
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Causas habituales + solución**:
- **Clave JWT ausente o cambiada** rara vez es la causa. Un `EVEY_JWT_PRIVATE_KEY_PATH` (por defecto `/data/jwt.key`) ilegible o dañado impide que el servidor arranque; uno ausente se regenera. Los tokens de acceso firmados con la clave antigua se rechazan entonces, pero las aplicaciones los renuevan con su token de actualización, que se comprueba en la base de datos y no con la clave: los usuarios siguen con la sesión iniciada. Confirma que el volumen `evey-data` está montado en `/data`: sin él también falta la base de datos.
- **El límite de peticiones salta en cada petición**: comprueba `EVEY_RATELIMIT_LOGIN_MAX` (por defecto `10` por IP y minuto) y `EVEY_RATELIMIT_GLOBAL_MAX` (por defecto `300`). Un proxy inverso que reenvía todo el tráfico como una sola IP lo hará saltar enseguida: añade el proxy a **Proxies de confianza** en Admin → Acceso remoto (se aplica sin reiniciar; `EVEY_TRUSTED_PROXIES` también sirve). El override de desarrollo ya fija `1000` / `10000`.
- Desfase de reloj entre servidor y clientes → `date -u` en el equipo.

## 3. La transcodificación falla / los vídeos no se reproducen transcodificados

**Síntoma**: la reproducción directa funciona, pero falla cuando hace falta transcodificar. Errores en el reproductor, y los registros del servidor muestran códigos de salida de ffmpeg.

**Diagnóstico**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Causas habituales + solución**:
- **Falta el binario de ffmpeg** (imagen personalizada sin él): reconstruye desde el `Dockerfile` oficial.
- **Directorio de caché sin permiso de escritura**: `EVEY_TRANSCODING_CACHE_DIR` vale `/data/transcode` por defecto en el código, pero el compose lo apunta a `/cache/transcode` mediante el volumen con nombre `evey-cache`. `EVEY_TRICKPLAY_CACHE_DIR` es `/data/trickplay` en la imagen (`/cache/trickplay` es el valor por defecto fuera de ella). Comprueba que se puede escribir:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Caché llena** → consulta la sección 6.
- **La aceleración por hardware falla**: pon **Aceleración por hardware** en **Desactivada (solo software)** en **Admin → Transcodificación → Calibración de codificación** (se aplica en caliente y vuelve a medir) para mantener toda transcodificación en el procesador, y confirma que el problema viene del hardware antes de volver a **Auto**.
- **"El servidor está ocupado transcodificando otros flujos"**: la capacidad en directo se mide, no se fija; consulta [Transcodificación y capacidad del servidor](../playback/transcoding.md#sharing-the-capacity). `EVEY_TRANSCODING_MAX_CONCURRENT` (**Máximo concurrente** en **Admin → Caché de transcodificación**) solo cubre las tareas de transcodificación fuera de la reproducción en directo.

### 3c. "Se queda cargando / se cortó" y los registros no dicen por qué

**Síntoma**: un espectador informa de parones o cortes en un título
transcodificado, y los registros del servidor muestran `hls-vod: segment timeout`
con `err:"context canceled"`, lo que significa que colgó el CLIENTE, no que el
servidor agotara el tiempo.

**Diagnóstico**: el servidor ya registra cada decisión que tomó. Empieza por ahí,
porque es gratis:

```sh
# Por qué una petición de segmento reutilizó un codificador o lanzó uno, y qué le
# costó al reproductor. Solo se registra cuando cuesta algo: una espera de más de
# un segmento, o un lanzamiento. `over_client_patience: true` es la prueba.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Los propios codificadores: lanzamientos, terminaciones y su motivo.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Esas líneas dicen lo que el servidor *decidió*. Cuando necesitas lo que ffmpeg
*consiguió* realmente segundo a segundo, la posición real de la frontera a lo
largo del tiempo, activa el registro de progreso por codificador:

```sh
# ACTIVAR. Se aplica al SIGUIENTE codificador; sin reiniciar. $TOKEN es un JWT de administrador.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce el problema y lee una sesión: out_time = contenido producido hasta
# ahora, speed = cuánto se adelantó el codificador al tiempo real.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# DESACTIVAR de nuevo al terminar.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Déjalo desactivado fuera de una investigación.** Es un archivo por
codificador, unos pocos MB por sesión, escrito para cada reproducción de la
máquina: disco real y escrituras reales en un NAS modesto. El nombre del archivo
lleva el hash de la decisión, el escalón (`-vN-`) y el segmento de inicio
(`-sN-`), así que casa con el registro JSON de arriba; un relanzamiento en la
misma posición tiene su propio archivo en lugar de sobrescribir la ejecución
anterior, y eso es lo que hace visible la rotación de codificadores.

**Causas habituales + solución**:
- **Saltos cortos hacia delante repetidos**: cada uno puede lanzar un codificador
  nuevo, y el límite por hash (`MaxSessionsPerHash`, 2 por defecto) expulsa el
  usado hace más tiempo. Verás `session killed reason:lru_cap` en el registro.
  Normal con moderación.
- **El cliente tiene menos paciencia que el servidor**: `clientSegmentPatience`
  (6 s) es el valor con el que se dimensiona la ventana de alcance de un salto;
  AndroidX Media3 abandona un segmento a los ~8 s mientras que el
  `WaitBudgetDur` del servidor es de 60 s. Una línea `reach decision` con
  `over_client_patience: true` significa que el servidor iba a pedir una espera
  que ningún reproductor aguantaría.

### 3b. 410 `stale_rendition` / el reproductor vuelve a pedir su master a mitad de reproducción

**Síntoma**: el reproductor se para un momento y luego se recupera; los registros
muestran `hls-vod: stale rendition URL refused (the ladder moved under a live player)`
y peticiones respondidas con `410 Gone` y `X-Evey-Error: stale_rendition`.

**Qué significa**: es una *protección que salta*, no el fallo en sí. Cada URI de
manifiesto lleva el hash de la decisión con la que se construyó (`fp=`). Un 410
significa que un reproductor presentó una URL de una sesión cuya escala ABR se ha
remodelado desde entonces: servirla le daría al descodificador bytes de otra
rendición (la clase de incidente de la "pantalla arcoíris", ADR 0022 §1b). El
rechazo fuerza en su lugar una nueva petición limpia del master.

**Diagnóstico**: el WARN lleva `sealed_fp` frente a `current_fp` y el id del medio.
- Una **ráfaga justo después de un despliegue o reinicio, o de una recalibración
  del administrador** es lo esperado: las sesiones en directo sobrevivieron a un
  cambio de capacidades. Nada que hacer; los reproductores se resincronizan en un
  sondeo.
- Un **goteo constante durante el funcionamiento normal** significa que algo
  sigue remodelando escalas bajo flujos en directo: busca capacidades que
  oscilan (`docker logs evey-server | grep -iE 'calibrat|capability'`), un
  regulador de admisión que se agita bajo carga, o un override de límite CMCD
  que oscila.
- Los reproductores que **entran en bucle con el 410 sin recuperarse** tienen un
  master en caché (versión antigua de la aplicación, proxy que guarda en caché
  los `.m3u8` de forma agresiva). Comprueba que el proxy no guarda manifiestos en
  caché (`Cache-Control: no-cache` debe pasar).

## 4. Conexión a la base de datos rechazada / errores

**Síntoma**: los registros del servidor contienen `dial tcp ... connect: connection refused`, `pq: ...`, o las consultas agotan el tiempo.

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

**Causas habituales + solución**:
- **Disco lleno** bajo `./evey-data` (la base de datos vive en `./evey-data/postgres`) → libera espacio y después `docker compose restart evey`.
- **Pool agotado** (`too many connections` o esperas largas) → `EVEY_DB_MAX_OPEN_CONNS` (por defecto `25`). Termina las sesiones atascadas:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Contenedor de Postgres aparte** (disposición del repositorio): `docker ps --filter name=evey-postgres`, `docker logs --tail=200 evey-postgres`; `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` del servidor deben coincidir con el servicio `postgres`.

## 5. Meilisearch caído

**Síntoma**: la búsqueda devuelve resultados degradados, los registros muestran `meilisearch unavailable, using SQL search fallback`.

**Comportamiento**: la búsqueda sigue funcionando por SQL (`cmd/evey/wire.go` conecta un adaptador de búsqueda resiliente que registra esa línea cuando Meilisearch no es accesible). No hace falta ninguna acción del usuario para seguir en línea; solo empeora la calidad de la clasificación.

Meilisearch se ejecuta dentro de `evey-server` (`EVEY_EMBEDDED_MEILI`, activado por defecto) y escucha en `127.0.0.1:7700` solo dentro del contenedor.

**Diagnóstico + solución**:
```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```
- No está sano → `docker compose restart evey`.
- Índice corrupto → detén el contenedor, aparta `./evey-data/meili`, arranca de nuevo y reconstruye el índice a partir de la base de datos: `docker exec evey-server evey admin reindex-search`.

## 6. Disco de la caché de transcodificación lleno

**Síntoma**: las transcodificaciones fallan con `no space left on device`; el healthcheck del contenedor oscila.

**Diagnóstico**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # el nombre generado por compose puede variar
```

**Solución**:
- Purga segura, la caché se puede regenerar:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Esto también elimina `photos/` (variantes redimensionadas de fotos) y
  `track-art/` (la portada incrustada en las etiquetas de una pista, extraída una
  vez y redimensionada) bajo ella: ambas se reconstruyen en la siguiente
  petición. **Limpiar todo** en Admin → Caché de transcodificación hace la misma
  purga.
- El trickplay es más caro de regenerar; purga solo si es necesario:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Baja el **Presupuesto de caché de transcodificación** (**Admin → Configuración del servidor → Configuración del servidor**, `10` GB por defecto) si el volumen del equipo va justo; sea cual sea el presupuesto, el limpiador mantiene 2 GiB libres en el volumen de la caché.

## 7. Un plugin se ha caído

**Síntoma**: un plugin deja de responder; los registros del servidor muestran panics del ejecutor de plugins.

**Diagnóstico**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Busca: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Solución**:
- Prueba primero un reinicio quirúrgico, sin parar el contenedor: `POST /api/v1/admin/plugins/<id>/enable`. Funciona incluso cuando el plugin ya está `enabled` pero atascado en estado `Error`: la guarda de `StartPlugin` lo relanza en cuanto el estado no es `Running`. Prefiérelo a reiniciar todo el servidor, sobre todo cuando no tienes acceso SSH al equipo a mano.
- Si el reinicio quirúrgico no lo arregla, desactiva el plugin problemático desde el panel de administración.
- Si el propio servidor está inestable, reinicia: `docker compose restart evey`.
- Inspecciona los archivos de plugins en `./evey-data/plugins/` (equipo) == `/data/plugins` (contenedor). Elimina el zip o directorio del plugin roto y reinicia.
- Envía la traza del panic + el id del plugin a su autor.

## 8. Un envío se congeló, se quedó sin sonido o el teléfono no para de cargar

Todo lo de abajo se puede leer desde la página de registros de administración (o
`GET /api/v1/admin/logs/search?q=<term>&limit=200`): no hace falta un shell en la
máquina. Las horas están en UTC. Cada baliza es una línea `player-client: playback event reported` con `type=` y una cadena `details=`. Las credenciales nunca aparecen
en lo que devuelven estas rutas (los bearer, los JWT y los valores `token=`/`password=` se leen
`[redacted]`), así que el token de una URL de envío no es algo que una búsqueda en
los registros pueda recuperar: busca el id del medio, no el token.

1. **¿Estaba vivo el receptor, y hasta cuándo?** Busca `cast_receiver_alive`.
   El receptor envía una cada 30 s mientras reproduce; `details` lleva
   `buf=<behind>/<ahead>/<ranges>` (segundos en búfer por detrás y por delante del
   cabezal de reproducción) y `pos_sec`. Una reproducción sana muestra `ahead` en
   decenas de segundos. **La hora de la última baliza es cuando el receptor murió
   o lo mataron**: nada después significa que el televisor ya no ejecuta el
   receptor, diga lo que diga el teléfono.
2. **¿Por qué se paró?** Busca `cast_receiver_finished`: `reason=INTERRUPTED`
   es un nuevo LOAD que sustituye al que estaba en marcha (busca un `StartStream`
   unos segundos antes), `reason=ERROR` lleva el error del reproductor. Ningún
   `finished` tras el último `alive` = el proceso del receptor desapareció (lo más
   frecuente, un Chromecast HD matado por memoria con un master de copia de más
   de 20 Mbps).
3. **¿Qué vio el teléfono?** Busca `cast_sender_stall`: el emisor envía
   `state=playing frozen=<n>s pos=<p>` cuando el SDK de Cast todavía dice PLAYING
   pero la posición no se ha movido en 20 s, y `state=idle reason=<r>` cuando el
   receptor indicó un motivo de inactividad. Estas líneas se registran en WARN,
   así que el filtro de avisos de la página de registros las muestra. Es el lado
   del emisor en el mismo minuto: compara su `pos` con el último `pos_sec` del
   receptor.
4. **¿Hubo una cascada en la sesión?** Busca `refresh rejected`: un token de
   actualización "not found" cierra la sesión de la aplicación y la vuelve a
   abrir (credenciales guardadas), lo que vuelve a emitir el flujo; la línea
   indica ahora el `jti`, `issued` y `age` del token, lo que distingue una copia
   obsoleta de una falsificada. Busca `StartStream` para ver las reemisiones; el
   emisor ya no recarga el receptor con una URL reemitida para el mismo título.
5. **¿Era el flujo demasiado grande para el dispositivo?** Busca `advertises a fallback rung` (el bitrate de pico del master de copia) y `plan fitted to the client's line` (la medida `maxKbps` del receptor o su rescate `abr=floor`, ahora una
   entrada del plan: el master de transcodificación que sigue lleva el motivo
   `client_line_exceeded` o `abr_floor_requested`): una sesión que empezó con un
   master de copia de más de 20 Mbps y bajó a un escalón 1080p ajustado al cabo
   de unos segundos es el rescate del escalón mínimo haciendo su trabajo; un
   nuevo LOAD que vuelve a empezar con el master de copia es el patrón que mata a
   un Chromecast HD.
6. **¿Falta de recursos en el servidor?** Busca `on-demand resource refused`: una
   ráfaga de rechazos `context canceled` en segmentos consecutivos es un
   desplazamiento rápido por la línea de tiempo (cada petición cancelada por la
   siguiente), no un fallo del servidor; las líneas `served` con duraciones largas
   sí lo son.

Si los pasos 1-3 muestran el receptor vivo y el emisor callado, el problema está
en la sesión Cast del teléfono (el SDK, la red hasta el televisor); si las
balizas del receptor se paran primero, hay que mirar el televisor (memoria,
códec, bitrate del flujo).

## Terminar una sesión o un dispositivo comprometido

Síntoma: el administrador ve un dispositivo inesperado en **Reproduciendo ahora** (panel de administración), o un usuario informa de un dispositivo desconocido en su lista de sesiones.

1. Identifica el `session_id`:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Termínala desde la interfaz de administración: **Admin → Reproduciendo ahora**, el icono de parada de la sesión (**Detener sesión**) y después **Detener** para confirmar; o mediante gRPC `AdminSessionService/KillSession` con el `session_id`. La sesión se termina 5 s después para que el reproductor pueda recibir la orden de parada.
3. Si el flujo sigue en marcha de todos modos, reinicia el contenedor: todos los codificadores se paran con él.
   ```sh
   docker compose restart evey
   ```
4. Si la propia cuenta está comprometida, **Admin → Usuarios → Desactivar** sobre ella: eso borra sus tokens de actualización e invalida sus tokens de acceso de inmediato, así que se cierra la sesión en todos los dispositivos. Define una contraseña nueva (`evey admin reset-password --user <email>`, que también termina las sesiones de la cuenta) y después vuelve a **Activar** la cuenta.

## Desbloquear un escaneo de biblioteca atascado

Síntoma: un escaneo de biblioteca aparece "en curso" durante horas pero los registros no muestran progreso en los archivos.

1. Comprueba si el escaneo sigue registrando algo: si el proceso está colgado, la CPU está inactiva y los registros recientes no muestran ninguna línea `scan:`:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. El bloqueo de un escaneo está **en memoria** (ninguna fila en la base de datos), así que reiniciar el servidor lo quita:
   ```sh
   docker compose restart evey
   ```
3. Si un archivo concreto provoca panics repetidos (mira las trazas), ponlo en cuarentena sacándolo de la ruta de la biblioteca y vuelve a escanear.
4. Si se agotan los watches de inotify (límite del kernel), súbelo:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Chuleta general de diagnóstico

```sh
# Estado de todo
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Seguir todos los contenedores
docker compose logs --tail=100 -f

# Volcado de la configuración del servidor
docker exec evey-server env | grep ^EVEY_ | sort

# Tamaño de la base de datos
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Disco del equipo
df -h ./evey-data
```
