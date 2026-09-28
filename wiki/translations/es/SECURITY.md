---
source_sha: 2afa372fc288cb82c9e90ad0f1bb879056bb0e14682a438cab6ce64d779bf48d
---
# Entorno aislado de extensiones y modelo de amenazas

> **¿Qué página busca?** Esta describe lo que hace el *servidor* para contener
> una extensión: las capas, los modos de aislamiento, lo que cada uno todavía no
> puede impedir y cómo configurarlo. Si está escribiendo una extensión y quiere
> saber qué puede declarar y qué se rechazará, lea en su lugar
> [el modelo de seguridad de las extensiones](plugins/security-model.md). Las dos
> páginas tratan el mismo tema desde extremos opuestos y, a propósito, no se
> repiten.

Este documento describe las capas de defensa que Evey aplica a las extensiones de terceros, los límites de esas defensas y la hoja de ruta hacia un aislamiento más fuerte.

## Modelo de amenazas: qué intentaría una extensión hostil

Las extensiones de Evey se ejecutan como procesos del sistema operativo separados que intercambian gRPC con el anfitrión por sockets Unix. El anfitrión no confía por defecto en ningún código de extensión. Los riesgos de los que nos defendemos, por orden de gravedad:

1. **Suplantación en las RPC del anfitrión** - una extensión maliciosa se hace pasar por otra para obtener sus permisos.
2. **Escalada de privilegios** - una extensión escapa de su entorno aislado y ejecuta órdenes arbitrarias como el usuario del servidor.
3. **Lectura / escritura en el sistema de archivos** - una extensión lee o escribe rutas fuera de su directorio de instalación.
4. **Exfiltración por red** - una extensión abre conexiones salientes para enviar datos a casa.
5. **Abuso de recursos** - minado de criptomonedas, bomba fork, agotamiento de descriptores de archivo.
6. **Cadena de suministro** - el autor de una extensión distribuye malware ofuscado o compilado que el analizador estático no puede inspeccionar.

## Capas de defensa (actuales)

### 1. Análisis estático en la instalación (VISTA PREVIA + INSTALACIÓN)

Cada ZIP de extensión se descomprime en un directorio temporal y lo recorre [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go). Categorías examinadas, con su gravedad:

| Categoría | Gravedad | Ejemplos |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `exec()`, `child_process.exec`, PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby comilla invertida/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, reverse/bind shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, operaciones con enlaces simbólicos |
| `obfuscation` | CRITICAL | `eval`, `exec`, decodificación base64, secuencias hexadecimales, `__builtins__`, PHP `assert(base64_decode(...))`, shell `base64 -d \| bash`, `echo <blob> \| openssl` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, señal al PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, Python `__subclasses__()`, `__class__.__bases__`, recorrido de la jerarquía de clases con `__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`/`pickle.load`, `marshal.loads`, `__reduce__`/`__reduce_ex__`; WARNING: `yaml.load()` sin Loader explícito |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer`, `unsafe.Slice`, `unsafe.SliceData`, `unsafe.StringData` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` (opacos para el escáner) |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` (código extraíble) |
| `mime_mismatch` | WARNING (tratado como crítico) | la extensión del archivo dice imagen, el contenido es otra cosa |
| `suspicious_url` | WARNING | esquema no HTTP en un literal de URL |
| `unvendored_deps` | WARNING | presencia de `requirements.txt`, `package.json` |

Los hallazgos CRITICAL o `mime_mismatch` bloquean la instalación (`Safe = false`).

`compiled_binary` y `embedded_archive` bloquean la instalación **por defecto** salvo que el manifiesto declare el permiso `native-binary`. El administrador ve el aviso y el permiso pedido en el paso de vista previa, antes de aceptar.

### 2. Control por los permisos del manifiesto

El `plugin.yaml` de una extensión declara los permisos que pide. El runner los aplica en dos niveles:

- **`subprocess`** - puede lanzar procesos hijos. Sin él, el shim seccomp por LD_PRELOAD deniega `execve`/`execveat` a las extensiones interpretadas. (El seccomp del núcleo del lanzador *no* deniega `execve` - tiene que ejecutar la extensión -, pero los hijos heredan igualmente el filtro + el dominio Landlock.)
- **`native-binary`** - necesario para incluir `.so` / `.dll` / `.exe` / archivos comprimidos. El administrador debe aceptar el código opaco al instalar.
- **Permisos de API** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) - comprobados en cada RPC del anfitrión por [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

### 3. Separación de UID (DAC)

Un proceso de extensión puede rebajarse al UID 1001 (`evey-plugin`), distinto del UID del servidor (1000, `evey`), para que los secretos del servidor le resulten ilegibles por simple DAC. Lo controla `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, por defecto `auto`):

