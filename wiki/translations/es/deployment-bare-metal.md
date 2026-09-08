---
source_sha: 8e943dfd72345f44add4c70011cd163dc77c351f5f410f69d2165ba7c88ea11d
---

# Despliegue sin contenedor (sin Docker)

Cómo ejecutar Evey directamente sobre un host (con systemd, un script de inicio
o a mano) en lugar de usar la [imagen Docker](installation.md). La vía
recomendada es la imagen Docker: trae ya conectados el sandbox de extensiones,
el endurecimiento y los entornos de ejecución. Prescindir del contenedor
significa que es **usted** quien reproduce ese mismo nivel de protección: los
binarios adecuados uno junto a otro, un usuario dedicado que no sea root, el
mecanismo de sandbox que admita su núcleo y directorios de secretos accesibles
solo para su propietario. Esta página es el paso a paso para conseguirlo.

Si lo único que busca es la instalación fácil, deténgase aquí y siga
[installation.md](installation.md).

## 1. Requisitos previos

### Binarios (compilados desde el código fuente)

Compile los dos binarios Go y **colóquelos en el mismo directorio**. El ejecutor
de extensiones localiza el lanzador mirando primero en `$EVEY_PLUGIN_LAUNCHER`,
después junto a su propio ejecutable (`os.Executable()`) y por último en `$PATH`,
así que tenerlos uno al lado del otro es la disposición correcta más sencilla.

```bash
# from the repo root
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# install side by side, e.g.:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/
```

Orden de búsqueda en `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (debe ser un archivo ejecutable): anulación explícita.
2. `evey-plugin-launcher` en el **mismo directorio** que el binario `evey` en
   ejecución.
3. `evey-plugin-launcher` en algún punto de `$PATH`.

### El shim seccomp LD_PRELOAD

Compile el shim `libevey-sandbox.so` (un archivo C pequeño; hace falta un
compilador de C):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

El ejecutor lo busca en la **ruta fija `/app/libevey-sandbox.so`**
(`sandboxLibPath`, en `runner.go`). Si no está, las extensiones **se niegan a
arrancar** (fail-closed), salvo si trabaja en modo de desarrollo. Este shim añade
una lista de denegación seccomp para las extensiones enlazadas dinámicamente y se
superpone a Landlock o a bubblewrap.

> La imagen Docker guarda bajo `/app` los binarios de código abierto, el SDK y
> este shim (allí no hay ningún secreto). Sin contenedor puede reproducir esa
> misma ruta (`/app/libevey-sandbox.so`) o crear un enlace simbólico.

### Entornos de ejecución

Instale aquello que invoque el `entrypoint:` de sus extensiones. El ejecutor
reconoce estos intérpretes (`knownInterpreters`, en `runner.go`): `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Como mínimo:

- **`python3`**: la mayoría de las extensiones de la comunidad.
- **`node`**, **`php`**, **`ruby`**, **`lua`**: solo si sus extensiones los usan.

A eso se suman las dependencias básicas de ejecución:

- **ffmpeg** (transcodificación; la imagen incluye además los controladores VAAPI
  y `chromaprint` para la detección de intros).
- **PostgreSQL** (el servidor y una base de datos).
- **Meilisearch** (índice de búsqueda; o ejecútelo integrado con el binario
  oficial).
- **`bwrap`** (bubblewrap): solo si piensa usar bubblewrap como mecanismo de
  sandbox (véase más abajo).
- **`libcap`** (`setcap`): para conceder la capacidad de archivo que se describe
  en §4.

## 2. Ejecutar con un usuario dedicado que no sea root

**No ejecute nunca el servidor como root.** Cree dos usuarios sin privilegios que
coincidan con los UID que utiliza el anfitrión de las extensiones
(`pluginUID = 1001` y `pluginGID = 1001`, en `pluginhost/host_server.go`):

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) ejecuta el servidor.
- `evey-plugin` (UID 1001) es el UID al que se rebajan las extensiones (lo hace
  el lanzador cuando la separación de UID está activa), de modo que una extensión
  comprometida acaba corriendo como un usuario distinto y sin privilegios,
  incapaz de leer los secretos del servidor por los simples permisos del sistema
  de archivos. Véase §4.

