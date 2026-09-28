---
source_sha: a7cdb2195f99559246f23706d11f64ad40c0cb3dcd88a52f3caf61af02c7ce8c
---
# Configuración

Dos lugares, una regla. **Lo que tiene que existir antes de que arranque el
servidor** - secretos, cableado del contenedor, datos del proxy inverso - vive
en el archivo compose. **Lo que un operador ajusta después** - nivel de log,
idioma, límites de transcodificación, aceleración por hardware, vigilancia de
archivos, entorno aislado y cuotas de las extensiones - vive en **Admin >
Configuración del servidor**, se guarda en la base de datos, sobrevive a una
recreación del contenedor y casi siempre se aplica sin reiniciar.

Fijar uno de esos ajustes en el entorno solo siembra el primer arranque. Por eso
esta página no lista todas las `EVEY_*` que acepta el binario: las demás son
semillas del primer arranque y salidas de emergencia. `internal/config/config.go`
es la referencia exhaustiva si la necesita.

## 1. Lo que debe fijar usted

Nada, en la instalación por defecto: la ruta de los medios y la etiqueta de la
imagen están en el propio archivo compose, y no hay `.env`. El
`docker-compose.yml` del repositorio, en cambio, lee `MEDIA_DIR` de `.env`:

```env
MEDIA_DIR=/path/to/your/media
```

En ambos casos, Postgres y Meilisearch se ejecutan integrados en el contenedor y
generan sus propios secretos en el primer arranque, guardados en
`./evey-data/keys` (`postgres.password`, `meili.key`, 0600). El servidor los lee
mediante `EVEY_DB_PASSWORD_FILE` / `MEILI_MASTER_KEY_FILE` (fijados en la
imagen), por eso `docker exec evey-server evey admin ...` tampoco necesita
contraseña en la línea de órdenes. Fije `EVEY_DB_PASSWORD` / `MEILI_MASTER_KEY`
explícitamente solo cuando traiga su propia base de datos o su propio motor de
búsqueda - un valor explícito siempre gana al archivo.

Los metadatos funcionan desde el principio: hay una clave de TMDB integrada. Una
clave propia (gratuita en themoviedb.org) le da su propia cuota; introdúzcala en
**Admin > Configuración del servidor > Claves API**, o siémbrela con
`EVEY_TMDB_API_KEY`. Una vez publicado el servidor, `EVEY_SERVER_BASE_URL` es la
única variable que hay que añadir - sección 2.

## 2. Una instancia publicada, en dos variables

Detrás de un proxy inverso y con transcodificación por hardware, todo el entorno
son dos variables. Todo lo demás que se suele poner en un archivo compose o ya
está en la imagen o es un valor por defecto:

```yaml
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # your reverse proxy, /32 if you know its address
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # hardware transcoding
    group_add:
      - "991"                                 # the group owning /dev/dri/renderD128 on YOUR host
```

Líneas que **no** conviene copiar de configuraciones antiguas:

