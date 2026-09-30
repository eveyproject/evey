---
source_sha: a919f7b655a1b0f7627661e72171149d235f92679027f0fe8b24a98631bd4185
reviewed: false
---
# Despliegue sin contenedor (sin Docker)

Ejecutar Evey directamente en un equipo (systemd, un script de init o a mano) en
lugar de la [imagen Docker](installation.md). La imagen Docker es el camino
recomendado: trae el sandbox de plugins, el endurecimiento y los entornos de
ejecución ya conectados. Ir sin contenedor significa que **tú** reproduces esa
postura: los binarios correctos uno junto a otro, un usuario dedicado no root, el
backend de sandbox que admite tu kernel y directorios de secretos accesibles solo
para su propietario. Esta página es el paso a paso para eso.

Si solo quieres la instalación sencilla, para aquí y usa
[installation.md](installation.md).

## 1. Requisitos previos

### Binarios (compilar desde las fuentes)

Compila los dos binarios Go y **colócalos en el mismo directorio**. El ejecutor
de plugins localiza el lanzador comprobando primero `$EVEY_PLUGIN_LAUNCHER`,
después junto a su propio ejecutable (`os.Executable()`) y después en `$PATH`:
tenerlos juntos es la disposición correcta más sencilla.

```bash
# desde la raíz del repositorio
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# instalar uno junto a otro, por ejemplo:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# opcional: autocompletado del shell y página de manual, ambos generados por el binario
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

Orden de búsqueda en `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (debe ser un archivo ejecutable): anulación explícita.
2. `evey-plugin-launcher` en el **mismo directorio** que el binario `evey` en ejecución.
3. `evey-plugin-launcher` en algún lugar del `$PATH`.

### El shim seccomp LD_PRELOAD

Compila el shim `libevey-sandbox.so` (un pequeño archivo C; necesita un
compilador de C):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

El ejecutor lo busca en la **ruta fija `/app/libevey-sandbox.so`**
(`sandboxLibPath` en `runner.go`). Si falta, los plugins **se niegan a arrancar**
(fallo en cerrado), salvo en modo desarrollo. Este shim añade una lista de
denegación seccomp para los plugins enlazados dinámicamente y se superpone a
Landlock/bubblewrap.

> La imagen Docker guarda los binarios de código abierto, el SDK y este shim en
> `/app` (ningún secreto ahí). Sin contenedor puedes reproducir esa ruta
> (`/app/libevey-sandbox.so`) o crear un enlace simbólico.

### Entornos de ejecución

Instala lo que invoque el `entrypoint:` de tus plugins. El ejecutor reconoce
estos intérpretes (`knownInterpreters` en `runner_start.go`): `python3`, `python`,
`node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Como mínimo:

- **`python3`**: la mayoría de los plugins de la comunidad.
- **`node`**, **`php`**, **`ruby`**, **`lua`**: solo si tus plugins los usan.

Más las dependencias básicas de ejecución:

- **ffmpeg** (transcodificación; la imagen también incluye los controladores
  VAAPI + `chromaprint` para la detección de intros).
- **PostgreSQL** (servidor + una base de datos).
- **Meilisearch** (índice de búsqueda; o ejecutarlo integrado mediante el binario
  oficial).
- **`bwrap`** (bubblewrap): solo si piensas usar el backend de sandbox bubblewrap
  (ver abajo).
- **`libcap`** (`setcap`): para conceder la capacidad de archivo descrita en §4.

## 2. Ejecutar con un usuario dedicado no root

**Nunca ejecutes el servidor como root.** Crea un grupo y dos usuarios sin
privilegios, como hace la imagen Docker; los plugins bajan al UID 1001
(`pluginUID` en `pluginhost/host_server.go`):

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

El id del grupo importa: el lanzador baja un plugin al UID 1001 y al grupo propio
del servidor, y rechaza cualquier id por debajo de 1000. Un grupo creado solo con
`useradd --system` recibe un id de sistema por debajo de 1000, y los plugins en
modo `basic` se rechazan entonces.

- `evey` (UID 1000) ejecuta el servidor.
- `evey-plugin` (UID 1001) es el UID al que bajan los plugins (lo hace el lanzador
  cuando la separación de uid está activa), así que un plugin comprometido acaba
  como un usuario distinto y sin privilegios que no puede leer los secretos del
  servidor con los permisos normales del sistema de archivos. Consulta §4.

El servidor se marca como **no volcable** al arrancar (`PR_SET_DUMPABLE=0`,
`cmd/evey/hardening_linux.go`) para que un plugin que corre en la misma máquina
no pueda leer `/proc/<server-pid>/{mem,environ}`.

## 3. Elegir el backend de sandbox para tu kernel

Define `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (el valor por defecto) resuelve
**landlock → bubblewrap → basic** y elige el más fuerte que admite tu equipo
(`resolveAutoMode()` en `runner_landlock.go`). Elige de forma explícita si
quieres un comportamiento de fallo en cerrado.

### Landlock (recomendado)