Al arrancar, el servidor se marca a sí mismo como **no volcable**
(`PR_SET_DUMPABLE=0`, `cmd/evey/hardening_linux.go`) para que una extensión que
conviva en la misma máquina no pueda rebuscar en
`/proc/<server-pid>/{mem,environ}`.

## 3. Elegir el mecanismo de sandbox que admita su núcleo

Defina `EVEY_PLUGINS_SANDBOX_MODE`. El valor por defecto, `auto`, recorre
**landlock → bubblewrap → basic** y se queda con el más sólido que admita el host
(`resolveAutoMode()`, en `runner_landlock.go`). Elija uno de forma explícita si
prefiere un comportamiento fail-closed.

### Landlock (recomendado)

Una lista blanca del sistema de archivos ruta por ruta (las extensiones solo ven
su propio directorio y el del socket, nunca sus datos ni sus secretos), más un
filtro de salida TCP de Landlock v4. **No necesita privilegios, ni montajes, ni
espacios de nombres**: el lanzador se lo aplica a sí mismo y a continuación
ejecuta la extensión con `execve`.

**Requisito del núcleo**: Landlock tiene que estar compilado en el núcleo *y*
figurar además en la lista de LSM activos. La función `landlockAvailable()` del
ejecutor (`landlock_probe_linux.go`) sondea `landlock_create_ruleset(VERSION)` y
devuelve un errno cuando la llamada al sistema no existe (núcleo `<5.13`) o
cuando Landlock no está en la lista de LSM activos. Compruébelo así:

```bash
uname -r                              # se necesita 5.13 o superior
cat /sys/kernel/security/lsm          # debe incluir "landlock"
```

Si `landlock` no aparece en esa lista, actívelo en la línea de órdenes del núcleo
y reinicie (la lista exacta varía según la distribución: conserve las entradas
que ya haya y añada `landlock` al final):

```
# por ejemplo, en GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Después:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # explícito y fail-closed (rechaza una extensión si Landlock no puede aplicarse)
# o déjelo en el valor por defecto:
EVEY_PLUGINS_SANDBOX_MODE=auto        # se resuelve como landlock cuando está disponible
```

El modo `landlock` pasa `-require-landlock` al lanzador, que prefiere salir con
un código distinto de cero antes que ejecutar una extensión sin confinar si
Landlock no se aplica.

### bubblewrap