- `auto` - rebaja **solo cuando el aislamiento se resuelve a `basic`** (sin confinamiento del sistema de archivos por Landlock/bubblewrap) - el único caso en que DAC es lo único que protege los secretos. Se omite con landlock/bubblewrap, donde es redundante.
- `on` - siempre (una capa independiente extra por encima de Landlock/bubblewrap).
- `off` - nunca.

**La rebaja la hace el lanzador, no el servidor.** El servidor sigue sin root y sin capacidades; `evey-plugin-launcher` lleva `cap_setuid` (una capacidad de archivo, `setcap cap_setuid+ep` en el `Dockerfile`) y hace `setgid`/`setuid` a sí mismo hacia el grupo compartido + UID 1001 justo antes del exec. Rechaza cualquier UID de destino inferior a 1000. Si no puede rebajar (sin capacidad, con `no_new_privs` activo o sin el binario del lanzador), el resultado depende del modo: en `basic`, donde el UID es el único muro alrededor de los secretos, la extensión se **rechaza** (falla cerrado); con `landlock`, donde el sistema de archivos ya está tapiado, avisa y ejecuta la extensión con el UID del servidor. Un servidor que corre como root rebaja las extensiones directamente mediante `SysProcAttr` y no necesita capacidad. Guía en palabras sencillas: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Lo que aporta, **incluso en un núcleo sin Landlock** (cuando está activa):