| Línea | Por qué sobra |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | La imagen ya las fija en `/data/logs`, `/cache/transcode`, `/data/trickplay`. Repetirlas no cambia nada. |
| `EVEY_FORWARDED_PROTO` | Obsoleta - se deduce del esquema de `EVEY_SERVER_BASE_URL`. Fijarla solo registra un aviso de obsolescencia. |
| `EVEY_KEYFRAME_BACKFILL=on` | Cualquier valor distinto de `off` lo activa, incluida su ausencia. |
| `LIBVA_DRIVER_NAME` | libva elige el controlador a partir del id PCI de la GPU. Fuércelo solo si su tarjeta necesita un controlador que libva no elige (compruébelo con `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` elige el confinamiento más fuerte que admite su host. Fíjelo explícitamente solo si quiere una garantía de *fallar cerrado* - `landlock` se niega a arrancar una extensión que no puede confinar, donde `auto` se degrada con un aviso. |
| un `group_add` para `/dev/dri/card0` | Evey solo abre `renderD128`. Compruébelo con `ls -ln /dev/dri` y añada solo ese grupo. |

## 3. Los dos ajustes que el panel de administración no sustituye

Los dos tienen un parecido en **Admin → Acceso remoto**, y ninguno hace
innecesaria la variable de entorno. Esto confunde a mucha gente, así que conviene
decirlo explícitamente.

La diferencia está en dónde vive cada uno. El bloque de acceso remoto
(activar/desactivar, URL externa, rangos LAN, techo de bitrate remoto) se
**guarda en la base de datos** - sus variables de entorno siembran el primer
arranque y después se ignoran. `EVEY_SERVER_BASE_URL` y `EVEY_TRUSTED_PROXIES`
son lo contrario: se leen del entorno en cada arranque y **nada en el panel las
sobrescribe nunca**. Así que un valor que en el panel parece un duplicado es, en
estos dos casos, otro ajuste que casualmente menciona una dirección.

**`EVEY_SERVER_BASE_URL` no lo sustituye «URL externa».** El valor del panel
alimenta las URL que *construye* el servidor. Pero si el servidor confía en
`X-Forwarded-Proto` - y por tanto si cree que se sirve por HTTPS - se decide **al
arrancar, solo a partir de la variable de entorno**. Si falta la variable, recae
en `http://localhost:9091`: las cookies `Secure`, las redirecciones y todo enlace
absoluto se degradan a `http://`, y las URL de compartir/descargar apuntan a
localhost. El valor del panel se lee demasiado tarde para ayudar.

**`EVEY_TRUSTED_PROXIES` no lo sustituye «Redes LAN».** Son dos etapas, no dos
nombres para lo mismo:

1. `EVEY_TRUSTED_PROXIES` decide **a quién se le cree el `X-Forwarded-For`**, y
   por tanto qué dirección recibe un cliente.
2. Las redes LAN clasifican luego **esa** dirección como local o remota.

La segunda opera sobre el resultado de la primera. Sin la primera, todos los
clientes llegan con la dirección del proxy y todos se clasifican igual - que es
exactamente como el control de acceso remoto pasó cuatro meses sin actuar (vea
`internal/config/remote_access_resolve.go`). El loopback siempre es de confianza y
no se puede quitar; su entrada se *añade* a él.

## 4. Propietario de los archivos (PUID/PGID)

**Por defecto: automático.** Si `PUID`/`PGID` no están fijados, el punto de
entrada mira al arrancar el propietario de sus montajes `/media*` y ejecuta el
servidor como ese usuario, para que las funciones que *escriben en* su
biblioteca (la reparación del índice de búsqueda reconstruye los archivos en su
sitio) funcionen desde el principio - sin `chown`, sin configuración. El log de
arranque dice qué se eligió:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Los montajes de medios que pertenecen a **usuarios distintos** se tratan en la
medida de lo posible: el servidor adopta el usuario propietario de más montajes
y se une a los grupos de los demás montajes como grupos suplementarios, así que
las reparaciones en esos montajes funcionan cuando sus archivos son escribibles
por el grupo (el aviso de arranque lista qué montajes podrían seguir rechazando
escrituras). La detección automática solo se aparta del todo (recae en
`1000:1000` con un aviso) cuando todos los montajes de medios pertenecen a root o
al uid reservado `1001` - pase sus medios a un usuario normal con chown, o fije
`PUID`/`PGID` explícitamente:

```env
PUID=1026   # run the server as this user id  (host: `id -u <media-owner>`)
PGID=100    # ...and this group id
```

Los valores explícitos siempre ganan a la detección.

Cómo funciona: el contenedor arranca como root solo el tiempo justo para
reasignar su usuario interno (y volver a apropiarse de los directorios de estado
propios de Evey `/data`, `/cache` - **nunca** de sus montajes de medios), y luego
suelta los privilegios. Los grupos suplementarios concedidos con `group_add:` en
el compose (p. ej. el grupo render de su host para la transcodificación por
hardware con `/dev/dri`) se conservan tras soltarlos. El proceso del servidor en
sí nunca se ejecuta como root.

Alternativa sin root en absoluto: si incluso ese root pasajero le resulta
inaceptable, fije en su lugar `user: "UID:GID"` en el servicio de su archivo
compose. El punto de entrada detecta el arranque sin root y se salta la
reasignación (incluida la detección del propietario de los medios) - entonces le
toca a usted mantener `./evey-data` escribible para ese UID.

Notas:
- `PUID=1001` se rechaza: es el usuario interno `evey-plugin`, que se mantiene
  aparte para que las extensiones nunca compartan la identidad del servidor.
- Los medios se montan en lectura-escritura por defecto para que las
  reparaciones funcionen sin más; añada `:ro` al volumen de medios si prefiere una
  biblioteca de solo lectura (las reparaciones fallarán entonces con un error
  claro).

## 5. Referencia de variables

### Servidor
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_DEMO=false                        # public demo instance only (demo.eveyproject.org): locks admin writes, no mail/push/webhooks
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### Base de datos
```env
# Embedded (default when EVEY_DB_HOST is unset): Postgres 17 inside the container.
EVEY_EMBEDDED_POSTGRES=            # true | false; unset = embedded iff EVEY_DB_HOST is unset
EVEY_POSTGRES_DATA_DIR=/data/postgres   # cluster location inside the container (under ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # unset = generated on first boot into /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # read when EVEY_DB_PASSWORD is unset (image default)

# External: use these INSTEAD of the block above, not alongside it.
# Setting the host turns the embedded database off.
EVEY_DB_HOST=postgres              # the repository compose's sibling service is `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # or disable | require | verify-full
```

El Postgres integrado solo escucha en el loopback del contenedor y pide una
contraseña por TCP; la generada es la que usan el servidor (y `evey admin`). Su
directorio de datos tiene que estar en un sistema de archivos de verdad (ext4,
btrfs, xfs, ...), no en una carpeta de red. Las migraciones se ejecutan solas al
arrancar en las dos disposiciones.

### Autenticación
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### Bibliotecas
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

La sonda de escaneo de la biblioteca lee los metadatos de `.mkv`/`.webm` y de
`.mp4`/`.m4v` en proceso (mkvgo, solo la cabecera - ~35 veces más rápido al
reescanear/reparar/en un NAS) sin ninguna configuración: siempre está activa.
ffprobe sigue siendo la autoridad y se ocupa de todo archivo que la ruta rápida
no puede trasladar campo por campo (HDR/Dolby Vision, códecs no reconocidos, …).
Vea [adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md).

### Transcodificación
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

La configuración del hardware está en [transcoding.md](transcoding.md).

**Audio Dolby en un Chromecast** (`EVEY_CAST_WASM_AUDIO_ENABLED`, por defecto
`false`). Evey puede entregar a un receptor de cast la pista AC-3/E-AC-3 original
y dejar que la descodifique en WebAssembly en la tele, evitando una
transcodificación. Está **desactivado para los receptores de cast** porque un
aparato modesto de un operador murió a mitad de película, dos veces, con esa vía
en juego - y su descodificador se midió allí a 3,13 veces el tiempo real, así que
no ganaba nada. Desactivado, un cast recibe el master multiplexado normal con una
pista AAC: sonido desde el primer segundo, en todos los aparatos. Los navegadores
NO se ven afectados - conservan el descodificador wasm, validado en ellos. Ponga
`true` para volver a activarlo en los cast.

**Prefiera el panel de administración.** La concurrencia, la aceleración por
hardware, AV1, el presupuesto de la caché y los límites avanzados (máximo de
sesiones HLS simultáneas, techos de altura por software y software-HDR, codificar
un peldaño cada vez, un codificador de hardware por peldaño) están todos en
**Admin > Transcodificación**. Los valores guardados allí sobreviven a los
reinicios y tienen prioridad sobre el entorno; el panel indica cuáles solo surten
efecto tras reiniciar.

**Los indicadores de los workers en segundo plano** se fijan como filas en
**Admin > Configuración del servidor** (el panel sugiere las claves y muestra para
cada una la variable de entorno equivalente y el valor esperado). Todos se leen
al arrancar, así que se aplican al reiniciar:

| Clave | Valor |
| --- | --- |
| `transcoding.keyframe_backfill` | `off` para desactivar el relleno del índice de fotogramas clave (activo por defecto) |
| `transcoding.keyframe_backfill_batch` | elementos por pasada, > 0 |
| `transcoding.keyframe_backfill_pause_ms` | milisegundos entre elementos, >= 0 |
| `transcoding.keyframe_backfill_idle_sec` | segundos entre pasadas en reposo, > 0 |
| `transcoding.level_backfill` | `off` para desactivar el relleno de los niveles H.264/HEVC (activo por defecto) |
| `transcoding.level_backfill_batch` | elementos por pasada, > 0 |
| `transcoding.level_backfill_pause_ms` | milisegundos entre elementos, >= 0 |
| `transcoding.level_backfill_idle_sec` | segundos entre pasadas en reposo, > 0 |
| `transcoding.pause_bg_on_playback` | `off` para que el trabajo de fondo siga durante la reproducción (activo por defecto: se aparta) |
| `transcoding.snapshot_fastpath` | `off` para forzar el extractor ffmpeg exacto en lugar de la ruta rápida de fotogramas clave de mkvgo |

Cada uno tiene también un equivalente en el entorno (`EVEY_KEYFRAME_BACKFILL`,
`EVEY_LEVEL_BACKFILL_BATCH`, ...), pero la fila del panel gana.

**Presupuesto de CPU en segundo plano** (Admin > Configuración del servidor >
Potencia asignada) limita el procesador que pueden tomar juntas todas las tareas
de fondo, en caliente (sin reiniciar):

| Clave | Valor |
| --- | --- |
| `background.cpu_percent` | parte de la capacidad de CPU de la que tiran todas las tareas de fondo: `25` (por defecto, suave) o `50` (impulso) |
| `background.cpu_percent.<task>` | la parte propia de una tarea pesada, de `1` a `80`, que sustituye a la global para esa tarea: `library_scan`, `trickplay_generation`, `intro_detection`, `complexity_classifier` |

Las partes nunca se suman: el total de lo que corre en segundo plano se queda por
debajo de la parte más alta fijada. Una parte se convierte en núcleos enteros
(25 % de 8 núcleos = 2), uno como mínimo. Cada ffmpeg de fondo reserva sus hilos
en ese presupuesto antes de arrancar; el escaneo de bibliotecas y la detección de
intros dimensionan su paralelismo con él, y la generación de miniaturas trickplay
sus hilos de descodificación. La reproducción sigue pausando todo análisis de
fondo (`transcoding.pause_bg_on_playback`). `EVEY_LIBRARY_SCAN_WORKERS` y
`EVEY_INTRO_DETECTION_MAX_PARALLEL` dimensionan esas dos reservas a mano y se
saltan el presupuesto para ellas. Las antiguas claves `trickplay.adaptive_cpu`
(`false` = 50 %) e `intro_detection.use_all_cores` (`true` = 80 % para la
detección de intros) se siguen respetando hasta que se fijen las nuevas.

**Presupuesto de la caché de transcodificación** (Admin > Configuración del
servidor, `transcoding.cache_max_gb`, `10` por defecto, se aplica al reiniciar)
limita toda la caché de transcodificación en el disco: segmentos de reproducción,
remux de arranque rápido y descargas compatibles juntos, bajo un solo número. Es
un tamaño absoluto a propósito: la caché vive en el volumen `evey-cache` junto a
lo que el operador tenga allí, y el uso del resto del volumen es cosa suya. Un
único barredor lo aplica, empezando por lo menos usado recientemente, y solo
respeta lo que un espectador está leyendo. Sea cual sea el presupuesto, también
deja 2 GiB libres en el volumen de la caché y retira una descarga que nadie ha
recogido en 7 días. Una sesión de copia (remux) nunca guarda la película entera:
mantiene una ventana de segmentos alrededor del espectador y vuelve a buscar en
la fuente para todo lo que quede fuera. La vía mkvgo sin almacenamiento, que sirve
la mayoría de las fuentes MKV y MP4, no toca la caché en absoluto.

### Extensiones (entorno aislado y límites)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - run plugins as a separate user to wall off secrets (auto = on only when the effective sandbox resolves to "basic", i.e. neither Landlock nor bubblewrap); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0-19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

Guía por escenario: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Modelo completo: [SECURITY.md](SECURITY.md).

### Correo (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

Vea [notifications.md](notifications.md).

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

Vea [oidc.md](oidc.md).

### Copias de seguridad
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

Vea [backups.md](backups.md).

### Proxy inverso
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # added to the always-trusted loopback, not a replacement
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

El comportamiento HTTPS (cookies Secure, HSTS, confianza en `X-Forwarded-Proto`)
sigue el esquema de `EVEY_SERVER_BASE_URL`: una URL base `https://` lo activa.
`EVEY_FORWARDED_PROTO=true|false` está obsoleta y solo se conserva como
sobrescritura.

Vea [reverse-proxy.md](reverse-proxy.md).

### Detección de intros
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # every player auto-skips detected intros (web/mobile read the flag in the segments response; cast receivers get intro segments gated by it)
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
`intro_providers.skip_preview_enabled` e `intro_providers.auto_skip_enabled`
también se pueden configurar en `/admin/server-config` (primero la base de datos,
el entorno como respaldo). Reinicie el servidor tras cambiar
`intro_providers.enabled`; los interruptores de saltar el avance y de saltar
intros automáticamente se aplican en caliente (sin reiniciar).

Vea [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md).

### Registro (logs)
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # the image's default; the compose file maps it to ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

El archivo diario `evey-YYYY-MM-DD.log` bajo `EVEY_LOG_DIR` es el log de
referencia. `docker logs evey-server` muestra el mismo flujo, pero el controlador
json-file de Docker lo rota, y tras una rotación la orden puede parecer detenida
horas antes que el contenedor. Cuando no coinciden, fíese del archivo - o, como
root, de los archivos en bruto `/var/lib/docker/containers/<id>/*-json.log*`.
Fuera de la imagen (una instalación sin contenedor) el valor por defecto es un
directorio `logs` relativo al directorio de trabajo, así que apunte `EVEY_LOG_DIR`
a una ruta absoluta escribible.

### Acceso remoto (quién puede conectarse desde fuera de su red)
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # unset = auto (see below); true/false to decide yourself
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # the public address, if it differs from EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # CIDRs treated as LAN
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = no cap for remote (WAN) sessions
```

> ⚠️ El control protege el punto de escucha **HTTP** (`EVEY_SERVER_PORT` + 1). El
> punto de escucha gRPC nativo en `EVEY_SERVER_PORT` es un transporte interno y no
> está controlado, así que se enlaza **solo a loopback** (`EVEY_GRPC_BIND`, por
> defecto `127.0.0.1`): su único cliente es la pasarela HTTP del mismo proceso.
> Ponga `EVEY_GRPC_BIND=0.0.0.0` para exponerlo deliberadamente - el arranque
> avisará entonces de que el control de acceso remoto no lo cubre, y protegerlo con
> un cortafuegos pasa a ser tarea suya.

**El control.** Con el acceso remoto desactivado, todo cliente cuya IP no esté en
`EVEY_REMOTE_ACCESS_LAN_NETWORKS` se rechaza en todas las rutas con
`403 {"code":"remote_access_disabled"}`. Es una decisión de despliegue («esta
instancia es para mi propia red»), no una capa de seguridad - lo que protege su
biblioteca es la autenticación.

**Automático por defecto.** Deje `EVEY_REMOTE_ACCESS_ENABLED` sin fijar y el
servidor decide al arrancar: si `EVEY_REMOTE_ACCESS_EXTERNAL_URL` o
`EVEY_SERVER_BASE_URL` es una dirección accesible públicamente, el acceso remoto
queda **activado** (un servidor que usted publicó está para que se llegue a él);
si no, se queda en la LAN. La decisión se registra al arrancar, y
`evey admin doctor` la informa.

Fije la variable - o use **Admin → Acceso remoto** - para decidir explícitamente;
la deducción nunca sobrescribe una elección explícita. Si mantiene a propósito un
servidor publicado solo en la LAN, el log de arranque lleva un aviso que lo dice,
porque el síntoma que le llegaría si no es «en tu móvil funciona, en el mío no».

> Detrás de un proxy inverso esto solo funciona si el proxy transmite la IP real
> del cliente y `EVEY_TRUSTED_PROXIES` cubre el proxy. Si no, todos los clientes
> parecen la propia dirección del proxy - vea [reverse-proxy.md](reverse-proxy.md).

Cuando `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` está fijado, las sesiones de
reproducción cuya IP de cliente se clasifica como WAN se limitan a ese bitrate; el
límite solo BAJA el techo del cliente/perfil. Dos reglas de seguridad que conviene
conocer:

- Una IP de cliente privada según RFC1918 **nunca** se limita, aunque haya
  olvidado listar su rango en `EVEY_REMOTE_ACCESS_LAN_NETWORKS` - el límite solo
  cae sobre una dirección claramente pública.
- Si usa rangos locales poco habituales (CGNAT `100.64.0.0/10`, una subred de
  VPN/Tailscale) y NO quiere limitar a esos espectadores, lístelos explícitamente en
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS`. Detrás de un proxy inverso, asegúrese de que
  transmite la IP real del cliente (`X-Forwarded-For`), o todas las sesiones
  parecerán venir de la dirección del proxy.
