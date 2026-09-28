---
source_sha: 80123a8d7d69b50b13baa0a7c638767ebf54582aa06f7b85e176a6fee202d4ca
---
# Modelo de seguridad de las extensiones: qué puede declarar un autor

> **¿Qué página busca?** Esta es para los autores de extensiones: el catálogo de
> permisos, las reglas que debe cumplir una petición externa, las ranuras en las
> que puede montarse y qué ocurre en una actualización. Si administra un
> servidor y quiere saber cómo se contiene una extensión y qué no puede impedir
> todavía el entorno aislado, lea en su lugar
> [el entorno aislado y modelo de amenazas de extensiones](../SECURITY.md).

Las extensiones de Evey son código de terceros que se ejecuta como procesos del sistema operativo separados, junto al servidor. Este documento explica qué podría intentar una extensión maliciosa, cómo se defiende Evey de cada vía de ataque, y los límites honestos de la implementación actual.

## Modelo de amenazas: qué se confía a un manifiesto

| Amenaza | Descripción |
|---|---|
| RCE en el servidor | La extensión escapa de su entorno aislado y ejecuta órdenes arbitrarias como el proceso del servidor |
| Exfiltración de secretos | La extensión lee claves JWT, credenciales de la base de datos o tokens de usuario del sistema de archivos |
| Escalada horizontal | La extensión se hace pasar por otra por IPC para llamar a RPC para las que no tiene permiso |
| Exfiltración por red | La extensión abre conexiones salientes para enviar los datos capturados a un host del atacante |
| DoS / abuso de recursos | La extensión agota CPU, memoria, PID o disco (bombas fork, minado de criptomonedas, disco lleno) |
| Phishing por la interfaz | La extensión inyecta una capa a pantalla completa en el navegador del usuario para robar credenciales |
| Cadena de suministro | El autor distribuye código ofuscado o compilado que el análisis estático no puede inspeccionar |

## Capas de defensa

### 1. Validación del manifiesto (en la instalación)

Cada `plugin.yaml` se valida antes de la extracción. La instalación se rechaza si:

