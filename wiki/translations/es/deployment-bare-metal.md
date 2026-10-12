---
source_sha: 4aeebbe2feb50999e8470ea3f56b3e47af196ca12cf0100e22527f66ffe278e3
reviewed: true
---
# Sin contenedor

Cómo ejecutar Evey directamente en un equipo (systemd, un script de inicio o
a mano) en lugar de usar la [imagen Docker](installation.md). La imagen es el
camino recomendado: trae el sandbox de plugins, el endurecimiento y los
entornos de ejecución de los plugins ya preparados. Sin contenedor, tienes que
reproducir tú mismo ese nivel de protección: los binarios correctos uno junto
a otro, un usuario dedicado que no sea root, el mecanismo de sandbox que
admite tu kernel y carpetas de secretos accesibles solo para su propietario.
Esta página lo recorre en orden.

## Compilar e instalar los binarios

Compila los dos binarios Go e instálalos en la misma carpeta: el ejecutor de
plugins busca el lanzador primero en `$EVEY_PLUGIN_LAUNCHER`, después junto a
su propio ejecutable y por último en el `$PATH`.

```bash
# desde la raíz del repositorio
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# opcional: autocompletado del shell y página de manual, los dos generados por el binario
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` muestra la versión, el commit y la fecha de compilación sin
necesitar ninguna configuración.

### La capa seccomp

Compila `libevey-sandbox.so` (un solo archivo C, necesita un compilador C) e
instálalo en la ruta que espera el ejecutor, `/app/libevey-sandbox.so`:

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

La ruta está fijada en el ejecutor. Sin ese archivo, los plugins se niegan a
arrancar (cierre por defecto), salvo si el servidor funciona en modo
desarrollo. Esta capa añade una lista de llamadas al sistema prohibidas
(seccomp) para los plugins enlazados dinámicamente, además de Landlock o
bubblewrap. La imagen la guarda en `/app` con el SDK; un enlace simbólico
también sirve.

### Entornos de ejecución y servicios

Instala lo que invoque el `entrypoint:` de tus plugins. El ejecutor reconoce
`python3`, `python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash` y `sh`;
`python3` cubre la mayoría de los plugins de la comunidad. Después, los
servicios con los que habla el servidor:

| Dependencia | Para qué |
|---|---|
| **ffmpeg** y **ffprobe** | transcodificación y análisis de archivos; la imagen también incluye los controladores VAAPI y `fpcalc` (chromaprint) para detectar las intros |
| **PostgreSQL** (servidor más una base de datos) y **`pg_dump`** | la base de datos; `pg_dump` escribe el volcado automático previo a las migraciones |
| **Meilisearch** | el índice de búsqueda; sin él, la búsqueda recurre a SQL |
| **`bwrap`** (bubblewrap) | solo para el mecanismo de sandbox bubblewrap |
| **`setcap`** (libcap) | para dar al lanzador su capacidad de archivo |

La base de datos y el motor de búsqueda integrados son una función del punto
de entrada de la imagen, no del binario: sin contenedor, los ejecutas tú
mismo y apuntas `EVEY_DB_HOST` y `EVEY_MEILI_URL` hacia ellos.

## Ejecutar con un usuario dedicado que no sea root

Nunca ejecutes el servidor como root. Crea un grupo y dos usuarios sin
privilegios, como hace la imagen:

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) ejecuta el servidor.
- `evey-plugin` (UID 1001) es el usuario al que pasan los plugins cuando la
  separación de uid está activa. Así, un plugin comprometido acaba como otro
  usuario sin privilegios, que no puede leer los secretos del servidor por
  simples permisos de archivo.

!!! warning
    El id del grupo importa: el lanzador hace pasar un plugin al UID 1001 y al
    grupo del propio servidor, y rechaza cualquier uid o gid por debajo de
    1000. Un grupo creado solo con `useradd --system` recibe un id de sistema
    por debajo de 1000, y entonces los plugins en modo `basic` se rechazan.

El servidor se marca como no volcable al arrancar (`PR_SET_DUMPABLE=0`), así
que un plugin que se ejecuta en el mismo equipo no puede leer
`/proc/<server-pid>/{mem,environ}`.

## Elegir el mecanismo de sandbox según tu kernel