- Los secretos del servidor quedan protegidos por DAC frente a una extensión con uid 1001: `jwt.key` (`0600`) y las copias de seguridad (directorio `0700` + archivos `0600`) pasan a ser solo del propietario **en tiempo de ejecución**, así que la protección vale incluso con un `/data` montado desde el host; una extensión con uid 1001 no puede leerlos. (La imagen también aplica `chmod 700` a `/data/{keys,logs,backups}`, pero un `/data` montado desde el host conserva sus propios permisos - lo garantizado son los modos en ejecución de arriba.)
- Junto con `PR_SET_DUMPABLE=0` en el servidor (vea [§8](#8-server-process-hardening-pr_set_dumpable0)), la clave JWT en memoria y los secretos del entorno (`DB_PASSWORD`, `MEILI_MASTER_KEY`) no se pueden leer vía `/proc/<server>/{mem,environ,maps}`.

Todas las extensiones comparten el UID 1001, así que DAC por sí solo no las aísla *entre sí* - eso es lo que añade la capa de confinamiento del sistema de archivos (Landlock / bubblewrap). Con `basic` + separación de UID, una extensión maliciosa podría leer o perturbar los archivos de una hermana; aislar las extensiones entre sí sin Landlock requeriría un UID *por extensión*. Vea [§4](#4-syscall-confinement-kernel-seccomp--ld_preload), la tabla por modo más abajo y los [límites conocidos](#known-limits-of-the-sandbox).

### 4. Confinamiento de las llamadas al sistema (seccomp del núcleo + LD_PRELOAD)

Dos filtros seccomp aplican casi la misma lista negra (solo difieren en las llamadas de señales - vea la tabla); ambos llaman primero a `prctl(PR_SET_NO_NEW_PRIVS)` y luego instalan un programa BPF que responde EPERM a las llamadas denegadas y permite todo lo demás.

1. **Aplicado por el núcleo (la capa principal).** El shim [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) se antepone al argv de la extensión e instala el filtro BPF ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)) **justo antes de `execve`**, según la arquitectura del build. El filtro se hereda a través de `execve` en la extensión y en cualquier hijo que lance después. Como lo instala el lanzador y no `LD_PRELOAD`, **restringe también los binarios estáticos/compilados de las extensiones** - que nunca cargan `libevey-sandbox.so` y antes se saltaban el shim por completo. Una arquitectura que no coincide en el campo ABI de `seccomp_data` da `SECCOMP_RET_KILL_PROCESS` (frustra la confusión de números de llamada entre i386/x32).
2. **LD_PRELOAD (doble protección para las extensiones dinámicas).** [`sandbox/sandbox.c`](../sandbox/sandbox.c) es un pequeño constructor precargado en las extensiones interpretadas; la misma lista negra, y además fija `PR_SET_DUMPABLE=0` en la propia extensión.

**Cuál de los dos se ejecuta depende del modo de aislamiento** (vea la tabla por modo más abajo): el filtro del núcleo del lanzador se aplica en `landlock` y `basic`; el shim LD_PRELOAD en `strict` y `basic`. `bubblewrap`/`strict` **no** ejecutan el lanzador, así que un binario **estático** bajo `bubblewrap` a secas no recibe ninguna lista negra de llamadas al sistema - su contención ahí son los espacios de nombres de bwrap + el aislamiento del sistema de archivos, no seccomp.

Llamadas al sistema bloqueadas, agrupadas por categoría:

| Categoría | Llamadas bloqueadas |
|---|---|
| Control de procesos / depuración | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Señales a procesos | `kill` (ambos filtros); el lanzador deniega además `tgkill` sin más, mientras que el shim LD_PRELOAD deniega `tkill`/`tgkill` solo cuando apuntan al PID 1 (el servidor) |
| Montaje de sistemas de archivos | `mount`, `umount2`, `pivot_root`, `chroot` |
| Espacios de nombres | `unshare`, `setns` |
| Módulos del núcleo | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| E/S de hardware | `ioperm`, `iopl` (solo shim LD_PRELOAD) |
| Swap / gestión de memoria | `swapon`, `swapoff` |
| Control del sistema | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load` (ambos filtros); `kexec_file_load` (solo shim LD_PRELOAD) |
| Familia setuid | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capacidades | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

`execve`/`execveat` se **deniega en el shim LD_PRELOAD** (para que una extensión interpretada sin `subprocess` no pueda lanzar hijos) pero **NO en el filtro del núcleo del lanzador** - el propio lanzador tiene que hacer `execve` de la extensión. Los hijos lanzados heredan el filtro del lanzador + el dominio Landlock, así que siguen confinados en cualquier caso.

**Sin salida IP para NINGUNA extensión (desde 2026-07-11).** A toda extensión - con permiso de red o sin él - se le pasa `-deny-inet` (landlock/basic) o `--unshare-net` (bubblewrap): el proceso de la extensión no puede abrir ningún `socket(AF_INET)` / `socket(AF_INET6)`. Una extensión que necesita la red NO se conecta directamente; llama al intermediario `Fetch` del anfitrión por su socket `AF_UNIX` de HostService (que sigue permitido), y este aplica del lado del anfitrión la lista blanca `external_fetch` y una conexión segura frente a SSRF. Esto cierra el agujero de la salida por IP literales (p. ej. `169.254.169.254`) que no podían cerrar una lista blanca DNS por host y un filtro del núcleo por puerto. Vea [ADR-0014](adr/0014-landlock-plugin-sandbox.md).

Límites - léalos con atención:

- **Lista negra, no lista blanca.** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone`, etc. están todos permitidos. (El modo `strict` añade la capa LD_PRELOAD sobre bubblewrap; una verdadera lista blanca de llamadas sigue en la hoja de ruta.)
- **El aislamiento del sistema de archivos viene de otra capa.** seccomp no restringe rutas; eso lo hace Landlock (o bubblewrap). En un núcleo sin Landlock y sin bubblewrap, la extensión puede leer cualquier archivo legible por todos (pero no los secretos del servidor - están protegidos por DAC, vea §3).
- **Confinamiento de red.** La salida IP directa está cortada para toda extensión, con permiso de red o sin él: `-restrict-net` (sin concesiones `-net-connect`) más `-deny-inet` bloquean todos los sockets `AF_INET`/`AF_INET6` en el núcleo. No hay filtro del núcleo por puerto ni por host. Una extensión con permiso de red solo llega a la red a través del intermediario `Fetch` del anfitrión por su socket `AF_UNIX`, que aplica la lista blanca `external_fetch` del lado del anfitrión.
- **Falla cerrado para las extensiones interpretadas.** Si el runner no encuentra `libevey-sandbox.so` y la extensión no tiene `subprocess`, [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) se niega a arrancarla y emite un evento `plugin.error`. El seccomp del núcleo del lanzador funciona en la medida de lo posible (un núcleo sin seccomp recae en las demás capas).

### 5. Socket gRPC del anfitrión por extensión + autenticación SO_PEERCRED

El anfitrión expone un socket Unix por extensión en `/tmp/evey/plugins/<plugin_id>/host.sock`. Cada socket lo atiende un servidor gRPC dedicado con un interceptor que:

1. Lee el PID del par mediante `getsockopt(SO_PEERCRED)` - **ese valor lo proporciona el núcleo, no se puede falsificar en espacio de usuario**.
2. Lo compara con el PID que el runner registró con `SetExpectedPID` justo después de `cmd.Start`.
3. Estampa el ID de la extensión en el contexto gRPC mediante un closure (el ID sale de la identidad del socket que escucha, nunca del flujo de datos).

Efecto:

- Una extensión no puede hacerse pasar por otra enviando una cabecera `x-plugin-id` (la cabecera se ignora).
- Una extensión no puede alcanzar el socket de anfitrión de otra y llamar a sus RPC - aunque haga `connect()`, la comprobación del PID la rechaza con `Unauthenticated: PID mismatch`.
- Una extensión que lanza un hijo con fork/exec no puede enviar llamadas al anfitrión a través del hijo (otro PID → rechazada). Es intencionado y concuerda con el control del permiso `subprocess`.

### 6. Control por la vista previa del administrador

`POST /api/v1/admin/plugins/preview` ejecuta el analizador estático y devuelve el manifiesto + los hallazgos sin instalar. El administrador ve:

- Los permisos pedidos (incluidos `native-binary`, `subprocess`, `network`, `api.*`).
- Todos los hallazgos con gravedad, archivo, línea y categoría.
- Las URL extraídas agrupadas por tipo (literal, plantilla, concatenación).
- El SHA256 de la fuente (coincide con la instalación que viene).

El administrador es la frontera de confianza para los permisos elevados - Evey muestra la superficie, el administrador acepta o rechaza.

### 7. Aviso de caída de una extensión

El runner publica un evento de dominio `plugin.error` cuando:

- Falta la biblioteca del entorno aislado y la extensión no tiene `subprocess`.
- El proceso de la extensión termina con un código distinto de cero.
- El interceptor de autenticación registra un intento con PID equivocado (solo slog de nivel aviso - todavía no conectado al bus).

El servicio de notificaciones se suscribe y avisa al administrador por el canal configurado (web push / correo / webhook).

### 8. Endurecimiento del proceso del servidor (`PR_SET_DUMPABLE=0`)

Al arrancar, el servidor llama a `prctl(PR_SET_DUMPABLE, 0)` ([`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go)). Esto pasa a root la propiedad de `/proc/<server>/{mem,environ,maps,fd,…}`, así que un proceso que comparta el UID del servidor no puede leerlos. Importa sobre todo en el repliegue sin Landlock (p. ej. un núcleo antiguo de Synology), donde una extensión podría si no rascar por `/proc` la clave JWT en memoria del servidor y sus secretos de entorno (`DB_PASSWORD`, `MEILI_MASTER_KEY`). En los núcleos con Landlock, a la extensión ya se le niega el `/proc` del servidor; esta es la doble protección válida para cualquier núcleo. No afecta a la lectura de sus propios archivos por el servidor; un fallo se registra, no es fatal.

## Modos de aislamiento

`sandbox_mode` se configura con `evey.yaml` o `EVEY_PLUGINS_SANDBOX_MODE` (por defecto `auto`). Elige el **backend** de confinamiento, y el mecanismo de llamadas al sistema/salida depende del backend: `landlock` y `basic` ejecutan la extensión a través del lanzador, que instala la lista negra seccomp del núcleo + `-deny-inet`; `bubblewrap`/`strict` aíslan en cambio la red con un espacio de nombres (`--unshare-net`) y no tienen seccomp del lanzador (`strict` solo añade el shim LD_PRELOAD, válido solo para las dinámicas). Lo que SÍ vale en todos los modos: la separación de UID, el monitor de recursos en espacio de usuario y el hecho de que **la salida IP directa está cortada** (por `-deny-inet` o `--unshare-net`), así que una extensión de red solo sale a través del intermediario `Fetch` del anfitrión. El runner elige el backend en [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go); `auto` se resuelve una vez, en el primer arranque de una extensión ([`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go)).

### `auto` (por defecto)

Resuelve el mejor backend que admite de verdad el host, en este orden: **landlock → bubblewrap → basic**.

- Prefiere **landlock**: no necesita privilegios, funciona con el seccomp + AppArmor por defecto de Docker sin ningún cambio en el host y no se puede relajar una vez aplicado - el valor por defecto correcto para el despliegue principal en contenedor.
- Recae en **bubblewrap** solo donde falta Landlock *y* un entorno bwrap de prueba se puede montar de verdad ([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) - bare metal / contenedor con privilegios con userns + mount utilizables).
- Si no, **basic**. El modo resuelto se registra en el primer arranque de una extensión.

### `landlock`

- **Confina:** el sistema de archivos mediante una lista blanca Landlock LSM que aplica el lanzador - directorios del sistema (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`, `/dev`) en solo lectura/ejecución; el directorio de instalación propio de la extensión + su directorio de socket en lectura-escritura; todo lo demás (en particular `/data`, los directorios de extensiones vecinas) denegado. Red: **ninguna salida IP directa para ninguna extensión** - `-restrict-net` (sin concesión `-net-connect`) más `-deny-inet` bloquean todos los sockets `AF_INET`/`AF_INET6`; las extensiones con permiso de red solo llegan a la red por el intermediario `Fetch` del anfitrión a través de su socket `AF_UNIX`. Además: la lista negra seccomp del núcleo + LD_PRELOAD.
- **Requiere:** núcleo ≥ 5.13 con Landlock en la lista de LSM activos, y el binario `evey-plugin-launcher`. Sin mount, sin unshare, sin privilegios - funciona limpio con el perfil de seguridad por defecto de Docker.
- **Se elige cuando:** `sandbox_mode=landlock` (la extensión falla cerrado si no se puede imponer Landlock) o `auto` se resuelve a él (el caso normal de la imagen oficial).
- **Límites:** la salida IP directa está cortada para toda extensión (`-deny-inet` se mantiene incluso con ABI de Landlock <4); la lista blanca por host vive ahora del lado del anfitrión en el intermediario `Fetch`, no en el núcleo. El confinamiento del sistema de archivos se mantiene en cualquier caso.

### `bubblewrap`

- **Confina:** sistema de archivos + red + vista de procesos mediante 6 espacios de nombres separados (`--unshare-user/-pid/-uts/-ipc/-cgroup/-net` - `--unshare-net` se aplica sin condiciones a **toda** extensión; la salida directa pasa por el intermediario) y una lista blanca de montajes - directorios del sistema en solo lectura, un directorio de extensión + socket con escritura, un `/proc` sintético y un `/dev` mínimo, un `/tmp` en tmpfs; `/data`, `/var`, `/home`, `/root` y los directorios vecinos simplemente no se montan. `applySandbox` envía este modo a `applyBwrapSandbox`, que nunca invoca al lanzador: el seccomp del núcleo del lanzador **no** se aplica aquí, y tampoco el shim LD_PRELOAD (eso es solo `strict`). El confinamiento de las llamadas al sistema viene solo de los espacios de nombres separados.
- **Requiere:** el binario `bwrap` Y las llamadas `mount`/`unshare`; dentro de Docker eso necesita relajar la seguridad, así que en la práctica solo vale para bare metal (o un contenedor con privilegios).
- **Se elige cuando:** `sandbox_mode=bubblewrap`, o `auto` en un host sin Landlock donde pasa la sonda de bwrap.
- **Límites:** `--unshare-cgroup` es un *espacio de nombres* de cgroup, no un límite de recursos (esos vienen del monitor en espacio de usuario). En este modo no se aplica ninguna lista negra seccomp del núcleo (vea arriba); tampoco se usa `-deny-inet`, porque `--unshare-net` ya bloquea todos los sockets IP.

### `strict`

- Igual que `bubblewrap` **más** la biblioteca seccomp por LD_PRELOAD encima. Mismos requisitos y límites que bubblewrap. (Una verdadera *lista blanca* de llamadas al sistema sigue en la hoja de ruta.)

### `basic`

- **Confina:** rebaja al UID 1001 + la lista negra seccomp del núcleo + `-deny-inet` + Landlock en la medida de lo posible - todo aportado envolviendo la extensión en el lanzador, cuando el binario del lanzador está presente (lo está en la imagen oficial). Si falta el lanzador (algunas imágenes de desarrollo), se degrada a solo LD_PRELOAD + rebaja de UID.
- **Se elige cuando:** `sandbox_mode=basic`, o `auto` recae en él (sin Landlock y con bubblewrap inutilizable - p. ej. un núcleo antiguo muy cerrado).
- **Límites:** es el modo más débil en confinamiento del *sistema de archivos*: cuando el núcleo no tiene Landlock **no hay confinamiento del sistema de archivos por ruta**, así que una extensión puede leer archivos no sensibles legibles por todos, y el *listado* de procesos en `/proc` es visible. **No** es «sin aislamiento»: los secretos del servidor siguen protegidos por DAC (separación de UID + `PR_SET_DUMPABLE=0`), la extensión no tiene salida IP (`-deny-inet`), las llamadas peligrosas se bloquean en el núcleo (incluidos los binarios estáticos) y el monitor limita los recursos.

### Resumen de la postura

| Despliegue | Confinamiento del sistema de archivos | Red | Llamadas peligrosas | Secretos | Veredicto |
|---|---|---|---|---|---|
| Docker + Landlock (`auto`→landlock) | lista blanca por ruta | sin salida directa (`-deny-inet`); por intermediario | bloqueadas (incl. estáticos) | protegidos | **fuerte** |
| Docker sin Landlock (p. ej. NAS antiguo, `auto`→basic) | ninguno (lo legible por todos es visible) | sin salida IP (`-deny-inet`) | bloqueadas (incl. estáticos) | protegidos (DAC + DUMPABLE + seccomp) | correcto, no equivalente a Landlock |
| Bare metal (`auto`→bubblewrap) | espacio de nombres + lista blanca de montajes | espacio de nombres de red | los espacios de nombres (PID/mount/net) aíslan; seccomp LD_PRELOAD en `strict` - el filtro del núcleo del lanzador *no* se aplica en modo bwrap | protegidos | **el más fuerte** |

## Límites conocidos del entorno aislado

| Límite | Impacto | Mitigación actual | Hoja de ruta |
|---|---|---|---|
| Sin confinamiento del sistema de archivos por ruta en modo `basic` (host sin Landlock) | una extensión puede leer archivos no sensibles legibles por todos; los directorios de extensiones vecinas también son visibles | los secretos del servidor siguen protegidos por DAC (separación de UID + `PR_SET_DUMPABLE=0`); el analizador señala patrones como `/etc/passwd`; `auto` prefiere Landlock/bubblewrap donde los hay | - (Landlock o bubblewrap son la respuesta; ambos ya se distribuyen) |
| Todas las extensiones comparten el UID 1001 | una extensión maliciosa puede enviar señales a sus hermanas (las señales al PID 1 / al servidor las bloquea seccomp) | `PR_SET_DUMPABLE=0` cierra las lecturas de `/proc/<pid>/{environ,mem,maps,fd}` entre procesos del mismo UID; Landlock/bubblewrap aíslan por completo los directorios de las extensiones | espacio de nombres de usuario por extensión con bubblewrap |
| seccomp es una lista negra, no una lista blanca | la extensión puede hacer libremente `open`/`read`/`write`/`socket` (AF_UNIX)/`connect`; ptrace, mount, espacios de nombres, bpf, kexec, carga de módulos, familia setuid, keyring y sockets IP (todas las extensiones) se bloquean en el núcleo, también para binarios estáticos - vea [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | seccomp del núcleo mediante el lanzador (antes del exec, cubre binarios estáticos) + `-deny-inet`; control por el permiso `subprocess` | verdadero modo de lista blanca de llamadas para las extensiones `strict` |
| El intermediario traslada la superficie SSRF al anfitrión | un fallo en la validación de URL/IP del intermediario podría dejar que una extensión alcanzara una dirección interna a través del anfitrión | el intermediario aplica en Go la lista blanca `external_fetch` + una conexión segura frente a SSRF (IP privadas/reservadas rechazadas al conectar); los operadores DEBERÍAN añadir un cortafuegos de salida al contenedor (descartar `169.254.0.0/16`, permitir BD+Meili) como capa independiente | nftables por espacio de nombres / salida dedicada del intermediario (V2) |
| El analizador por expresiones regulares se puede sortear con ofuscación | un atacante motivado puede ocultar patrones | vista previa del administrador + revisión de permisos; la frontera de aplicación es el entorno aislado, no el analizador | puntuación de riesgo en el servidor + comprobación de la reputación del autor |

## Configuración de `sandbox_mode`

Configurable con `evey.yaml` (o la variable de entorno `EVEY_PLUGINS_SANDBOX_MODE`); el desglose detallado por modo está en la sección [Modos de aislamiento](#sandbox-modes) de arriba:

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, por defecto `auto`) es un mando aparte que controla si las extensiones se ejecutan con un UID distinto (vea [§3](#3-uid-separation-dac)): `auto` la activa solo cuando el modo se resuelve a `basic`, `on` la fuerza en todas partes (capa independiente extra), `off` la desactiva. La lleva el lanzador, falla cerrado en `basic` (en la medida de lo posible solo con `landlock`); incompatible con `no_new_privs`. Recorrido por escenario: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Correspondencia por modo (todos los valores dependen de lo que permita el host - p. ej. Landlock requiere un núcleo ≥ 5.13 con Landlock en la lista de LSM activos):

| Capa | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| Separación de UID (1001) - `EVEY_PLUGINS_UID_SEPARATION`, por defecto `auto` | activa solo en basic | desactivada (ponga `on` para forzarla) | activa | por userns¹ | por userns¹ |
| seccomp del núcleo (lanzador, antes del exec, cubre binarios estáticos) | sí si se resuelve a landlock/basic; no si se resuelve a bubblewrap | sí | sí | no (bwrap no tiene seccomp del lanzador; `applySandbox` envía bubblewrap/strict a `applyBwrapSandbox`, que nunca invoca al lanzador) | no (strict solo añade el shim LD_PRELOAD sobre bwrap - solo extensiones dinámicas, sin seccomp del núcleo) |
| seccomp por LD_PRELOAD (extensiones interpretadas) | sí | sí | sí | no | sí |
| `-deny-inet` (ningún socket IP para NINGUNA extensión) | sí | sí | sí | n/a (espacio de red) | n/a (espacio de red) |
| Confinamiento del sistema de archivos | se resuelve a lo mejor disponible | lista blanca Landlock | ninguno (solo DAC) | lista blanca de montajes | lista blanca de montajes |
| Red (todas las extensiones: sin salida IP directa; salida por el intermediario `Fetch` del anfitrión) | `-deny-inet` | `-restrict-net` + `-deny-inet` | `-deny-inet` | `--unshare-net` | `--unshare-net` |
| Espacios de nombres (user/pid/uts/ipc/cgroup) | n/a | n/a | n/a | separados | separados |
| Límites de recursos | monitor en espacio de usuario + `nice` | monitor en espacio de usuario + `nice` | monitor en espacio de usuario + `nice` | monitor en espacio de usuario + `nice` | monitor en espacio de usuario + `nice` |

¹ En `bubblewrap`/`strict` no se usa el lanzador, así que la rebaja por `cap_setuid` no se aplica; el **espacio de nombres de usuario** separado aporta en su lugar el aislamiento de UID.

Nota: una verdadera *lista blanca* de llamadas al sistema para `strict` sigue en la hoja de ruta; hoy `strict` = bubblewrap + la lista negra LD_PRELOAD.

Dependencias de la implementación:

- `evey-plugin-launcher` (Go puro, sin CGO), distribuido junto al binario del servidor - aplica Landlock + el filtro seccomp del núcleo. Lo usan `landlock`, `auto` y `basic`.
- `bubblewrap`, instalado con `apt-get` en la etapa de ejecución del `Dockerfile` (paquete Debian, ~200 KB); lo usan `bubblewrap`/`strict`.
- Para bubblewrap: espacios de nombres de usuario sin privilegios activados en el núcleo del host (`kernel.unprivileged_userns_clone=1` donde exista ese sysctl). Dentro de Docker, montar los espacios de nombres/montajes suele necesitar `--security-opt seccomp=unconfined` o un AppArmor relajado - por eso `auto` prefiere Landlock para el despliegue en contenedor.

## Salida de red de las extensiones (por intermediario, desde 2026-07-11)

Las extensiones **no tienen acceso directo a la red**: el entorno aislado deniega
todos los sockets IP (`--unshare-net` / `-deny-inet`) a toda extensión, sea cual
sea el permiso `network`. Una extensión que necesita la red llama a la RPC del
anfitrión `HostService.Fetch` por su socket `AF_UNIX` de anfitrión, condicionada
al permiso `network`. El anfitrión (`internal/adapters/driving/pluginhost/fetch.go`)
entonces, en Go:

- comprueba que el host de destino está en la lista blanca `external_fetch`
  declarada por la extensión (ahora un verdadero contrato por host, no una pista
  DNS en la medida de lo posible) - un host exacto, comparado sin distinguir
  mayúsculas (el comparador también entiende una forma `*.domain`, pero el
  instalador rechaza las entradas con comodín, así que ninguna llega nunca a él);
- restringe el esquema a `http`/`https` y el método a `GET`/`HEAD`;
- conecta a través del cliente seguro frente a SSRF (`internal/pkg/ssrf`), que
  rechaza las IP privadas/reservadas/de enlace local **después** de la resolución
  DNS, al conectar, de modo que el DNS rebinding y las redirecciones se revalidan;
- no adjunta ninguna credencial propia del servidor y limita el cuerpo de la
  respuesta.

**Endurecimiento por el operador (recomendado).** El intermediario traslada la
superficie SSRF al anfitrión, así que añada en el contenedor `evey-server` una
capa de salida independiente que no confíe en el código: descarte
`169.254.0.0/16` (metadatos de la nube / enlace local) y limite la subred
interna a los endpoints de Postgres + Meilisearch. Un fallo en la validación del
intermediario sigue chocando entonces con un muro de red; las dos capas fallan
de forma independiente.

**Lo que el intermediario no cubre.** Las formas de sortear la antigua lista
blanca DNS (IP literales, una extensión que trae su propio resolvedor, reutilizar
una conexión resuelta con trucos de SNI/Host) pertenecían a la salida directa y
han desaparecido con ella: el proceso de la extensión no puede abrir ningún
socket IP. Lo que queda es el límite que ya señala el modelo de seguridad de las
extensiones: el intermediario controla *adónde* va una llamada, no *qué* lleva,
así que los datos pueden salir todavía hacia un host de la lista blanca.

**Recomendaciones de mitigación**:
- Trate el permiso `network` como «auditar a mano»: permita solo hosts a los que
  confíe los datos de la extensión, y lea lo que la extensión les envía.
- Los operadores deberían vigilar el tráfico saliente de la máquina que ejecuta
  las extensiones (Prometheus + exportador de flujos, o simplemente
  `ss -tnp | grep <plugin-pid>`).

**Hoja de ruta V2** (después de la 1.0):
- Un cortafuegos de salida nftables a nivel de contenedor como capa
  independiente detrás del intermediario.
- Un proxy HTTP de salida opcional con inspección a nivel de petición.

## Exposición de red (fuera del modelo de amenazas de las extensiones)

Todo lo anterior trata de las extensiones. La otra mitad de la superficie de
ataque de este servidor es quién puede llegar a él, y la gobiernan dos
invariantes que conviene decir aquí porque quien busca «cómo expongo Evey sin
riesgo» empieza por este archivo:

- **Hay un solo punto de escucha pensado para publicarse, y no es el gRPC.** El
  puerto gRPC nativo (`EVEY_SERVER_PORT`, 9090 por defecto) es un transporte
  interno cuyo único cliente es la pasarela HTTP del mismo proceso; la API HTTP
  es ese puerto **más uno**. El punto de escucha gRPC se enlaza a loopback por
  defecto (`EVEY_GRPC_BIND=127.0.0.1`) precisamente porque el control de acceso
  remoto de abajo es un middleware HTTP y no lo cubre. Ampliar ese enlace es
  posible y deja un aviso al arrancar; cortarlo con un cortafuegos pasa a ser
  entonces tarea del operador.
- **El control de acceso remoto decide LAN o WAN a partir de la ÚNICA IP de
  cliente validada en la frontera del proxy de confianza, nunca a partir de una
  cabecera de la petición.** Un cliente no puede afirmar ser `127.0.0.1` con
  `X-Forwarded-For` para que lo traten como local, y una petición que nunca cruzó
  esa frontera se clasifica como WAN (rechazada) en lugar de recibir confianza.
  Tenga en cuenta que es un control de *despliegue* («esta instancia es para mi
  propia red»), no una capa de autenticación - lo que protege los datos es la
  autenticación.

Detalles y lista completa de variables: [configuration.md](configuration.md#remote-access-who-may-connect-from-outside-your-network)
y [deployment-bare-metal.md](deployment-bare-metal.md).

## Qué pueden contener los logs

El archivo de log del servidor es lo que sirven las rutas de logs de la
administración (seguimiento, búsqueda, flujo en directo), lo que acaba en los
informes de fallos y lo que lleva una copia de seguridad, así que las
credenciales se borran **al escribir** (`internal/pkg/logger/redact.go`): un
atributo cuya clave nombra un secreto (`authorization`, `token`, `access_token`,
`refresh_token`, `api_key`, `password`, `pin`, `secret`, ...) se sustituye
entero, y cualquier cadena - el mensaje incluido - que lleve un bearer, un JWT o
un par `secret=valor` se limpia. Las rutas de logs de la administración aplican
la misma regla a la salida para las líneas escritas antes de que existiera ese
borrado. Dos consecuencias para el código: un valor no secreto no debe
registrarse bajo un nombre de clave secreto (quedaría enmascarado - `token_info`
y `device_pin` son los nombres usados hoy), y un nuevo tipo de credencial debe
añadirse a esa lista antes de registrarse una sola vez.

## Informar de una vulnerabilidad

Informe en privado, nunca en una incidencia pública: `security@eveyproject.org`.
Es la dirección que da la [política de seguridad](../SECURITY.md) del
repositorio, y cubre la autenticación, los permisos y las fugas del entorno
aislado de las extensiones como cualquier otra cosa. No haga pruebas contra
instancias de Evey compartidas o públicas que no sean suyas.
