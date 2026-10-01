---
source_sha: f4ecae392bc0b50e2c1ef8c70f50db7121732b5c6e7c194891cf416bffb92680
reviewed: false
---
# Permisos y slots

Esta página es para autores de plugins: lo que puede declarar un manifiesto, lo
que se comprueba al instalar un plugin y las reglas que se le aplican en
ejecución. Cómo contiene el servidor a un plugin, y lo que esa contención no
puede impedir, se describe en [Modelo de seguridad y sandbox de plugins](../SECURITY.md).

## Qué se comprueba al instalar

Un plugin se rechaza al instalar cuando:

- falta `kind`, o no es ni `plugin` ni `theme`;
- a un `kind: plugin` le falta `language` o `entrypoint`, o un `kind: theme`
  los declara, o declara `permissions`;
- un permiso o una capacidad es desconocido;
- un módulo frontend apunta a un slot fuera de la [lista de slots](#slot-whitelist);
- una entrada de `external_fetch` incumple las
  [reglas de acceso externo](#external-fetch-rules);
- el ZIP supera los límites al extraerlo: más de 5.000 archivos, un archivo de
  más de 100 MB, más de 500 MB extraídos, un ratio de compresión superior a
  100:1, una ruta que sale del archivo o un enlace simbólico;
- el análisis estático encuentra un patrón crítico (ejecución de shell,
  ofuscación, importación dinámica, escape del sandbox, cambio de privilegios,
  minería...). Los programas compilados y los archivos comprimidos incrustados se
  rechazan, salvo que el manifiesto declare `native-binary`.

El análisis también lee los comentarios: una línea `# pip install foo` en un
comentario se señala como código real. Quita esas líneas antes de publicar. La
lista completa de categorías está en
[Modelo de seguridad y sandbox de plugins](../SECURITY.md#static-analysis).

Antes de instalar, el administrador ve los permisos, slots y hosts declarados y
los hallazgos del análisis.

## En qué condiciones se ejecuta el plugin

Declare lo que declare el manifiesto:

- el plugin ve su propio directorio, no los datos del servidor;
- no tiene acceso directo a la red: un plugin con el permiso `network` llega a
  los hosts de su lista `external_fetch` a través de la llamada `Fetch` del
  anfitrión;
- sin `subprocess` no puede lanzar procesos hijos (los hilos funcionan);
- se ejecuta con límites de recursos que el administrador puede ajustar: por
  defecto 256 MiB de memoria, 64 procesos e hilos, 100 MiB de disco para su
  directorio y una prioridad de CPU más baja.

Firmar es opcional durante el desarrollo. Cómo firmar un plugin y qué política
de firmas puede imponer un servidor: [Escribir un plugin](dev-setup.md).

## Catálogo de permisos

Un permiso se declara bajo `permissions`; cualquier otro valor hace que se rechace la instalación. Recibir webhooks (`webhook.receiver`) y aportar metadatos (`metadata.provider`) son capacidades, declaradas bajo `capabilities`: consulta la [Referencia de la API de plugins](api-reference.md#capabilities).

| Permiso | Lo que concede |
|---|---|
| `network` | El plugin puede abrir conexiones de red salientes. **Debe** combinarse con una lista `external_fetch` no vacía; sin ella se rechaza al instalar. |
| `subprocess` | El plugin puede lanzar procesos hijos. Sin él se aplican dos capas: el shim seccomp LD_PRELOAD bloquea `execve`/`execveat` para los plugins interpretados, y el `-deny-fork` del lanzador bloquea la propia creación de procesos (`fork`/`vfork`/`clone` sin `CLONE_THREAD`) en el kernel. Los hilos funcionan en ambos casos. (El seccomp del kernel del lanzador no deniega `execve`, porque tiene que ejecutar el plugin, pero cualquier hijo hereda el filtro del kernel + el dominio Landlock.) |
| `native-binary` | El plugin puede incluir `.so`/`.dll`/`.exe`/`.wasm`/`.jar` o archivos comprimidos incrustados. El administrador ve el código opaco en la vista previa de la instalación y debe aceptarlo. |
| `filesystem` | Intención declarada de acceder al sistema de archivos más allá del directorio propio del plugin. Se registra y se muestra al administrador; el sandbox no concede acceso a archivos fuera del directorio del plugin, declare lo que declare. |
| `media.read` | El plugin puede llamar a los RPC `MediaRead`: leer los metadatos de los archivos multimedia y el acceso a los flujos. |
| `media.write` | El plugin puede llamar a los RPC `MediaWrite`: crear y actualizar registros de medios. |
| `api.library` | El plugin puede llamar a los RPC de gestión de bibliotecas: listar y buscar bibliotecas, lanzar escaneos. |
| `api.user` | El plugin puede llamar a los RPC con ámbito de usuario (leer su propio perfil, sus preferencias). |
| `api.metadata` | El plugin puede llamar a los RPC de metadatos: buscar y escribir datos de enriquecimiento. |
| `notification.sender` | El plugin puede publicar eventos de notificación (push web, correo, webhook). |
| `public` | La interfaz del plugin puede leer una proyección segura de los medios (id, título, tipo, año, resumen, póster), nunca rutas ni flujos. Consulta la [Referencia de la API de plugins](api-reference.md#public). |
| `prefs` | La interfaz del plugin puede leer y escribir las preferencias del usuario actual en el espacio de nombres propio del plugin. Consulta la [Referencia de la API de plugins](api-reference.md#prefs). |
| `hardware.gpu` | **El permiso de mayor riesgo: solo para administradores, nivel de plugin oficial.** Concede el nodo de render DRM del equipo (`-render-dev`, el `renderD1xx` real, nunca un nodo card, nunca `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`): una superficie de ataque en el controlador del kernel y un límite de memoria de GPU COMPARTIDO con el transcodificador (posible fuga de memoria entre procesos en GPU AMD, Apple y Qualcomm; una GPU colgada puede bloquear la transcodificación). Sin este permiso, los nodos de dispositivo de GPU quedan totalmente excluidos de la lista de dispositivos de Landlock (denegados sean cuales sean los permisos de archivo). Con él, el lanzador añade una regla Landlock `IOCTL_DEV` explícita solo para ese archivo (a partir del ABI v5); en ABI de Landlock más antiguos, Landlock no regula los ioctl en absoluto, así que la lectura y escritura por sí solas ya permiten enviar órdenes a la GPU. El instalador muestra un aviso CRITICAL claro en la vista previa; concédelo solo a plugins oficiales de confianza. |

## Reglas de acceso externo

Se declaran en `external_fetch: [...]` en el manifiesto y las comprueba al
instalar `validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`).
Todo o nada: una entrada no válida rechaza toda la instalación.

Una entrada es un **nombre de host simple**. Se tolera un prefijo de esquema, que
se descarta: solo se conserva el nombre de host.

| Entrada | Válida | Motivo |
|---|---|---|
| `api.tmdb.org` | Sí | Nombre de host exacto, la forma normal |
| `https://api.tmdb.org` | Sí | Esquema analizado y descartado, nombre de host conservado |
| `*.tmdb.org` | No | Los comodines se rechazan: lista cada subdominio que necesites |
| `*` | No | Misma regla |
| `192.168.1.1`, `::1` | No | Las IP literales se rechazan, ambas familias |

En ejecución, el intermediario del anfitrión añade sus propios límites, que el
manifiesto no puede ampliar: solo `http` y `https`, solo `GET` y `HEAD`, el host
de destino comparado con tu lista sin distinguir mayúsculas, y las direcciones
privadas o reservadas rechazadas tras la resolución DNS; las redirecciones se
vuelven a comprobar con ambas reglas.

> La comparación del intermediario entiende también una forma `*.example.com`,
> pero el instalador rechaza una entrada así, así que nunca puede llegar a ella.
> Escribe nombres de host explícitos.

## Lista de slots

Los plugins solo pueden inyectar interfaz en slots permitidos de forma explícita. Las rutas críticas faltan a propósito.

| Slot | Requiere sesión | Notas |
|---|---|---|
| `home.section` | Sí | Zona de contenido principal de la página |
| `home.hero` | Sí | Zona del banner principal |
| `library.sidebar` | Sí | Barra lateral de la vista de biblioteca |
| `media.detail.sidebar` | Sí | Barra lateral del detalle de un medio |
| `player.overlay` | Sí | Superposición en el reproductor |
| `settings.user.appearance` | Sí | Sección Apariencia de los ajustes de usuario |
| `sidebar.bottom` | Sí | Zona inferior de la barra de navegación |
| `admin.nav` | Sí | Una entrada de navegación en el panel de administración que abre la página propia y en sandbox del plugin; no inyecta nada en las pantallas de administración existentes |

**Nunca permitidos:** `admin.*` (salvo `admin.nav`), `login.*`, `setup.*`, `profile.security.*`: estas rutas son críticas para la autenticación y los privilegios, y una inyección de plugins ahí es una vía de escalada de privilegios o de phishing.

El contenido de `admin.nav` también está aislado como cualquier otro slot: se dibuja en un `<iframe sandbox="allow-scripts">` (sin `allow-same-origin`) con la CSP `default-src 'none'`, así que es contenido de terceros mostrado en la zona de administración, no una capacidad de administrador concedida al plugin.

Fuente: `internal/domain/plugin/slots.go`.

Los módulos frontend se ejecutan en un `<iframe>` aislado sin acceso a la página
anfitriona. El puente limita lo que pueden hacer: `evey.toast` como máximo 3
mensajes por segundo, 200 caracteres, sin HTML; `evey.navigate` solo acepta
`/home`, `/library`, `/media`, `/player`, `/settings` y `/search`;
`evey.resize` se limita a 50-800 px.

## Flujo de una actualización

1. El administrador sube un ZIP nuevo → `PreviewPluginUpdate` ejecuta el análisis estático y la verificación cosign, y devuelve las diferencias de permisos, slots y `external_fetch` respecto a la versión instalada.
2. Si las diferencias no están vacías (permisos nuevos, slots nuevos, hosts externos cambiados), el administrador debe aceptarlas de forma explícita antes de aplicar la actualización (`requires_ack = true`).
3. La suma de control de la fuente se registra en la vista previa y se comprueba al aplicar (contra TOCTOU: el ZIP analizado es el ZIP instalado).
4. Si la política cosign es `enforce`, una firma incorrecta al aplicar bloquea la actualización aunque la vista previa haya pasado.