Define `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (el valor por defecto) prueba en
este orden landlock, bubblewrap y basic, y elige el más fuerte que admite el
equipo. Fija un modo explícito si quieres que, en caso de fallo, el plugin no
arranque.

### Landlock (recomendado)

Una lista de rutas permitidas en el sistema de archivos (un plugin ve su
propia carpeta y la carpeta de sockets, nunca tus datos ni tus secretos), más
un filtro de salida TCP de Landlock v4. No necesita privilegios, ni montajes,
ni espacios de nombres: el lanzador se lo aplica a sí mismo y después ejecuta
el plugin.

Landlock tiene que estar compilado en el kernel y en la lista de LSM activos.
El ejecutor prueba `landlock_create_ruleset`; la prueba falla cuando la
llamada al sistema no existe (kernel anterior a 5.13) o cuando Landlock no
está en la lista activa. Para comprobarlo:

```bash
uname -r                              # se necesita 5.13 o posterior
cat /sys/kernel/security/lsm          # debe incluir "landlock"
```

Si falta `landlock` en esa lista, añádelo a la línea de comandos del kernel y
reinicia (conserva las entradas existentes, la lista cambia según la
distribución):

```
# por ejemplo en GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Después, deja `auto` o fíjalo:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # cierre por defecto: un plugin se rechaza si Landlock no se puede aplicar
```

`landlock` pasa `-require-landlock` al lanzador, que termina con un código
distinto de cero en lugar de ejecutar un plugin sin confinar.

### bubblewrap

Si Landlock no está disponible, instala `bwrap`. Pone cada plugin en
espacios de nombres de usuario, pid, uts, ipc y red, con una lista de
montajes permitidos. Necesita espacios de nombres de usuario sin privilegios
y la llamada al sistema `mount`: es así en la mayoría de los Linux sin
contenedor, pero está bloqueado dentro de un contenedor sin privilegios, y por
eso Landlock es el valor por defecto en contenedor. `auto` solo elige
bubblewrap cuando falta Landlock y una prueba desechable de `bwrap` funciona.

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espacios de nombres
EVEY_PLUGINS_SANDBOX_MODE=strict      # espacios de nombres más la capa seccomp
```

Comprueba que el equipo permite crear los espacios de nombres y los montajes:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (último recurso)

`EVEY_PLUGINS_SANDBOX_MODE=basic` es un cambio de uid más la capa seccomp; el
lanzador, si está presente, sigue instalando su lista seccomp del kernel y
Landlock en la medida de lo posible. No hay confinamiento del sistema de
archivos por rutas: un plugin se ejecuta con el UID 1001 y solo con seccomp.
Úsalo en kernels antiguos o muy restringidos donde no funciona ni Landlock ni
bubblewrap, y cuenta con la separación de uid descrita más abajo para
proteger los secretos.

Un plugin que tiene el permiso `subprocess` necesita el lanzador incluso en
modo `basic`: la capa seccomp no sobrevive a un `exec` de un proceso hijo,
solo el seccomp del kernel que instala el lanzador. Sin el lanzador, un plugin
así se niega a arrancar en lugar de ejecutarse sin confinar.

## Mantener los secretos fuera del alcance de un plugin

La separación de uid ejecuta cada plugin como `evey-plugin` (UID 1001),
distinto del servidor (UID 1000), así que un plugin no puede leer
`jwt.key`, las copias de seguridad ni los registros por simples permisos de
archivo. Es importante en modo `basic`; con Landlock o bubblewrap, el sistema
de archivos ya está cerrado, así que ahí es redundante.

`EVEY_PLUGINS_UID_SEPARATION`:

| Valor | Comportamiento |
|---|---|
| `auto` (por defecto) | activa solo cuando el sandbox queda en `basic` |
| `on` | siempre, como capa adicional independiente |
| `off` | nunca |

El cambio de usuario lo hace el lanzador, no el servidor. El servidor sigue
sin ser root y sin ninguna capacidad; el lanzador lleva una capacidad de
archivo y cambia al UID 1001 justo antes de ejecutar el plugin:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

El lanzador rechaza cualquier uid o gid de destino por debajo de 1000. Cuando
no puede cambiar de usuario (falta la capacidad, `no_new_privs` activado), un
plugin en modo `basic` se rechaza, porque ahí el uid es su única barrera; con
Landlock, registra un aviso y se ejecuta con el uid del servidor.

!!! warning
    La separación de uid es incompatible con `no_new_privs`. Si el servidor
    funciona bajo systemd con `NoNewPrivileges=yes`, la capacidad de archivo
    se ignora al ejecutar: los plugins se rechazan en modo `basic` y, con
    Landlock, la separación de uid se queda en el mismo uid. Elige una de las
    dos cosas.

Si en cambio el servidor funciona como root (no recomendado), hace pasar
directamente los plugins al UID 1001 y la capacidad no hace falta. La
explicación en lenguaje sencillo de cada caso está en
[Configurar el sandbox de plugins](plugins/sandbox-deployment.md).

## Permisos de los archivos secretos