- `kind` falta o no es `plugin`/`theme`
- `kind: plugin` no tiene `language` o `entrypoint`
- `kind: theme` declara `language`, `entrypoint` o `permissions`
- `frontend[].slot` no está en la lista blanca de ranuras del servidor (vea la [lista blanca de ranuras](#slot-whitelist))
- las entradas de `external_fetch` incumplen las reglas de la lista blanca de hosts (vea las [reglas de peticiones externas](#external-fetch-rules))
- alguna capacidad o permiso declarado es desconocido

Fuente de verdad: `internal/domain/plugin/model.go`, `internal/adapters/driven/pluginrunner/capabilities.go`.

### 2. Extracción segura del ZIP

Antes de escribir ningún archivo en el disco:

- Límite de entradas: 5.000 archivos como máximo
- Límite por archivo: 100 MB
- Límite del tamaño total descomprimido: 500 MB
- Límite de la relación de compresión: 100:1 (detección de bombas zip)
- Control de salida de ruta: se rechazan las entradas con `..` en la ruta resuelta
- Enlaces simbólicos: rechazados sin condiciones (se comprueban los bits de tipo Unix de `ExternalAttrs`)
- Bits setuid/setgid/sticky: eliminados de todos los archivos extraídos

Fuente: `internal/adapters/driven/pluginrunner/zip.go`.

### 3. Analizador estático (en la instalación)

Cada archivo de texto del ZIP pasa por un analizador multilenguaje basado en expresiones regulares antes de que la extensión se acepte en la base de datos. Una pasada previa de normalización desactiva las ofuscaciones habituales (resolución de alias, plegado de concatenaciones, expansión de f-strings).

| Categoría | Gravedad | Ejemplos |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `shell_exec`/`proc_open`/`preg_replace /e`, Ruby comilla invertida/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING/CRITICAL | `requests.get`, `fetch(`, `net.Dial`, patrones de reverse/bind shell |
| `filesystem` | WARNING/CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, operaciones con enlaces simbólicos, `/proc/<pid>/environ` |
| `obfuscation` | CRITICAL | `eval(`, `base64.b64decode`, secuencias hexadecimales, cadenas de `String.fromCharCode`, cadenas de `chr()` en PHP, `str_rot13`, `assert(base64_decode(...))` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, `os.kill(1, ...)` |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module`, `getattr(__builtins__, ...)` |
| `sandbox_escape` | CRITICAL | `vm.runInNewContext`, `process.binding`, `new Function(`, Python `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL/WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` sin Loader |
| `native_code` | CRITICAL/WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` - opacos para el escáner |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` - pueden contener código sin analizar |
| `mime_mismatch` | WARNING (tratado como crítico) | la extensión del archivo dice imagen/fuente, el contenido es ejecutable |
| `suspicious_url` | WARNING | esquema no HTTP en un literal de URL (`ftp://`, `wss://`, `ssh://`) |
| `unvendored_deps` | WARNING | `requirements.txt`/`package.json` con dependencias declaradas pero sin módulos incluidos |

Los hallazgos CRITICAL o `mime_mismatch` fijan `Safe = false` y bloquean la instalación.

`compiled_binary` y `embedded_archive` bloquean la instalación salvo que el manifiesto declare el permiso `native-binary`. El administrador ve el aviso y el permiso en el paso de vista previa antes de aceptar.

Fuente: `internal/adapters/driven/pluginrunner/analyzer.go`, `analyzer_normalize.go`.

#### En la medida de lo posible, no una garantía

El analizador se basa en expresiones regulares y patrones. Desactiva clases conocidas de ofuscación pero **no puede garantizar que detecte todo intento deliberado de sortearlo**. Ejemplos de cobertura por clase:

| Clase | Detectado | Notas |
|---|---|---|
| Variable PHP invocable a partir de una concatenación | `$f = "sys" . "tem"; $f($cmd)` (una línea) y `$x="sys"; $y="tem"; $f=$x.$y; $f($cmd)` (varios pasos, resolución hasta punto fijo) | Hasta 5 iteraciones de punto fijo, los ciclos `$a=$b; $b=$a` terminan limpiamente |
| Builtin de Python buscado por nombre | `getattr(__builtins__, "exec")` y `__builtins__.__dict__["exec"]` (literal o mediante una variable resuelta) | El acceso con corchetes al dict dispara Critical cuando el literal es peligroso |
| Escape por `Function.constructor` en JS | `(fn).constructor("...")()` con llamada inmediata y `const F = (fn).constructor; F("...")` (referencia guardada, seguida reescribiendo la asignación) | Las llamadas con argumento variable `F(code)` se reescriben a `.constructor(code)` y las atrapa el patrón de argumento variable |
| Concatenación fragmentada en varios pasos | `$a="s"; $b="y"; $c="s"; $d="t"; $e="e"; $f="m"; $g=$a.$b.$c.$d.$e.$f; $g(...)` | Los fragmentos inocentes se siguen aparte de los valores «peligrosos»; la comprobación de peligro solo salta sobre la cadena final concatenada |

Cada clase de elusión tiene una prueba de regresión permanente en `tests/fixtures/plugins/malicious/`. Añadir una clase nueva exige: (1) fixture subido, (2) regla del analizador o pasada de normalización ampliada, (3) `installer_security_test.go` pasa `workerANeeded:true → false` cuando la detección funciona.

**Fuera de alcance (lo impone el entorno aislado):** transformaciones dinámicas a nivel de AST - `strrev("metsys")`, `base64.b64decode(...)`, `eval(open("/encoded").read())`, regex.replace en tiempo de ejecución. El analizador por expresiones regulares no puede resolverlas; el entorno aislado en ejecución (la lista blanca de Landlock o la lista blanca de montajes de bwrap, más la lista negra seccomp del núcleo) + el monitor de recursos en espacio de usuario + las credenciales del par por IPC garantizan que la extensión no puede escapar, exfiltrar `/data` ni escalar más allá de los permisos declarados.

**Falsos positivos conocidos:** el analizador encuentra coincidencias dentro de comentarios y docstrings. Una línea `# pip install foo` en un comentario dispara la regla `runtime_install`. Aceptable: pedir a los operadores que quiten esos comentarios antes de publicar es más barato que quitar comentarios por lenguaje en el analizador; la lista blanca de montajes de bwrap + el monitor de recursos en espacio de usuario hacen que el bloqueo en ejecución sea independiente.

#### Confirmación empírica en ejecución

La lista blanca de montajes se verificó de extremo a extremo en una pila en marcha mediante una extensión canario (`tests/fixtures/plugins/canary-sandbox-runtime/`) que sondea su vista del entorno aislado al arrancar:

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```

`/data` aparece no porque se monte el `/data` del host (nunca se monta) sino porque bubblewrap sintetiza los directorios padre vacíos del destino de escritura propio de la extensión (`/data/plugins/<id>`); por eso solo contiene ese subdirectorio. No se ve ningún `jwt.key`, `backups` ni `artworks`. `/proc/1/environ` SÍ es legible, pero el PID 1 dentro del espacio de nombres es la propia extensión (`--unshare-pid`), así que leer su propio entorno no es una fuga.

Vuelva a ejecutar el canario tras cualquier cambio en `runner_bwrap.go` para confirmar que la disposición de montajes sigue bloqueando.

**La frontera de aplicación es el entorno aislado en ejecución (Landlock o bubblewrap) + seccomp del núcleo + monitor de recursos en espacio de usuario + credenciales del par por IPC, no el analizador.** Una extensión que cuele un patrón CRITICAL por delante del analizador sigue sin poder leer `/data` (Landlock lo deniega / bwrap nunca lo monta), hacer una llamada al sistema denegada, lanzar una bomba fork por encima del límite de PID o invocar métodos gRPC para los que no tiene permiso. El analizador es una capa de la defensa en profundidad, útil para sacar a la luz código obviamente malicioso en la vista previa y reducir la superficie de ataque en ejecución - no el perímetro.

### 4. Verificación de la firma cosign

Configurable con la clave `cosign_policy` de `evey.yaml`:

| Política | Comportamiento |
|---|---|
| `off` | No se comprueba la firma; `Plugin.Signature` es nil |
| `warn` (por defecto) | El fallo de verificación se registra y se muestra al administrador, pero no bloquea la instalación; sin clave pública configurada, la verificación no hace nada |
| `enforce` | El fallo de verificación bloquea la instalación con un error |

Fuente: `internal/adapters/driven/pluginrunner/cosign.go`.

### 5. Saneador de CSS

Se aplica a `theme.css` en la instalación. El conjunto completo de reglas está en [`docs/themes/security-model.md`](../themes/security-model.md). El saneado elimina o reescribe las construcciones peligrosas en lugar de rechazar todo el archivo (los avisos se registran).

### 6. Escáner de JS de temas

Se aplica a `theme.js` en la instalación. Un patrón que coincide hace que se rechace la instalación. La lista de patrones bloqueados está en [`docs/themes/security-model.md`](../themes/security-model.md).

### 7. Entorno aislado en ejecución (Landlock / bubblewrap + seccomp + monitor de recursos en espacio de usuario)

> **El valor por defecto es `landlock`** (`sandbox_mode=auto` se resuelve a él en
> la imagen Docker oficial): una lista blanca del sistema de archivos con Landlock
> LSM + `-restrict-net` (ninguna salida IP directa para ninguna extensión) + la
> lista negra seccomp del núcleo y de `LD_PRELOAD` + `-deny-inet`, todo bajo el
> seccomp+AppArmor por defecto de Docker sin ningún cambio en el host. Vea
> [`docs/plugins.md`](../plugins.md) para el mapa y
> [`docs/SECURITY.md`](../SECURITY.md#sandbox-modes) para la tabla completa por modo.

Las capas de confinamiento de abajo se aplican según lo que permita el host. El
lanzador (y el seccomp del núcleo que instala) solo se usa en los modos
**`landlock`** y **`basic`**: `applySandbox` envía `bubblewrap`/`strict` por una
ruta de código bwrap aparte que nunca invoca al lanzador, así que esos dos modos
se apoyan en el aislamiento por espacios de nombres propio de bwrap (más el shim
LD_PRELOAD en `strict`).

- **Separación de UID** (`EVEY_PLUGINS_UID_SEPARATION`, por defecto `auto`) -
  cuando está activa, cada extensión pasa al uid 1001 (`evey-plugin`), distinto
  del uid 1000 del servidor, mediante `cap_setuid` en el **lanzador** (el
  servidor sigue sin privilegios). `auto` solo la activa en el modo `basic` (sin
  Landlock/bwrap), donde DAC es el único muro alrededor de los secretos; `on` la
  fuerza en todas partes; `off` la desactiva. Cuando está activa, `jwt.key`
  (`0600`) y `/data/{keys,logs,backups}` (solo para el propietario) son
  ilegibles por DAC para una extensión con uid 1001, incluso sin Landlock.
- **Lista negra seccomp del núcleo (solo `landlock`/`basic`)** - la instala
  `evey-plugin-launcher` según la arquitectura del build, antes de `execve`, así
  que restringe **también los binarios estáticos/compilados de las extensiones**
  (que nunca cargan `libevey-sandbox.so`). Bloquea `ptrace`, `mount`, `unshare`,
  `setns`, `bpf`, `kexec`, la carga de módulos, la familia setuid, `capset`, el
  keyring, las señales al PID 1, etc. Se hereda a través de `execve` en la
  extensión y sus hijos. **En `bubblewrap`/`strict` este filtro del lanzador no
  se aplica** - `strict` solo añade el shim `LD_PRELOAD` `libevey-sandbox.so`
  (la misma lista negra, más `execve`/`execveat`, para que una extensión
  interpretada sin `subprocess` no pueda lanzar hijos), que solo vale para
  extensiones dinámicas y no cubre los binarios estáticos.
- **`-deny-inet`** - para **cualquier** extensión, tenga permiso de red o no, el
  lanzador responde EPERM a `socket(AF_INET/AF_INET6)` en el núcleo: ninguna
  salida IP, incluso con una ABI de Landlock <4. `AF_UNIX` sigue permitido para
  el socket gRPC de HostService.
- **`-deny-fork`** - para toda extensión **sin** el permiso `subprocess`, la
  creación de procesos recibe EPERM en el núcleo (`fork`, `vfork`, `clone` sin
  `CLONE_THREAD`); la creación de hilos no se toca, así que los entornos de
  node, Python y Go no se ven afectados. `clone3` devuelve `ENOSYS`, no `EPERM`,
  para que glibc recurra al `clone(2)` inspeccionable en lugar de fallar al
  crear hilos.
- **`PR_SET_DUMPABLE=0` en el servidor** - el servidor se marca como no volcable
  al arrancar, así que una extensión con el mismo uid no puede leer
  `/proc/<server>/{mem,environ,maps}` (la clave JWT en memoria, `DB_PASSWORD`,
  `MEILI_MASTER_KEY`).
- **Monitor de recursos en espacio de usuario** - vea abajo; no se usan cgroups.

Lo que añade el **backend de sistema de archivos/red** depende del modo:

| Modo | Confinamiento del sistema de archivos | Red | Se elige cuando |
|---|---|---|---|
| `landlock` | Lista blanca Landlock LSM (directorios del sistema en solo lectura; directorio de la extensión + directorio del socket en escritura; `/data` y hermanos denegados) | `-restrict-net` (toda salida IP cortada, sin filtro por puerto/host) + `-deny-inet` | `sandbox_mode=landlock` (falla cerrado si no existe); `auto` en un núcleo con Landlock - el caso normal de la imagen oficial |
| `bubblewrap` | lista blanca de montajes en espacios de nombres separados (vea abajo) | `--unshare-net` siempre (toda extensión, sea cual sea el permiso `network`) | `sandbox_mode=bubblewrap`; `auto` donde falta Landlock pero la sonda de bwrap pasa (bare metal / con privilegios) |
| `strict` | igual que `bubblewrap` + seccomp por LD_PRELOAD | igual que `bubblewrap` | `sandbox_mode=strict` |
| `basic` | ninguno (solo DAC; último recurso de `auto`) | solo `-deny-inet` | `sandbox_mode=basic`; `auto` cuando no se puede usar ni Landlock ni bubblewrap |

`auto` se resuelve **landlock → bubblewrap → basic** una sola vez, en el primer
arranque de una extensión, y registra el modo elegido. `basic` **no** es «sin
aislamiento»: sigue aplicando separación de UID, el seccomp del núcleo +
LD_PRELOAD, `-deny-inet`, Landlock en la medida de lo posible y el monitor de
recursos - solo pierde el *confinamiento del sistema de archivos por ruta*
cuando el núcleo no tiene Landlock (una extensión puede entonces leer archivos
no sensibles legibles por todos, y el *listado* de procesos en `/proc` es
visible; `/proc/<server>/{mem,environ}` siguen protegidos).

Cuando `sandbox_mode` es `bubblewrap` o `strict` (por elección en `evey.yaml`), cada proceso de extensión se ejecuta dentro de un espacio de nombres de bubblewrap con:

**Aislamiento de espacios de nombres (5 separados):**
- `--unshare-user` - espacio de nombres de usuario propio, mapeo de UID
- `--unshare-pid` - espacio de nombres de PID aislado
- `--unshare-uts` - nombre de host aislado
- `--unshare-ipc` - IPC aislado
- `--unshare-cgroup` - espacio de nombres de cgroup aislado
- `--unshare-net` - espacio de nombres de red **siempre** separado, para toda extensión sea cual sea el permiso `network`; la salida directa pasa por la RPC `Fetch` del anfitrión

**Lista blanca de montajes:**

El entorno aislado monta una **lista blanca selectiva** de rutas del sistema en solo lectura necesarias en ejecución (intérpretes, bibliotecas compartidas, SSL, DNS) sin exponer nunca rutas de datos. **No** se monta `/` entero: solo los directorios de abajo, así que `/data`, `/var`, `/home`, `/root` y los directorios de extensiones vecinas simplemente no existen dentro del espacio de nombres.

| Ruta | Acceso | Motivo |
|---|---|---|
| `/usr`, `/bin`, `/sbin` | Solo lectura | Intérpretes y binarios del sistema |
| `/lib`, `/lib64` | Solo lectura (`--ro-bind-try`) | Bibliotecas compartidas (se saltan las rutas ausentes según la distribución) |
| `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | Solo lectura (`--ro-bind-try`) | Almacén de confianza TLS, resolución DNS, detección del entorno. **No** `/etc` entero: `/etc/passwd`, `/etc/shadow`, `/etc/ssh` nunca se montan. |
| `<plugin_dir>` | Lectura-escritura | Directorio de instalación propio de la extensión |
| `<plugin_sock_dir>` | Lectura-escritura | Directorio del socket IPC por extensión (vuelto a montar tras el tmpfs de `/tmp`) |
| `/proc` | Montaje proc | Necesario para los entornos de ejecución |
| `/dev` | Sintético (`--dev`) | Conjunto mínimo de dispositivos (null, zero, full, random, urandom, tty, pts) - ningún dispositivo físico |
| `/tmp` | tmpfs | Espacio temporal efímero; el `/tmp` del host no es visible |

Rutas **no** montadas: `/data`, `/var`, `/home`, `/root`, `/run`, `/etc/passwd`, `/etc/shadow`, `/etc/ssh`, los directorios de otras extensiones.

Fuente: `buildBwrapCmd` en `internal/adapters/driven/pluginrunner/runner_bwrap.go`.

**Monitor de recursos en espacio de usuario** (por extensión):

Los límites de recursos los aplica un barredor en espacio de usuario (no cgroups, inutilizables en un contenedor Docker sin privilegios). El monitor consulta `/proc/<pid>/{statm,status,stat}` cada ~2 s y mata la extensión si supera algún límite. `nice` se aplica al lanzarla para que una extensión nunca prive de CPU al servidor.

- Límite de memoria (RSS, 256 MiB por defecto) - mediante `/proc/<pid>/statm`
- Límite de PID/hilos (64 por defecto) - mediante `/proc/<pid>/status`
- Prioridad de CPU - mediante `nice` (10 por defecto)
- Límite de disco (directorio de instalación, 100 MiB por defecto) - mediante un `filepath.Walk` recursivo cada ~30 s

Vea [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md) para el motivo del abandono de los cgroups.

Nota: `--unshare-cgroup` en la lista de espacios de nombres de bwrap de abajo crea un _espacio de nombres_ de cgroup aislado (la extensión ve una jerarquía de cgroups nueva) pero **no** impone límites de recursos; esos vienen del monitor en espacio de usuario de arriba.

Fuente: `internal/adapters/driven/pluginrunner/runner_landlock.go`, `runner_bwrap.go`, `runner_resmon.go`; `cmd/evey-plugin-launcher/{main.go,seccomp_linux.go,landlock_linux.go,droppriv_linux.go}`; `cmd/evey/hardening_linux.go`; `sandbox/sandbox.c`.

### 8. (retirada) Lista blanca DNS

La lista blanca DNS de V1 por `/etc/hosts` descrita en versiones anteriores de
este documento se ha **eliminado**. `runner_dns.go` ya no existe;
`buildBwrapCmd` no monta `/etc/hosts` (vea el comentario en
`internal/adapters/driven/pluginrunner/runner_bwrap.go`: «no /etc/hosts
whitelist is bind-mounted here anymore»). La sustituyó un mecanismo más
estricto: la salida IP directa está cortada del todo para toda extensión
(`-restrict-net` + `-deny-inet` en `landlock`/`basic`, `--unshare-net` en
`bubblewrap`/`strict`), y las extensiones con permiso de red solo llegan a la
red a través del intermediario `Fetch` del anfitrión, que aplica la lista
blanca `external_fetch` y una conexión segura frente a SSRF del lado del
anfitrión (vea la [salida de red de las extensiones](../SECURITY.md#plugin-network-egress-brokered-since-2026-07-11) en `docs/SECURITY.md`).

### 9. Autenticación por credenciales del par IPC

El anfitrión expone un socket Unix por extensión en `/tmp/evey/plugins/<plugin_id>/host.sock`. Cada llamada gRPC entrante se autentica leyendo el PID del par mediante `getsockopt(SO_PEERCRED)` - un valor que proporciona el núcleo y que no se puede falsificar en espacio de usuario. El PID se compara con el valor registrado justo después de `cmd.Start`.

Efectos:
- Una extensión no puede hacerse pasar por otra: las cabeceras `x-plugin-id` se ignoran; la identidad la da el socket que escucha.
- Una extensión que lanza un hijo con fork/exec no puede enviar llamadas al anfitrión a través del hijo (otro PID → `Unauthenticated`).

Fuente: `internal/adapters/driving/pluginhost/`.

### 10. Aislamiento de la interfaz

Los módulos frontales de las extensiones se ejecutan dentro de un `<iframe>` con:
- `sandbox="allow-scripts"` - sin acceso al mismo origen, sin envío de formularios, sin ventanas emergentes
- etiqueta CSP `<meta>` dentro del `srcdoc`: `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src <external_fetch_hosts>; img-src data: blob:; font-src data:`
- origen de postMessage impuesto del lado del anfitrión
- Límites de ritmo del puente: `toast` 3 mensajes/segundo como máximo, 200 caracteres, sin HTML
- Lista blanca de rutas de `navigate`: se rechazan `/admin/*`, `/auth/*`, `/setup/*`, `/profile/security/*`
- `resize` limitado: 50-800 px
- Lista blanca de ranuras impuesta en el servidor (instalación) y en el cliente (renderizado)

## Catálogo de permisos

| Permiso | Qué concede |
|---|---|
| `network` | La extensión puede abrir conexiones de red salientes. **Debe** combinarse con una lista `external_fetch` no vacía - sin ella se rechaza la instalación. |
| `subprocess` | La extensión puede lanzar procesos hijos. Sin él se aplican dos capas: el shim seccomp por LD_PRELOAD bloquea `execve`/`execveat` para las extensiones interpretadas, y el `-deny-fork` del lanzador bloquea en el núcleo la propia creación de procesos (`fork`/`vfork`/`clone` sin `CLONE_THREAD`). Los hilos siguen funcionando en cualquier caso. (El seccomp del núcleo del lanzador no deniega `execve` - tiene que ejecutar la extensión -, pero todo hijo hereda el filtro del núcleo + el dominio Landlock.) |
| `native-binary` | La extensión puede incluir `.so`/`.dll`/`.exe`/`.wasm`/`.jar` o archivos comprimidos embebidos. El administrador ve y debe aceptar el código opaco en la vista previa de la instalación. |
| `filesystem` | Intención declarada de acceder al sistema de archivos más allá del directorio propio de la extensión. Se aplica en la auditoría; todavía no es una regla dura del entorno aislado (vea Límites conocidos). |
| `media.read` | La extensión puede llamar a las RPC `MediaRead`: leer metadatos de archivos multimedia y acceso a flujos. |
| `media.write` | La extensión puede llamar a las RPC `MediaWrite`: crear/actualizar registros multimedia. |
| `api.library` | La extensión puede llamar a las RPC de gestión de bibliotecas: listar/buscar bibliotecas, lanzar escaneos. |
| `api.user` | La extensión puede llamar a RPC del ámbito del usuario (leer su propio perfil, preferencias). |
| `api.metadata` | La extensión puede llamar a las RPC de metadatos: buscar y escribir datos de enriquecimiento. |
| `notification.sender` | La extensión puede publicar eventos de notificación (web push, correo, webhook). |
| `webhook.receiver` | La extensión puede registrarse como destino de webhook (recibe HTTP POST de servicios externos). |
| `hardware.gpu` | **El permiso de mayor riesgo - solo administradores, nivel de primera mano.** Concede el nodo de render DRM del host (`-render-dev`, el `renderD1xx` real, nunca un nodo card, nunca `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`): una superficie de ataque sobre un controlador del núcleo y una frontera de memoria de GPU COMPARTIDA con el transcodificador (posible fuga de memoria entre procesos en GPU AMD/Apple/Qualcomm; una GPU colgada puede bloquear la transcodificación). Sin este permiso, los nodos de dispositivo GPU quedan fuera por completo de la lista blanca de dispositivos de Landlock (denegados sea cual sea el DAC). Con él concedido, el lanzador añade una regla Landlock explícita `IOCTL_DEV` limitada a ese único archivo (ABI v5+); en ABI de Landlock más antiguas, Landlock no controla los ioctl en absoluto, así que el acceso de lectura+escritura por sí solo ya permite enviar órdenes a la GPU. El instalador muestra un aviso CRITICAL bien visible en la vista previa; concédalo solo a extensiones de confianza y de primera mano. |

## Reglas de peticiones externas

Se declaran en `external_fetch: [...]` en el manifiesto y las comprueba en la
instalación `validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`).
Todo o nada: una entrada inválida rechaza la instalación entera.

Una entrada es un **nombre de host sin más**. Un prefijo de esquema se tolera y
se descarta - solo se conserva el host.

| Entrada | Válida | Motivo |
|---|---|---|
| `api.tmdb.org` | Sí | Nombre de host exacto, la forma normal |
| `https://api.tmdb.org` | Sí | Esquema analizado y descartado, host conservado |
| `*.tmdb.org` | No | Se rechazan los comodines - liste cada subdominio que necesite |
| `*` | No | La misma regla |
| `192.168.1.1`, `::1` | No | Se rechazan los literales IP, de ambas familias |

En tiempo de ejecución, el intermediario del anfitrión añade sus propios límites,
que el manifiesto no puede ampliar: solo `http` y `https`, solo `GET` y `HEAD`,
el host de destino comparado sin distinguir mayúsculas con su lista, y las
direcciones privadas o reservadas rechazadas tras la resolución DNS - las
redirecciones se vuelven a comprobar contra ambas reglas.

> El comparador del intermediario también entiende una forma `*.example.com`,
> pero el instalador rechaza esa entrada, así que nunca llega a él. Escriba
> nombres de host explícitos.

## Lista blanca de ranuras

Las extensiones solo pueden inyectar interfaz en ranuras permitidas explícitamente. Las rutas críticas faltan a propósito.

| Ranura | Requiere sesión | Notas |
|---|---|---|
| `home.section` | Sí | Zona de contenido de la página principal |
| `home.hero` | Sí | Zona del banner principal |
| `library.sidebar` | Sí | Barra lateral de la vista de biblioteca |
| `media.detail.sidebar` | Sí | Barra lateral del detalle de un medio |
| `player.overlay` | Sí | Capa superpuesta del reproductor |
| `settings.user.appearance` | Sí | Sección Apariencia de los ajustes del usuario |
| `sidebar.bottom` | Sí | Zona inferior de la bandeja de navegación |
| `admin.nav` | Sí | Una entrada en el menú de administración que abre la página aislada propia de la extensión - no inyecta nada en las pantallas de administración existentes |

**Nunca permitido:** `admin.*` (salvo `admin.nav`), `login.*`, `setup.*`, `profile.security.*` - estas rutas son críticas para la autenticación y los privilegios, y una inyección de una extensión ahí sería una vía de escalada de privilegios o de phishing.

El contenido de `admin.nav` sigue aislado como cualquier otra ranura: se muestra dentro de un `<iframe sandbox="allow-scripts">` (sin `allow-same-origin`) con CSP `default-src 'none'`, así que es contenido de terceros mostrado en el área de administración, no una capacidad de administración concedida a la extensión.

Fuente: `internal/domain/plugin/slots.go`.

## Flujo de actualización

1. El administrador sube un ZIP nuevo → `PreviewPluginUpdate` ejecuta el analizador estático + la comprobación cosign y devuelve una comparación de los permisos/ranuras/`external_fetch` cambiados respecto a la versión instalada.
2. Si la comparación no está vacía (permisos nuevos, ranuras nuevas, hosts externos cambiados), el administrador debe confirmarlo explícitamente antes de aplicar la actualización (`requires_ack = true`).
3. El hash de la fuente se toma en la vista previa y se verifica al aplicar (anti-TOCTOU: el ZIP analizado es el ZIP que se instala).
4. Si la política cosign es `enforce`, una firma que no coincide en el paso de aplicación bloquea la actualización aunque la vista previa pasara.

## Límites conocidos del modelo de permisos

| Límite | Impacto | Mitigación actual |
|---|---|---|
| `auto` puede resolverse a `basic` en un host sin Landlock y sin bubblewrap utilizable | Sin confinamiento del sistema de archivos **por ruta**: una extensión puede leer archivos no sensibles legibles por todos, y el listado de procesos en `/proc` es visible | Los secretos del servidor siguen protegidos (separación de UID + `PR_SET_DUMPABLE=0`); seccomp del núcleo (incluidos binarios estáticos) + `-deny-inet` siguen aplicándose; el modo resuelto se registra en el primer arranque de una extensión |
| Todas las extensiones comparten el UID 1001 | Una extensión maliciosa podría enviar señales a sus hermanas (las señales al PID 1 / al servidor las bloquea seccomp) | `PR_SET_DUMPABLE=0` bloquea las lecturas entre procesos de `/proc/<pid>/{environ,mem,maps,fd}`; Landlock / bubblewrap aíslan por completo entre sí los directorios de las extensiones |
| El intermediario `Fetch` controla *adónde* puede ir una llamada saliente, no *qué* sale en ella | Una vez un host está en la lista blanca `external_fetch`, el intermediario no puede inspeccionar el contenido, así que la exfiltración hacia un host permitido sigue siendo posible | Los administradores solo deberían permitir hosts a los que confíen los datos de la extensión; el intermediario sigue bloqueando destinos no permitidos e IP privadas/reservadas |
| El shim seccomp por LD_PRELOAD solo restringe las extensiones dinámicas (interpretadas) | Un binario de extensión enlazado estáticamente en modo `basic` sin el lanzador presente se salta por completo la lista negra del shim | La imagen oficial incluye siempre el lanzador (el seccomp del núcleo cubre los binarios estáticos en `landlock`/`basic`); las imágenes de desarrollo sin el binario del lanzador no deberían ejecutar extensiones que no sean de confianza |
| El analizador por expresiones regulares se puede sortear con ofuscación avanzada | Un atacante motivado puede ocultar patrones | Vista previa del administrador + revisión de permisos; la frontera de aplicación es el entorno aislado + el monitor de recursos + las credenciales del par por IPC, no el analizador |

## Qué añadirá V2

- Un cortafuegos de salida nftables a nivel de contenedor como capa independiente detrás del intermediario `Fetch` (endurecimiento para el operador, no un hueco del entorno aislado - la salida IP directa ya está cortada por completo para toda extensión)
- Un proxy de salida para una aplicación completa a nivel de petición
- Un modo de lista blanca seccomp (entorno aislado estricto: solo se permite un conjunto mínimo de llamadas al sistema)
- Cobertura de las entradas con comodín de `external_fetch` mediante aplicación por petición