Una lista de permitidos del sistema de archivos por ruta (los plugins solo ven su
propio directorio + el directorio de sockets, nunca tus datos ni tus secretos)
más un filtro de salida TCP Landlock v4. **No necesita privilegios, montajes ni
espacios de nombres**: el lanzador se lo aplica a sí mismo y después hace
`execve` del plugin.

**Requisito del kernel**: Landlock debe estar compilado *y* en la lista de LSM
activos. `landlockAvailable()` del ejecutor (`landlock_probe_linux.go`) sondea
`landlock_create_ruleset(VERSION)`; devuelve un errno cuando la llamada al sistema
no existe (kernel `<5.13`) o cuando Landlock no está en la lista de LSM activos.
Compruébalo:

```bash
uname -r                              # se quiere >= 5.13
cat /sys/kernel/security/lsm          # debe incluir "landlock"
```

Si falta `landlock` en esa lista, actívalo en la línea de comandos del kernel y
reinicia (la lista exacta varía según la distribución: conserva las entradas
existentes y añade `landlock`):

```
# por ejemplo en GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Después:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # explícito + fallo en cerrado (rechaza un plugin si Landlock no puede aplicarse)
# o déjalo en el valor por defecto:
EVEY_PLUGINS_SANDBOX_MODE=auto        # resuelve a landlock cuando está disponible
```

El modo `landlock` pasa `-require-landlock` al lanzador, que termina con un
código distinto de cero en lugar de ejecutar un plugin sin confinar si Landlock
no se aplica.

### bubblewrap