Haz lo mismo que la imagen: las carpetas que contienen secretos son
accesibles solo para su propietario, así que un plugin que pasa al UID 1001
no puede leerlas ni siquiera sin Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# el servidor es propietario de plugins/ (ahí extrae las instalaciones); el grupo
# evey, al que pertenece evey-plugin, solo lee y ejecuta los archivos instalados:
sudo chmod 750 /var/lib/evey/plugins
# solo el propietario en las carpetas secretas:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la propia clave de firma JWT (generada en 0600 en el primer arranque):
sudo chmod 600 /var/lib/evey/jwt.key
```

Adapta las rutas a donde apunten las variables `EVEY_*_DIR` y
`EVEY_JWT_PRIVATE_KEY_PATH`.

## Configurar el entorno

Toda la configuración se hace con variables de entorno `EVEY_*`;
[Referencia de configuración](configuration.md) tiene la tabla completa.
Puntos propios de una instalación sin contenedor:

- Apunta `EVEY_DB_HOST` y `EVEY_MEILI_URL` a tu PostgreSQL y a tu
  Meilisearch. `EVEY_DB_PASSWORD` no puede ser el valor por defecto `evey`.
- Las rutas por defecto del código Go se eligieron para el contenedor:
  define `EVEY_LOG_DIR` (por defecto la ruta relativa `logs`),
  `EVEY_BACKUP_DIR` (`/data/backups`), `EVEY_TRANSCODING_CACHE_DIR`
  (`/data/transcode`), `EVEY_TRICKPLAY_CACHE_DIR` (`/cache/trickplay`) y
  `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`) con tus propias carpetas.
- El servidor sirve HTTP sin cifrar: pon delante un proxy inverso para TLS,
  con una **URL externa** en `https://` y el proxy en **Proxies de
  confianza** (**Administración -> Acceso remoto**; `EVEY_SERVER_BASE_URL` y
  `EVEY_TRUSTED_PROXIES` son los valores de respaldo). Consulta
  [Configuraciones de proxy inverso](reverse-proxy.md).
- **El servidor abre dos puertos, y solo uno puede quedar expuesto.**
  `EVEY_SERVER_PORT` (por defecto `9090`) es la escucha gRPC nativa; la API
  HTTP con la que hablan tus clientes y tu proxy inverso usa ese puerto más
  uno (`9091`). El control de acceso remoto, que decide si los clientes de
  fuera de tu red local pueden conectarse, está en la cadena HTTP y no cubre
  la escucha gRPC. Por eso esta solo escucha en la interfaz local por defecto
  (`EVEY_GRPC_BIND=127.0.0.1`). Si tienes un motivo para publicarla, define
  `EVEY_GRPC_BIND=0.0.0.0`; el servidor avisa al arrancar de que el control
  no se le aplica, y entonces te toca a ti protegerla con un cortafuegos.
- Las migraciones se ejecutan al arrancar; no hay un paso de migración
  aparte.
- Los límites de recursos de los plugins (`EVEY_PLUGINS_MEMORY_MB`,
  `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) y el modo de sandbox se pueden
  cambiar en marcha desde **Administración -> Plugins -> Límites de
  recursos**; los valores del entorno son los valores iniciales.
- El reinicio del servidor desde el panel de administración depende del
  supervisor de la imagen y se indica como no disponible sin contenedor:
  reinicia el servicio tú mismo.

## Verificar el sandbox

El servidor registra el mecanismo que eligió. Cuando arranque el primer
plugin, busca:

- una vez, para `auto`: `plugin sandbox auto-detection complete
  selected_mode=landlock` (o `bubblewrap`, `basic`);
- en cada arranque de plugin: `plugin sandbox applied sandbox_mode=landlock
  plugin_id=...`;
- el aviso que no quieres ver si esperabas Landlock: `plugin sandbox
  auto-detection fell back to basic mode: no per-path filesystem isolation
  available ...`, que significa que no se encontró ni Landlock ni un `bwrap`
  que funcione. Vuelve a revisar la sección sobre los mecanismos de sandbox.

Confirma la parte del kernel por separado:

```bash
cat /sys/kernel/security/lsm              # muestra "landlock"
grep Seccomp: /proc/<plugin-pid>/status   # 2 = hay un filtro seccomp activo en el plugin
```

Una línea `Seccomp: 2` en un proceso de plugin en marcha confirma que el
filtro del kernel está instalado. Con `EVEY_PLUGINS_SANDBOX_MODE=landlock` y
Landlock sin aplicar, el plugin se niega a arrancar: es el cierre por defecto
funcionando como debe.

Para las copias de seguridad, las actualizaciones y los comandos de
recuperación, las páginas del contenedor sirven tal cual:
[Copias de seguridad y actualizaciones](backups.md),
[Recuperación](runbooks/recovery.md) y [Línea de comandos](runbooks/cli.md)
describen el mismo binario `evey admin`.

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/en/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
