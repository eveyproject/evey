---
source_sha: 0ac15ab64c61bb43ae41b6ec652dda6261a8eed8431ec891b4f9ad3b33dd7d21
reviewed: false
---
# Sandbox de plugins y modelo de amenazas

Esta página describe lo que hace el servidor para contener un plugin: las
comprobaciones al instalar, las capas en ejecución, los modos de sandbox y lo que
cada uno todavía no puede impedir. Si estás escribiendo un plugin y quieres saber
qué puedes declarar y qué se rechazará, lee [Permisos y slots](plugins/security-model.md).

## Modelo de amenazas

Los plugins se ejecutan como procesos aparte que hablan con el servidor por gRPC
sobre sockets Unix. El servidor no confía por defecto en ningún código de plugin.
Los riesgos de los que se defiende, del más grave al menos grave:

1. **Suplantación**: un plugin se hace pasar por otro para usar sus permisos.
2. **Escalada de privilegios**: un plugin escapa de su sandbox y ejecuta órdenes
   como el servidor.
3. **Acceso al sistema de archivos**: un plugin lee o escribe fuera de su propio
   directorio, en particular los secretos del servidor.
4. **Exfiltración por la red**: un plugin envía datos a un host de su elección.
5. **Abuso de recursos**: minería, bombas fork, agotamiento de la memoria o del
   disco.
6. **Cadena de suministro**: código ofuscado o compilado que el análisis no puede
   leer.

## Al instalar

### Extracción del ZIP

Antes de escribir ningún archivo:

- como máximo 5.000 entradas, 100 MB por archivo, 500 MB extraídos en total, y
  un ratio de compresión de 100:1 a partir de 1 MB extraído (bombas de
  descompresión);
- las entradas cuya ruta sale del archivo (`..`) se rechazan;
- los enlaces simbólicos se rechazan;
- se eliminan los bits setuid, setgid y sticky.

Fuente: `internal/adapters/driven/pluginrunner/zip.go`.

### Manifiesto y permisos

El manifiesto declara los permisos que pide un plugin. Un permiso desconocido
rechaza la instalación. Después se aplican:

- **`subprocess`** permite los procesos hijos. Sin él, el kernel rechaza la
  creación de procesos (`-deny-fork`), y el filtro `LD_PRELOAD` rechaza además
  `execve` para los plugins interpretados.
- **`native-binary`** permite incluir código compilado o archivos comprimidos,
  que el análisis no puede leer.