Si no puedes tener Landlock, instala el binario `bwrap`. bubblewrap coloca cada
plugin en espacios de nombres user/pid/uts/ipc/(net) con una lista blanca de
montajes. Necesita **los espacios de nombres de usuario sin privilegios
activados** y **la llamada al sistema `mount` disponible**: es así en la mayoría
de los Linux sin contenedor, pero está bloqueado dentro de un contenedor Docker
no privilegiado (precisamente por eso Landlock es el valor por defecto en el
contenedor). `auto` solo elige bubblewrap cuando Landlock **no existe** y una
sonda bwrap desechable tiene éxito de verdad (`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espacios de nombres bwrap
# o:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + el shim seccomp LD_PRELOAD
```

Comprueba que el equipo permite realmente la configuración de espacios de
nombres + montaje:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (último recurso)

`EVEY_PLUGINS_SANDBOX_MODE=basic`: bajada de UID + el shim seccomp LD_PRELOAD, y
el lanzador sigue instalando la lista de denegación seccomp del kernel y Landlock
en lo posible si existe. **No hay confinamiento del sistema de archivos por
ruta**: un plugin se ejecuta con el UID 1001 y solo seccomp. Úsalo solo en
kernels antiguos o cerrados donde no hay ni Landlock ni bubblewrap, y apóyate en
la separación de UID de §4 para proteger los secretos.

> **Un plugin con el permiso `subprocess` necesita el lanzador**, incluso en modo
> basic. El shim LD_PRELOAD no sobrevive a un `exec` de un proceso hijo, así que
> no puede confinar un plugin al que se permite lanzar subprocesos: solo lo hace
> el seccomp del kernel del lanzador (instalado justo antes del `exec` y heredado
> a través de él). Si falta `evey-plugin-launcher` (ver §1), un plugin así **se
> niega a arrancar** en producción (fallo en cerrado) en lugar de ejecutarse sin
> confinar. Los plugins sin ese permiso recurren al shim LD_PRELOAD como se
> describe arriba.

## 4. Separación de UID (proteger los secretos sin root)

La separación de UID ejecuta cada plugin con un usuario aparte (UID 1001,
`evey-plugin`), distinto del servidor (UID 1000), para que un plugin no pueda
leer el `jwt.key`, las copias ni los registros del servidor con los permisos
normales del sistema de archivos (DAC). Importa sobre todo en modo **`basic`**
(sin confinamiento por ruta de Landlock); con Landlock o bubblewrap el sistema de
archivos ya está tapiado, así que ahí es redundante.

Se controla con `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, por defecto
`auto`):

- `auto`: se activa solo cuando el sandbox resuelve a `basic`. En un kernel
  moderno sin contenedor (Landlock) queda desactivada, a propósito.
- `on`: siempre, incluso con Landlock/bubblewrap (una capa independiente más).
- `off`: nunca.

**La bajada la hace el lanzador, no el servidor.** El servidor sigue sin ser root
y **sin** capacidades; el pequeño binario `evey-plugin-launcher` lleva una única
capacidad de archivo y hace `setuid` a 1001 justo antes de ejecutar el plugin:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Así el servidor queda totalmente sin privilegios: una RCE en el servidor no puede
hacer `setuid(0)` porque el servidor no tiene capacidades; solo las tiene el
diminuto lanzador sin entradas (y rechaza cualquier UID de destino por debajo de
1000). Si el lanzador no puede bajar (falta la capacidad, o `no_new_privs` está
activado), un plugin en modo `basic` se **rechaza**, porque ahí el UID es su única
barrera para los secretos; con Landlock registra un aviso y se ejecuta con el UID
del servidor.

> **Incompatible con `no_new_privs`.** Si lanzas el servidor con
> `no-new-privileges` (o `NoNewPrivileges=yes` de systemd), la capacidad de
> archivo se ignora al ejecutar: los plugins se rechazan en modo `basic`, y con
> Landlock la separación de uid se degrada al mismo UID (con un aviso). Elige una
> de las dos.

Si en cambio el servidor se ejecuta **como root** (no recomendado), baja los
plugins a 1001 directamente y no hace falta la capacidad del lanzador.

Consulta la guía en lenguaje sencillo para el panorama por escenario:
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Permisos de los archivos secretos

Reproduce lo que hace la imagen Docker (`Dockerfile`): mantén los directorios con
secretos **accesibles solo para su propietario** para que un plugin bajado al
UID 1001 no pueda leerlos con DAC normal, incluso sin Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# El SERVIDOR es propietario de plugins/ (ahí extrae las instalaciones); el grupo
# evey, al que pertenece evey-plugin, solo lee/ejecuta los archivos instalados:
sudo chmod 750 /var/lib/evey/plugins
# solo el propietario (ni grupo ni otros) en los directorios secretos:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la propia clave de firma JWT:
sudo chmod 600 /var/lib/evey/jwt.key      # (generada con 0600 en el primer arranque)
```

(Ajusta las rutas a donde apunten `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH`.)

## 6. Entorno y configuración

Toda la configuración de ejecución se hace con variables de entorno `EVEY_*`.
Define la base de datos, la búsqueda, las rutas de almacenamiento, la ruta de la
clave JWT y las variables del proxy inverso igual que en el despliegue con
Docker: consulta [configuration.md](configuration.md) para la tabla completa e
[installation.md](installation.md) para el recorrido de base de datos, búsqueda y
proxy inverso. No dupliques la tabla aquí. Notas propias del despliegue sin
contenedor:

- Apunta `EVEY_DB_HOST` / `EVEY_MEILI_URL` a tu PostgreSQL / Meilisearch local.
- El servidor sigue sirviendo **HTTP sin cifrar**: pon un proxy inverso delante
  para TLS (una URL externa `https://` y el proxy en Proxies de confianza, en
  **Admin → Acceso remoto**, o `EVEY_SERVER_BASE_URL` y
  `EVEY_TRUSTED_PROXIES`). Consulta [reverse-proxy.md](reverse-proxy.md).
- **El servidor abre DOS puertos, y solo uno de ellos puede exponerse.**
  `EVEY_SERVER_PORT` (por defecto `9090`) es el escuchador gRPC nativo; la API
  HTTP con la que hablan tus clientes y tu proxy inverso es **ese puerto más uno**
  (`9091`). Apunta tu proxy inverso y tu cortafuegos solo al puerto HTTP. El
  puerto gRPC es un transporte interno: el bloqueo de red del acceso remoto, que
  decide si los clientes de fuera de tu LAN pueden conectar, está en la cadena
  HTTP y **no cubre el escuchador gRPC**. Por eso el escuchador gRPC se enlaza
  **solo al loopback** por defecto (`EVEY_GRPC_BIND=127.0.0.1`), para que una
  ejecución sin contenedor no pueda exponerlo por accidente. Si tienes un motivo
  para publicarlo (un cliente gRPC nativo en otro equipo), define
  `EVEY_GRPC_BIND=0.0.0.0`; el servidor avisa al arrancar de que el bloqueo no
  se le aplica, y protegerlo con el cortafuegos pasa a ser cosa tuya.
- Las migraciones se ejecutan automáticamente al arrancar; no hay paso de
  migración aparte.
- Los límites de recursos de los plugins (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) y el modo de sandbox se pueden editar en caliente
  desde Admin → Plugins; los valores del entorno son solo los valores iniciales.

## 7. Verificar tu sandbox

El servidor registra el backend de sandbox resuelto. Cuando arranque el primer
plugin, mira el registro del servidor:

- **resolución auto** (se registra una vez):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (o `bubblewrap` / `basic`).
- **por plugin** (en cada arranque de plugin):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **vuelta a basic** (un aviso que *no* quieres ver si esperabas Landlock):
  `plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available …`: significa que no se encontró ni Landlock ni
  un bwrap que funcione. Revisa §3.

Confirma las piezas del kernel por separado:

```bash
cat /sys/kernel/security/lsm          # ¿Landlock activo? (debe listar "landlock")
grep Seccomp: /proc/<plugin-pid>/status   # 2 = modo filtro activo en un plugin en ejecución
```

Una línea `Seccomp: 2` en un proceso de plugin vivo confirma que el filtro
seccomp del kernel está instalado. Si fijaste `EVEY_PLUGINS_SANDBOX_MODE=landlock`
y Landlock no se aplica, el plugin **se niega a arrancar**: es el contrato de
fallo en cerrado funcionando como debe.

---

Para todo lo demás (copias, actualizaciones, CLI de recuperación, solución de
problemas) se aplican igual las secciones orientadas a Docker de
[installation.md](installation.md): los subcomandos de `evey admin` y la mecánica
de las copias son el mismo binario.
