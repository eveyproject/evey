---
source_sha: 9f40858672d5badb8f1ffd97cf815109f93c3034126c5d4e6841092d5caf818a
reviewed: true
---
# Permisos y slots

Para quien crea plugins: lo que puede declarar un manifiesto, lo que
comprueba el instalador y las reglas que se aplican a un plugin mientras
funciona. El catálogo de abajo es la lista entre la que elegir; cómo encierra
el servidor a un plugin, y lo que ese encierro no puede impedir, se explica en
[Sandbox de plugins y modelo de amenazas](../SECURITY.md).

## Lo que se comprueba al instalar

Un plugin se rechaza al instalarlo cuando:

- falta `kind`, o no es ni `plugin` ni `theme`;
- a un `kind: plugin` le falta `language` o `entrypoint`, o un `kind: theme`
  los declara, o declara `permissions` o `capabilities`;
- un permiso o una capacidad es desconocido;
- un módulo de interfaz apunta a un slot que no está en la
  [lista de slots](#slot-whitelist);
- una entrada de `external_fetch` incumple las
  [reglas de las peticiones externas](#external-fetch-rules), o se declara
  `network` sin ninguna entrada;
- el ZIP incumple los [límites de extracción](../SECURITY.md#zip-extraction);
- el análisis estático encuentra un patrón crítico, o un aviso que el
  instalador trata como tal (deserialización insegura, ofuscación). Los
  binarios compilados, los archivos comprimidos incrustados y el código nativo
  se rechazan salvo que el manifiesto declare `native-binary`.

El análisis también lee los comentarios: una línea `# pip install foo` en un
comentario se señala igual que si fuera código real. Quita esas líneas antes
de publicar. Las categorías están en
[Sandbox de plugins y modelo de amenazas](../SECURITY.md#static-analysis).

Antes de instalar, el administrador ve los permisos, los slots y los hosts
declarados, y lo que ha encontrado el análisis. Nada de lo que declara un
plugin se concede sin avisar.

## Las condiciones en las que funciona el plugin

Declare lo que declare el manifiesto:

- el plugin ve su propia carpeta, no los datos del servidor;
- no tiene acceso directo a la red: un plugin con el permiso `network` llega
  a los hosts de su lista `external_fetch` a través de la llamada `Fetch` del
  servidor, y a nada más;
- sin `subprocess`, no puede lanzar procesos hijos (los hilos sí funcionan);
- funciona con unos límites de recursos que el administrador puede ajustar:
  por defecto, 256 MiB de memoria, 64 procesos e hilos, 100 MiB de disco para
  su carpeta y una prioridad de CPU más baja.

Firmar es opcional mientras desarrollas. Cómo firmar un plugin, y la política
de firmas que puede imponer un servidor:
[Escribir un plugin](dev-setup.md#7-sign-your-plugin).

## Catálogo de permisos

Un permiso se declara en `permissions`; cualquier otro valor hace que se
rechace la instalación. Recibir webhooks (`webhook.receiver`) y aportar
metadatos (`metadata.provider`) son capacidades, que se declaran en
`capabilities`: consulta la
[Referencia de la API de plugins](api-reference.md#capabilities).

| Permiso | Lo que concede |
|---|---|
| `network` | El backend puede llamar a `Fetch`. Tiene que ir con una lista `external_fetch` no vacía; sin ella, se rechaza la instalación. El proceso en sí sigue sin tener ningún socket. |
| `subprocess` | El backend puede lanzar procesos hijos. Sin él, el filtro del núcleo del lanzador rechaza `fork`, `vfork` y `clone` sin `CLONE_THREAD`, y la capa seccomp rechaza `execve` en los plugins interpretados. Los hilos siguen funcionando en ambos casos. El administrador ve un aviso específico. |
| `native-binary` | El paquete puede incluir `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, archivos comprimidos incrustados o código nativo que el análisis no puede leer. El administrador ve ese código opaco y tiene que aceptarlo. Obligatorio para un plugin `go`. |
| `filesystem` | Intención declarada de tocar el sistema de archivos fuera de la carpeta del plugin. Se registra y se muestra al administrador; el sandbox no da acceso a ningún archivo fuera de la carpeta del plugin, se declare lo que se declare. |
| `media.read` | `GetMediaItem` y `ListMediaItems` en el servidor: la ficha completa del medio. |
| `media.write` | Reservado para las llamadas de escritura de metadatos, que hoy no se sirven. |
| `api.library` | Las mismas llamadas de lectura de medios que `media.read`. |
| `api.user` | Reservado para `GetUserPreferences`, que hoy no se sirve. |
| `api.metadata` | Reservado para `UpdateMetadata`, que hoy no se sirve. |
| `notification.sender` | Reservado para `SendNotification`, que hoy no se sirve. |
| `public` | La interfaz puede leer una versión segura de los elementos multimedia (id, título, tipo, año, resumen, póster), nunca rutas ni flujos. Consulta [`evey.media.*`](api-reference.md#eveymedia-permission-public). |
| `prefs` | La interfaz puede leer y escribir las preferencias del usuario actual en el espacio de nombres propio del plugin. Consulta [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs). |
| `hardware.gpu` | El proceso recibe el nodo de renderizado DRM del servidor (el `renderD1xx` real, nunca un nodo card). Es una superficie de ataque del controlador del núcleo y una frontera de memoria de la GPU compartida con el transcodificador: una GPU bloqueada puede atascar la transcodificación. Sin él, los nodos de la GPU quedan totalmente fuera de la lista permitida de Landlock; con él, el lanzador añade una regla `IOCTL_DEV` limitada a ese único archivo (Landlock ABI 5 o posterior; en núcleos más antiguos, el acceso en lectura y escritura basta para enviar comandos). La vista previa muestra un aviso crítico. Concédelo solo a plugins que ejecutarías fuera de un sandbox. |

## Reglas de las peticiones externas

Se declaran en `external_fetch: [...]` y se comprueban al instalar
(`validateExternalFetch`). Es todo o nada: una sola entrada no válida hace
que se rechace toda la instalación.

Una entrada es un **nombre de host a secas**. Un prefijo de esquema se
tolera y se descarta; solo se guarda el nombre de host.

| Entrada | Válida | Motivo |
|---|---|---|
| `api.tmdb.org` | Sí | Nombre de host exacto, la forma normal |
| `https://api.tmdb.org` | Sí | El esquema se lee y se descarta, se guarda el nombre de host |
| `*.tmdb.org` | No | Los comodines se rechazan: enumera cada subdominio que necesites |
| `*` | No | La misma regla |
| `192.168.1.1`, `::1` | No | Las direcciones IP literales se rechazan, en las dos familias |

Mientras funciona, el intermediario del servidor añade sus propios límites,
que el manifiesto no puede ampliar: solo `http` y `https`, solo `GET` y
`HEAD`, el host de destino se compara con tu lista sin distinguir mayúsculas
de minúsculas (un sufijo `:port` en una entrada se ignora), las direcciones
privadas o reservadas se rechazan después de la resolución DNS, el cuerpo se
limita a 8 MiB y la petición a 20 segundos. Las redirecciones se vuelven a
comprobar con la lista y con la regla de direcciones.

La misma lista de hosts es el `connect-src` del marco del plugin, con el
prefijo `https://`: un widget puede llamar directamente a esos hosts, solo por
`https`.

## Lista de slots permitidos

Los plugins solo pueden inyectar interfaz en los slots permitidos de forma
explícita. Las rutas críticas no están, a propósito.

| Slot | Requiere sesión | Notas |
|---|---|---|
| `home.section` | Sí | Zona de contenido de la página principal |
| `home.hero` | Sí | Zona del banner principal |
| `library.sidebar` | Sí | Barra lateral de la vista de biblioteca |
| `media.detail.sidebar` | Sí | Barra lateral del detalle de un medio |
| `player.overlay` | Sí | Capa sobre el reproductor |
| `settings.user.appearance` | Sí | Sección Apariencia de los ajustes del usuario |
| `sidebar.bottom` | Sí | Zona inferior del panel de navegación |
| `admin.nav` | Sí | Una entrada de navegación en el panel de administración que abre la página propia del plugin, en su sandbox; no inyecta nada en las pantallas de administración existentes |

**Nunca permitidos:** `admin.*` (salvo `admin.nav`), `login.*`, `setup.*`,
`profile.security.*`. Son rutas críticas para la autenticación o los
privilegios, e inyectar algo ahí es una vía para escalar privilegios o hacer
phishing.

El contenido de `admin.nav` va en un sandbox como cualquier otro slot: se
muestra en un `<iframe sandbox="allow-scripts">` sin `allow-same-origin`, en
una página cuya política es `default-src 'none'`, así que es contenido de
terceros mostrado en la zona de administración, no una capacidad de
administración concedida al plugin.

El puente limita lo que un marco puede pedir a la página: `evey.toast` como
mucho 3 mensajes por segundo, 200 caracteres, sin HTML; `evey.navigate` solo
`/home`, `/library`, `/media`, `/player`, `/settings` y `/search`;
`evey.resize` limitado a 50-800 px. El marco no tiene ningún token: la página
anfitriona emite un token de cinco minutos limitado a (plugin, usuario) para
las llamadas `public` y `prefs`, y el servidor rechaza por defecto cualquier
método que ese token no nombre.

## Proceso de actualización

1. El administrador sube un ZIP nuevo. `PreviewPluginUpdate` ejecuta el
   análisis estático y la comprobación de la firma, y devuelve la diferencia
   de permisos, capacidades, slots y `external_fetch` con la versión
   instalada.
2. Si la diferencia no está vacía, el administrador tiene que aceptarla antes
   de que se aplique la actualización (`requires_ack = true`; por la ruta de
   subida, `accept_permission_changes=true`).
3. El hash del paquete se toma en la vista previa y se comprueba al aplicar:
   el ZIP que se analizó es el ZIP que se instala.
4. Con `EVEY_PLUGINS_COSIGN_POLICY=enforce`, un fallo de la firma en el paso
   de aplicar bloquea la actualización aunque la vista previa haya pasado.

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
analysis reads comments: docs/SECURITY.md (Static analysis)
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