- **Los permisos de API** (`api.library`, `api.user`, `api.metadata`,
  `media.read`, `media.write`, `notification.sender`...) los comprueba en cada
  llamada al anfitrión
  [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

La lista completa de permisos está en [Permisos y slots](plugins/security-model.md#permissions-catalog).

### Análisis estático

Cada archivo del paquete lo revisa
[`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go),
tras una pasada de normalización que deshace la ofuscación habitual (alias,
concatenación de cadenas, f-strings):

| Categoría | Gravedad | Ejemplos |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `system()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, comillas invertidas de Ruby, `open('\|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, shells inversas y de escucha |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, operaciones con enlaces simbólicos |
| `obfuscation` | CRITICAL | `eval`, decodificación base64, secuencias hexadecimales, `__builtins__`, `base64 -d \| bash` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, enviar señales al PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` sin Loader |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: `unsafe` de Go |
| `compiled_binary` | WARNING, luego BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` |
| `embedded_archive` | WARNING, luego BLOCK | `.zip`, `.tar.gz` |
| `mime_mismatch` | tratado como CRITICAL | una extensión que dice ser una imagen, con otro contenido |
| `suspicious_url` | WARNING | un esquema no HTTP en una URL literal |
| `unvendored_deps` | WARNING | `requirements.txt` o `package.json` sin los módulos incluidos |

Un hallazgo CRITICAL o un `mime_mismatch` rechaza la instalación.
`compiled_binary` y `embedded_archive` la rechazan salvo que el manifiesto
declare `native-binary`.

**Lo que el análisis no puede garantizar.** Funciona con patrones. Resuelve
clases de ofuscación conocidas (concatenación de cadenas en varios pasos hasta
obtener algo invocable, búsqueda de funciones integradas por nombre, escapes por
`Function.constructor`), cada una con un caso de regresión en
`tests/fixtures/plugins/malicious/`, pero no puede seguir todas las
transformaciones hechas en ejecución (`strrev("metsys")`, decodificar una carga
leída de un archivo). También detecta dentro de los comentarios: un comentario
`# pip install foo` activa `runtime_install`. **La frontera que se impone es el
sandbox en ejecución, no el análisis**: un plugin que cuela un patrón sin que se
detecte sigue sin poder leer los datos del servidor, hacer una llamada al sistema
denegada, superar sus límites de recursos ni llamar a un método del anfitrión
para el que no tiene permiso.

### Firma

Un paquete se puede firmar con cosign. `EVEY_PLUGINS_COSIGN_POLICY` fija qué hace
una verificación fallida:

| Política | Efecto |
|---|---|
| `off` | La firma no se comprueba. |
| `warn` (por defecto) | Un fallo se registra y se muestra al administrador, sin bloquear. Sin clave pública configurada, no se verifica nada. |
| `enforce` | Un fallo rechaza la instalación o la actualización. |

La clave pública la da `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. Fuente:
`internal/adapters/driven/pluginrunner/cosign.go`.

### Vista previa del administrador

`POST /api/v1/admin/plugins/preview` ejecuta el análisis sin instalar. El
administrador ve los permisos pedidos, cada hallazgo con su archivo, línea y
categoría, las URL encontradas en el código y el SHA256 del paquete; el mismo
hash se comprueba al aplicar la instalación, así que el paquete analizado es el
paquete instalado. Una actualización muestra la diferencia de permisos, slots y
hosts, y un permiso nuevo exige una aceptación explícita.

El administrador es la frontera de confianza para los permisos elevados: Evey
muestra la superficie, el administrador acepta o rechaza.

## En ejecución

### Sistema de archivos

Un plugin ve su propio directorio y su directorio de sockets, y no los datos del
servidor. Cómo se impone depende del [modo de sandbox](#sandbox-modes): una lista
de permitidos de Landlock, o una lista blanca de montajes de bubblewrap. En modo
`basic` en un kernel sin Landlock, no hay confinamiento por ruta: el usuario
aparte y los permisos de los archivos son entonces lo que protege los secretos
del servidor (abajo).

### Red

Un plugin **no tiene acceso directo a la red**, declare lo que declare: se
rechaza cualquier socket IP (`-deny-inet` en `landlock` y `basic`, un espacio de
nombres de red en `bubblewrap` y `strict`). Un plugin con el permiso `network`
llama al método `Fetch` del anfitrión por su socket Unix. El anfitrión, en
`internal/adapters/driving/pluginhost/fetch.go`:

- comprueba el destino con la lista `external_fetch` del plugin (hosts exactos,
  sin distinguir mayúsculas);
- solo permite `http` y `https`, y solo `GET` y `HEAD`;
- conecta mediante un cliente protegido contra SSRF (`internal/pkg/ssrf`) que
  rechaza las direcciones privadas, reservadas y de enlace local tras la
  resolución DNS, de modo que el DNS rebinding y las redirecciones se vuelven a
  comprobar;
- no añade ninguna credencial del servidor y limita el tamaño de la respuesta.

Esto cierra las vías que dejaba abiertas la salida directa (IP literales como
`169.254.169.254`, el resolvedor propio de un plugin). Lo que queda: el
intermediario controla *adónde* va una llamada, no *qué* lleva. Consulta
[Límites conocidos](#known-limits).

**Recomendado para quien administra el servidor.** El intermediario traslada la
superficie SSRF al anfitrión. Una regla de salida en el contenedor
`evey-server`, independiente del código, añade un segundo muro: descarta
`169.254.0.0/16` y limita la red interna a los endpoints de la base de datos y de
la búsqueda.

### Llamadas al sistema

Dos filtros seccomp rechazan las mismas llamadas al sistema peligrosas:

1. **El filtro del kernel**, instalado por
   [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) justo antes de lanzar el
   plugin ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)).
   Lo heredan el plugin y sus hijos y **cubre también los binarios compilados
   estáticos**. Una arquitectura que no corresponde en una llamada al sistema
   mata el proceso. Se aplica en `landlock` y `basic`.
2. **El filtro `LD_PRELOAD`**, [`sandbox/sandbox.c`](../sandbox/sandbox.c),
   cargado en los plugins interpretados que no declaran `subprocess`. Rechaza
   además `execve`/`execveat` y marca el plugin como no volcable. Se aplica en
   `landlock`, `basic` y `strict`. Si falta la biblioteca, se niega el arranque a
   un plugin así (`plugin.error`).

Llamadas al sistema rechazadas:

| Categoría | Llamadas al sistema |
|---|---|
| Depuración | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Señales | `kill`; el filtro del kernel rechaza también `tgkill`, el filtro `LD_PRELOAD` rechaza `tkill`/`tgkill` dirigidos al PID 1 |
| Montajes | `mount`, `umount2`, `pivot_root`, `chroot` |
| Espacios de nombres | `unshare`, `setns` |
| Módulos del kernel | `init_module`, `finit_module`, `delete_module` |
| BPF, perf | `bpf`, `perf_event_open` |
| E/S de hardware | `ioperm`, `iopl` (solo x86-64, donde existen) |
| Swap | `swapon`, `swapoff` |
| Control del sistema | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`, `kexec_file_load` |
| Primitivas de explotación del kernel | `io_uring_setup`, `io_uring_enter`, `io_uring_register`, `userfaultfd` |
| Identidad | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid`, `capset` |
| Llavero | `add_key`, `request_key`, `keyctl` |

Para un plugin sin `subprocess`, el kernel rechaza además la creación de procesos
(`fork`, `vfork`, `clone` sin `CLONE_THREAD`); los hilos siguen funcionando.

Es una **lista de denegación**: `open`, `read`, `write`, `mmap`, `socket` Unix y
`connect` siguen permitidos. Las rutas las confina la capa del sistema de
archivos, no seccomp. En `bubblewrap` no se usa el lanzador: un binario estático
no tiene ahí ningún filtro seccomp, y lo contienen los espacios de nombres y la
lista blanca de montajes.

### Un usuario aparte

Un plugin puede ejecutarse como el usuario 1001 (`evey-plugin`), distinto del
servidor (1000), para que los secretos del servidor le resulten ilegibles solo por
los permisos de los archivos. Lo controla `EVEY_PLUGINS_UID_SEPARATION`:

- `auto` (por defecto): solo cuando el sandbox resuelve a `basic`, el único caso
  en que los permisos de los archivos son el único muro alrededor de los
  secretos;
- `on`: siempre, como capa adicional;
- `off`: nunca.

El cambio lo hace el lanzador, que tiene `cap_setuid`; el propio servidor se
ejecuta sin privilegios ni capacidades. El lanzador rechaza cualquier destino por
debajo de 1000. Si no puede cambiar, un plugin en `basic` se rechaza (ahí el
usuario es el único muro), mientras que en `landlock` se ejecuta con el usuario
del servidor y un aviso. En `bubblewrap` y `strict`, el espacio de nombres de
usuario cumple esta función.

Cuando está activo, la clave de firma (`0600`) y las copias de seguridad
(directorio `0700`, archivos `0600`) solo son accesibles para su propietario en
ejecución, lo que se mantiene incluso con un `/data` montado desde el equipo.
Todos los plugins comparten el usuario 1001: los aísla entre sí la capa del
sistema de archivos, no el usuario.

### Autenticación del socket del anfitrión

Cada plugin tiene su propio socket, `/tmp/evey/plugins/<plugin_id>/host.sock`,
servido por su propio servidor gRPC. En cada llamada, el servidor lee el PID de
quien llama a través del kernel (`SO_PEERCRED`, que el espacio de usuario no
puede falsificar) y lo compara con el PID registrado al arrancar el plugin. La
identidad del plugin viene del socket, nunca de una cabecera. Así, un plugin no
puede suplantar a otro, llamar al socket de otro plugin ni encaminar llamadas a
través de un proceso hijo.

### Límites de recursos

Un monitor en espacio de usuario (los cgroups no están disponibles en un
contenedor sin privilegios) lee `/proc/<pid>` más o menos cada 2 segundos y
detiene un plugin que supera un límite:

- memoria (RSS), 256 MiB por defecto;
- procesos e hilos, 64 por defecto;
- disco usado por su directorio, 100 MiB por defecto, comprobado cada 30
  segundos;
- prioridad de CPU, `nice` 10 por defecto, aplicada al arrancar.

Los límites se fijan por plugin en **Admin → Plugins**. Consulta
[ADR-0013](adr/0013-userspace-plugin-resource-monitor.md).

### Aislamiento de la interfaz

La interfaz de un plugin se ejecuta en un `<iframe sandbox="allow-scripts">`, sin
acceso de mismo origen, formularios ni ventanas emergentes, bajo una
Content-Security-Policy que solo permite conexiones a sus hosts de
`external_fetch`. El puente limita lo que puede pedir a la página: `toast` como
máximo 3 por segundo, 200 caracteres, sin HTML; `navigate` solo acepta `/home`,
`/library`, `/media`, `/player`, `/settings` y `/search`; `resize` entre 50 y
800 px. Los slots se comprueban al instalar y de nuevo al dibujar.

### Endurecimiento del servidor

Al arrancar, el servidor se marca como no volcable (`PR_SET_DUMPABLE=0`,
[`hardening_linux.go`](../cmd/evey/hardening_linux.go)): un proceso que se
ejecuta con el mismo usuario no puede leer su memoria ni su entorno a través de
`/proc`, lo que protege la clave de firma en memoria y las contraseñas de la base
de datos y de la búsqueda, incluso en un kernel sin Landlock.

### Notificación de fallos

El ejecutor publica un evento `plugin.error` cuando falta la biblioteca del
sandbox para un plugin que la necesita, o cuando un plugin termina con un error.
El servicio de notificaciones lo transmite al administrador por los canales
configurados. Un PID que no corresponde en un socket del anfitrión se registra
como aviso.

## Modos de sandbox

El modo elige cómo se confinan el sistema de archivos y la red.
`EVEY_PLUGINS_SANDBOX_MODE` fija el valor inicial (por defecto `auto`); se puede
cambiar en **Admin → Plugins**. El ejecutor lo aplica en
[`applySandbox`](../internal/adapters/driven/pluginrunner/runner_sandbox.go). En
todos los modos: sin salida IP directa, el monitor de recursos y la autenticación
del socket del anfitrión.

### `auto` (por defecto)

Resuelve, una vez en el primer arranque de un plugin, al modo más fuerte que
admite el equipo: **landlock**, después **bubblewrap**, después **basic**.
Landlock va primero porque no necesita privilegios y funciona con el perfil de
seguridad por defecto de Docker. bubblewrap solo se elige cuando Landlock no
existe y un sandbox de prueba se construye de verdad
([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go)), lo
que en la práctica significa sin contenedor o en un contenedor privilegiado. El
modo resuelto se registra y se muestra en la línea **Sandbox del host** de
**Admin → Plugins → Panel de seguridad**.

### `landlock`

- **Sistema de archivos:** una lista de permitidos de Landlock aplicada por el
  lanzador. Los directorios del sistema (`/usr`, `/lib`, `/lib64`, `/bin`,
  `/sbin`, `/etc`, `/app`) en solo lectura; de `/dev`, solo `null`, `zero`,
  `full`, `random` y `urandom`, para que los nodos de la tarjeta gráfica y el
  terminal del servidor queden fuera de alcance; el directorio del plugin y su
  directorio de sockets en lectura y escritura; todo lo demás, incluidos `/data` y
  los demás plugins, denegado. Un plugin al que se concede `hardware.gpu` recibe
  su nodo de render como única excepción.
- **Red:** `-restrict-net` y `-deny-inet`; la salida solo a través de `Fetch`.
- **Llamadas al sistema:** el filtro del kernel y el filtro `LD_PRELOAD`.
- **Requiere:** un kernel 5.13 o posterior con Landlock en la lista de LSM
  activos, y el lanzador (ambas cosas se cumplen con la imagen oficial en un
  equipo reciente). Un `landlock` explícito se niega a arrancar un plugin que no
  puede confinar.

### `bubblewrap`

- **Sistema de archivos y procesos:** seis espacios de nombres separados (`user`,
  `pid`, `uts`, `ipc`, `cgroup`, `net`) y una lista blanca de montajes; dentro no
  existe nada más.

  | Ruta | Acceso |
  |---|---|
  | `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64` | solo lectura |
  | `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | solo lectura (nunca `/etc` entero: ni `passwd`, ni `shadow`, ni `ssh`) |
  | el directorio del plugin, su directorio de sockets | lectura y escritura |
  | `/proc` | su propio montaje |
  | `/dev` | un conjunto sintético mínimo (null, zero, random, urandom, tty, pts) |
  | `/tmp` | un tmpfs vacío |

  `/data`, `/var`, `/home`, `/root`, `/run` y los directorios de los demás
  plugins no existen dentro. Un plugin canario
  (`tests/fixtures/plugins/canary-sandbox-runtime/`) comprueba esta disposición
  contra una pila en marcha. Fuente: `buildBwrapCmd` en
  `internal/adapters/driven/pluginrunner/runner_bwrap.go`.
- **Red:** el espacio de nombres de red, para todos los plugins.
- **Llamadas al sistema:** ningún filtro seccomp; los espacios de nombres aportan
  el aislamiento.
- **Requiere:** el binario `bwrap` y las llamadas al sistema `mount`/`unshare`.
  En Docker eso exige un perfil de seguridad relajado, así que en la práctica es
  un modo sin contenedor.

### `strict`

`bubblewrap` más el filtro `LD_PRELOAD` (solo plugins interpretados).

### `basic`

El último recurso de `auto`: el usuario aparte, el filtro del kernel, el filtro
`LD_PRELOAD`, `-deny-inet` y Landlock si el kernel lo tiene. En un kernel sin
Landlock no hay confinamiento por ruta: un plugin puede leer archivos no
sensibles legibles por todos y ver la lista de procesos. Los secretos del
servidor siguen protegidos por el usuario aparte y `PR_SET_DUMPABLE`, la red está
cortada y las llamadas al sistema peligrosas se rechazan, binarios compilados
incluidos.

### Resumen

| Capa | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|
| Sistema de archivos | Lista de permitidos de Landlock | ninguna sin Landlock (solo permisos de archivos) | lista blanca de montajes | lista blanca de montajes |
| Red | `-restrict-net` + `-deny-inet` | `-deny-inet` | espacio de nombres de red | espacio de nombres de red |
| Seccomp del kernel (también binarios estáticos) | sí | sí | no | no |
| Seccomp `LD_PRELOAD` (plugins interpretados sin `subprocess`) | sí | sí | no | sí |
| Usuario aparte | con `on` | sí (`auto`) | espacio de nombres de usuario | espacio de nombres de usuario |
| Límites de recursos | sí | sí | sí | sí |

| Despliegue | Lo que obtienes |
|---|---|
| Docker en un kernel reciente (`auto` da `landlock`) | Confinamiento por ruta, sin salida directa, llamadas al sistema peligrosas rechazadas incluidos los binarios estáticos. El caso normal. |
| Docker en un kernel antiguo sin Landlock, por ejemplo un NAS antiguo (`auto` da `basic`) | Secretos protegidos y red cortada, pero los archivos legibles por todos se ven. Consulta [Configurar el sandbox de plugins](plugins/sandbox-deployment.md). |
| Sin contenedor (`auto` da `bubblewrap`) | El aislamiento de sistema de archivos y de procesos más fuerte, sin filtro seccomp para los binarios estáticos. |

## Configuración

| Variable | Por defecto | Efecto |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` o `basic`. También editable en **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` u `off`. Incompatible con `no_new_privs` en el contenedor. |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` o `enforce`. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | ninguna | La clave con la que se comprueban las firmas. |

La imagen oficial incluye `evey-plugin-launcher` (Landlock y el filtro del
kernel) y `bubblewrap`. bubblewrap necesita además los espacios de nombres de
usuario sin privilegios en el equipo. Qué configuración conviene a cada máquina,
en palabras sencillas: [Configurar el sandbox de plugins](plugins/sandbox-deployment.md).

## Límites conocidos

| Límite | Impacto | Lo que lo limita hoy |
|---|---|---|
| Sin confinamiento por ruta en `basic` en un kernel sin Landlock | Un plugin puede leer archivos no sensibles legibles por todos y los directorios de otros plugins | Secretos protegidos por el usuario aparte y `PR_SET_DUMPABLE`; `auto` prefiere Landlock o bubblewrap |
| Todos los plugins comparten el usuario 1001 | Un plugin puede enviar señales a sus hermanos (enviarlas al servidor se rechaza) | `PR_SET_DUMPABLE` impide leer otros procesos por `/proc`; Landlock y bubblewrap aíslan los directorios de los plugins |
| Seccomp es una lista de denegación | Las llamadas de archivos y de sockets Unix siguen permitidas | Las rutas las confinan Landlock o bubblewrap; los sockets IP se rechazan |
| El intermediario traslada la superficie SSRF al anfitrión | Un fallo en la validación del intermediario podría llegar a una dirección interna | Lista de permitidos y cliente protegido contra SSRF en Go; una regla de salida en el contenedor como capa independiente |
| El intermediario controla adónde va una llamada, no qué lleva | Los datos pueden salir hacia un host de la lista del plugin | Permite solo hosts a los que confiarías esos datos |
| El análisis funciona con patrones | Una ofuscación deliberada puede ocultar código | Vista previa del administrador; la frontera es el sandbox, no el análisis |
| `LD_PRELOAD` solo cubre los plugins dinámicos e interpretados | Un binario estático en `basic` sin el lanzador se le escapa | La imagen oficial siempre incluye el lanzador, cuyo filtro del kernel cubre los binarios estáticos |

## Exposición de red (fuera del modelo de amenazas de los plugins)

Dos invariantes rigen quién puede llegar siquiera al servidor:

- **Solo el puerto HTTP está pensado para publicarse.** El puerto gRPC
  (`EVEY_SERVER_PORT`, 9090 por defecto) es un transporte interno cuyo único
  cliente es la pasarela HTTP del mismo proceso; la API HTTP es ese puerto más
  uno. gRPC se enlaza al loopback por defecto (`EVEY_GRPC_BIND=127.0.0.1`) porque
  el bloqueo del acceso remoto es un middleware HTTP y no lo cubre. Ampliar el
  enlace registra un aviso; protegerlo con el cortafuegos pasa entonces a ser
  cosa de quien administra el servidor.
- **El bloqueo del acceso remoto decide a partir de la única dirección de cliente
  validada en la frontera de los proxies de confianza**, nunca a partir de una
  cabecera de la petición. Un cliente no puede afirmar ser `127.0.0.1` mediante
  `X-Forwarded-For`, y una petición que nunca cruzó esa frontera cuenta como
  remota. Es un control de despliegue, no una autenticación.

Consulta [Acceso remoto y HTTPS](administration/remote-access.md) y la
[Referencia de configuración](environment/server.md#remote-access).

## Lo que pueden contener los registros

El registro del servidor alimenta **Logs y depuración**, los informes de errores y
las copias de seguridad, así que las credenciales se borran al escribir una línea
(`internal/pkg/logger/redact.go`): un atributo cuya clave nombra un secreto
(`authorization`, `token`, `api_key`, `password`, `pin`, `secret`...) se sustituye
entero, y cualquier cadena que lleve un token bearer, un JWT o un par
`secret=value` se limpia. Las rutas de registros aplican la misma regla a las
líneas antiguas al salir. Para el código: nunca registres un valor no secreto
bajo un nombre de clave secreto (quedará enmascarado), y añade cualquier nuevo
tipo de credencial a esa lista antes de que llegue a registrarse.

## Informar de una vulnerabilidad

Informa en privado, nunca en una incidencia pública: `security@eveyproject.org`,
la dirección de la [política de seguridad](../SECURITY.md) del repositorio. Cubre
la autenticación de los plugins, los permisos y los escapes del sandbox como
cualquier otra cosa. No hagas pruebas contra instancias de Evey que no sean tuyas.