Si no consigue disponer de Landlock, instale el binario `bwrap`. bubblewrap
encierra cada extensión en espacios de nombres user, pid, uts, ipc y (net), con
una lista blanca de montajes. Necesita que los **espacios de nombres de usuario
sin privilegios estén activados** y que la **llamada al sistema `mount` esté
disponible**, algo cierto en casi cualquier Linux sin contenedor pero bloqueado
dentro de un contenedor Docker sin privilegios (justo por eso Landlock es el
valor por defecto en contenedor). `auto` solo elige bubblewrap cuando Landlock
**no está** y un sondeo desechable de bwrap se completa de verdad
(`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espacios de nombres de bwrap
# o bien:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap más el shim seccomp LD_PRELOAD
```

Compruebe que el host permite de verdad la combinación de espacios de nombres y
montajes:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (último recurso)

`EVEY_PLUGINS_SANDBOX_MODE=basic`: rebaja de UID más el shim seccomp LD_PRELOAD;
el lanzador sigue instalando la lista de denegación seccomp del núcleo y, si lo
encuentra, Landlock en la medida de lo posible. **No hay confinamiento del
sistema de archivos ruta por ruta**: la extensión se ejecuta con el UID 1001 y
nada más que seccomp. Reserve esta opción para núcleos antiguos o muy
restringidos donde no haya ni Landlock ni bubblewrap, y apóyese en la separación
de UID de §4 para proteger los secretos.

> **Una extensión a la que se ha concedido el permiso `subprocess` necesita el
> lanzador**, incluso en modo basic. El shim LD_PRELOAD no sobrevive al `exec` de
> un proceso hijo, así que no puede confinar a una extensión autorizada a crear
> subprocesos; solo lo consigue el seccomp del núcleo que instala el lanzador
> (justo antes del `exec`, y que se hereda a través de él). Si falta
> `evey-plugin-launcher` (véase §1), en producción una extensión así **se niega a
> arrancar** (fail-closed) en lugar de ejecutarse sin confinar. Las extensiones
> que no tienen ese permiso recurren al shim LD_PRELOAD, tal como se ha descrito
> más arriba.

## 4. Separación de UID (proteger los secretos sin root)

La separación de UID ejecuta cada extensión con un usuario propio (UID 1001,
`evey-plugin`), distinto del del servidor (UID 1000), para que una extensión no
pueda leer el `jwt.key`, las copias de seguridad ni los registros del servidor
por los simples permisos del sistema de archivos (DAC). Donde más cuenta es en el
modo **`basic`** (sin el confinamiento de rutas de Landlock); con Landlock o
bubblewrap el sistema de archivos ya está tapiado, así que allí resulta
redundante.

Se controla con `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, por
defecto `auto`):

- `auto`: se activa solo cuando el sandbox se resuelve como `basic`. En un núcleo
  moderno sin contenedor (con Landlock) queda desactivada, y así está pensado.
- `on`: siempre, incluso con Landlock o bubblewrap (una capa adicional e
  independiente).
- `off`: nunca.

**Quien hace la rebaja es el lanzador, no el servidor.** El servidor sigue sin
ser root y **sin** ninguna capacidad; el pequeño binario `evey-plugin-launcher`
lleva una única capacidad de archivo y se aplica `setuid` hasta 1001 justo antes
de ejecutar la extensión:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Así el servidor se mantiene por completo sin privilegios: una ejecución remota de
código en el servidor no puede llamar a `setuid(0)`, porque el servidor no posee
ninguna capacidad; la tiene únicamente el diminuto lanzador, que no recibe
entrada alguna (y que rechaza cualquier UID de destino por debajo de 1000).
Funciona **en la medida de lo posible**: si el lanzador no logra rebajar el UID
(falta la capacidad, o está activo `no_new_privs`), registra un aviso y ejecuta
la extensión con el UID del servidor en vez de fallar, de manera que esto nunca
impide el arranque de una extensión.

> **Incompatible con `no_new_privs`.** Si lanza el servidor con
> `no-new-privileges` (o con `NoNewPrivileges=yes` en systemd), la capacidad de
> archivo se ignora en el `exec` y la separación de UID degrada en silencio a un
> mismo UID (con un aviso). Hay que elegir entre una cosa y la otra.

Si en cambio el servidor se ejecuta **como root** (algo no recomendable), rebaja
las extensiones a 1001 por sí mismo y la capacidad del lanzador deja de hacer
falta.

Para ver el panorama escenario por escenario, consulte la guía en lenguaje llano:
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Permisos de los archivos con secretos

Reproduzca lo que hace la imagen Docker (`Dockerfile`): mantenga los directorios
que guardan secretos **accesibles solo para su propietario**, para que una
extensión rebajada al UID 1001 no pueda leerlos por simple DAC, ni siquiera sin
Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# El SERVIDOR es el propietario de plugins/ (ahí extrae las instalaciones); el
# grupo evey, al que pertenece evey-plugin, solo lee y ejecuta lo instalado:
sudo chmod 750 /var/lib/evey/plugins
# solo el propietario (ni grupo ni otros) en los directorios de secretos:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la propia clave de firma JWT:
sudo chmod 600 /var/lib/evey/jwt.key      # (se genera sola con 0600 en el primer arranque)
```

(Ajuste las rutas a los valores que haya dado a `EVEY_*_DIR` y
`EVEY_JWT_PRIVATE_KEY_PATH`.)

## 6. Entorno y configuración

Toda la configuración de ejecución pasa por variables de entorno `EVEY_*`. Defina
la base de datos, la búsqueda, las rutas de almacenamiento, la ruta de la clave
JWT y las variables del proxy inverso igual que en el despliegue con Docker: en
[configuration.md](configuration.md) está la tabla completa y en
[installation.md](installation.md), el recorrido por la base de datos, la
búsqueda y el proxy inverso. No hay que duplicar aquí esa tabla. Apuntes propios
de la instalación sin contenedor:

- Apunte `EVEY_DB_HOST` y `EVEY_MEILI_URL` a su PostgreSQL y su Meilisearch
  locales.
- El servidor sigue sirviendo **HTTP sin cifrar**: ponga delante un proxy inverso
  que se encargue de TLS (`EVEY_SERVER_BASE_URL` con esquema `https://` y
  `EVEY_TRUSTED_PROXIES`). Véase [reverse-proxy.md](reverse-proxy.md).
- **El servidor abre DOS puertos y solo uno de ellos puede quedar expuesto.**
  `EVEY_SERVER_PORT` (por defecto `9090`) es el escuchador gRPC nativo; la API
  HTTP con la que hablan sus clientes y su proxy inverso es **ese puerto más
  uno** (`9091`). Apunte el proxy inverso y el cortafuegos únicamente al puerto
  HTTP. El puerto gRPC es un transporte interno: el filtro de red de acceso
  remoto, que decide si los clientes de fuera de su LAN pueden llegar a
  conectarse, está integrado en la cadena HTTP y **no cubre el escuchador gRPC**.
  Por eso este último escucha por defecto **solo en loopback**
  (`EVEY_GRPC_BIND=127.0.0.1`), de modo que una ejecución sin contenedor no puede
  exponerlo por descuido. Si tiene motivos para publicarlo, como un cliente gRPC
  nativo en otra máquina, ponga `EVEY_GRPC_BIND=0.0.0.0`; el servidor avisa en el
  arranque de que el filtro no se le aplica, y protegerlo con el cortafuegos
  queda entonces de su cuenta.
- Las migraciones se ejecutan solas al arrancar; no hay ningún paso de migración
  aparte.
- Los límites de recursos de las extensiones (`EVEY_PLUGINS_MEMORY_MB`,
  `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) y el modo de sandbox se pueden
  modificar en caliente desde Admin → Extensiones; los valores del entorno son solo
  los predeterminados iniciales.

## 7. Verificar el sandbox

El servidor registra el mecanismo de sandbox que ha resuelto. En cuanto arranque
la primera extensión, consulte el registro del servidor:

- **resolución automática** (se registra una sola vez):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (o bien `bubblewrap` o `basic`).
- **por extensión** (en cada arranque de una extensión):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **repliegue a basic** (un aviso que *no* quiere ver si esperaba Landlock):
  `plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available …`, lo que significa que no se ha encontrado ni
  Landlock ni un bwrap operativo. Repase §3.

Confirme por su cuenta las piezas del núcleo:

```bash
cat /sys/kernel/security/lsm          # ¿Landlock activo? (debería aparecer "landlock")
grep Seccomp: /proc/<plugin-pid>/status   # 2 = modo filtro activo en una extensión en marcha
```

Una línea `Seccomp: 2` en un proceso de extensión vivo confirma que el filtro
seccomp del núcleo está instalado. Si ha fijado
`EVEY_PLUGINS_SANDBOX_MODE=landlock` y Landlock no se aplica, la extensión **se
niega a arrancar**: es el contrato fail-closed haciendo justo lo que debe.

---

Para todo lo demás (copias de seguridad, actualizaciones, CLI de recuperación,
resolución de problemas) sirven igual de bien las secciones orientadas a Docker
de [installation.md](installation.md): los subcomandos `evey admin` y la mecánica
de las copias son los del mismo binario.
