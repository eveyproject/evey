---
source_sha: 966bebcba12b15b7aa9682ef2f60b62c161622d98d6bd445ed664ef11f411f00
reviewed: true
---
# Referencia de la API de plugins { #plugin-api-reference }

El contrato con el que se escribe un plugin: los campos de `plugin.yaml`, los
dos servicios gRPC que intercambian el backend y el servidor, y el puente
`window.evey` que recibe un módulo de interfaz en su marco. La guía paso a
paso que los usa es [Escribir un plugin](dev-setup.md); lo que permite cada
permiso y lo que rechaza el instalador está en
[Permisos y slots](security-model.md).

## Manifiesto { #manifest }

Cada ZIP lleva un `plugin.yaml` (o `plugin.yml`) en su raíz, de 1 MB como
máximo.

```yaml title="plugin.yaml"
name: my-plugin              # obligatorio; único, en kebab-case
version: 1.2.3               # obligatorio; semver, X.Y.Z o X.Y.Z-label
kind: plugin                 # obligatorio; "plugin" o "theme"
language: nodejs             # obligatorio para un plugin; un runtime de la imagen
entrypoint: node index.js    # obligatorio para un plugin; la línea de comandos que ejecuta el servidor

description: "..."           # opcional
author: "..."                # opcional
min_evey_version: "1.0.0"    # opcional; se acepta y se ignora

permissions:                 # lo que pueden llamar el backend y la interfaz
  - network
  - media.read
external_fetch:              # obligatorio con "network"; solo nombres de host a secas
  - api.themoviedb.org
capabilities:                # los papeles que el plugin dice cumplir
  - metadata.provider

web_root: public/            # obligatorio si se declara frontend; se sirve como archivos estáticos
frontend:
  - slot: home.section       # uno de los slots permitidos
    src: public/front.js     # tiene que estar dentro de web_root

config_schema:               # los campos del panel Config de la tarjeta del plugin
  api_key:
    type: password           # nunca se devuelve a ningún cliente
    label: API key           # no se muestra: el panel rotula los campos con su clave
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| Campo | Regla |
|---|---|
| `name`, `version`, `kind` | Obligatorios. `version` tiene que seguir `X.Y.Z` o `X.Y.Z-label`; una actualización tiene que llevar una versión mayor que la instalada. |
| `language` | Obligatorio para `kind: plugin`: `python`, `nodejs`, `php`, `ruby`, `lua`, `bash`, `sh` o `go`. Elige la comprobación del runtime y los stubs que se copian junto al entrypoint; no lanza nada. |
| `entrypoint` | Obligatorio para `kind: plugin`. La primera palabra es el intérprete (o el binario, con `native-binary`) y el resto son sus argumentos; el proceso se ejecuta en la carpeta del plugin. |
| `permissions` | Cada valor tiene que estar en el [catálogo de permisos](security-model.md#permissions-catalog). `network` exige un `external_fetch` no vacío. |
| `external_fetch` | Nombres de host. Un esquema y una ruta final se descartan al analizar; los hosts coinciden exactamente, un comodín `*.example.com` se rechaza en la instalación; una entrada sin puerto solo permite los puertos 80 y 443, `host:port` solo ese puerto; una IP literal hace que se rechace la instalación ([reglas](security-model.md#external-fetch-rules)). |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` o `notification.sender`; todo lo demás se rechaza. Un tema no puede declarar ninguna. |
| `web_root` | Obligatorio si `frontend` no está vacío; relativo, sin `..` ni barra invertida. `.` sirve toda la carpeta del plugin y solo anota un aviso. |
| `frontend[]` | `slot` tiene que estar en la [lista de slots](#slots); `src` tiene que estar dentro de `web_root`. |
| `config_schema` | Un mapa de nombre de campo a `{type, label, description, required, default, min, max, options}`; `label` no se muestra, el panel rotula los campos con su clave. Tipos: `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (con `options: [{value, label}]`). |

Un tema (`kind: theme`) no debe declarar `language`, `entrypoint`,
`permissions` ni `capabilities`; su formato está en [Temas](../themes.md).

**Secretos.** Un campo de `type: password`, o con `secret: true`, se guarda y
se entrega al backend como cualquier otro, pero vuelve enmascarado en todas
las rutas de lectura, incluido el panel **Configuración** de administración. Guardar
el panel con la máscara sin tocar conserva el valor guardado, así que editar
otro campo nunca borra un secreto.

## Backend: los dos servicios { #backend-the-two-services }

El proceso del backend y el servidor se comunican por gRPC mediante dos sockets Unix cuyas
rutas llegan en el entorno:

```
EVEY_PLUGIN_ID=<plugin id>
EVEY_PLUGIN_SOCK=<path>   # tu proceso escucha aquí y sirve PluginService
EVEY_HOST_SOCK=<path>     # tu proceso se conecta aquí para llamar a HostService
```

El servidor autentica el proceso en su socket por el PID que informa el
núcleo (`SO_PEERCRED`); al proceso no se le entrega ningún token. Las
llamadas se limitan a 600 por minuto por plugin.
`proto/plugin/v1/plugin_api.proto` es el contrato; los stubs para los
lenguajes incluidos se copian junto al entrypoint cuando arranca el plugin
([requisitos previos](dev-setup.md#1-prerequisites)).

### PluginService: lo que llama el servidor { #pluginservice-what-the-server-calls }

| Método | Cuándo |
|---|---|
| `Initialize(plugin_id, config, host_version)` | En cuanto el proceso ha abierto su socket. `config` lleva los valores de `config_schema` que fijó el administrador. Responde `success: false` con un `error` para negarte a arrancar. |
| `HealthCheck()` | De vez en cuando, para distinguir un proceso vivo de uno bloqueado. Responde `healthy: true` mientras puedas dar servicio. |
| `Shutdown(reason)` | Cuando el plugin se desactiva, se actualiza, se desinstala o el servidor se para. Termina poco después de responder. |
| `OnEvent(event_type, payload, source)` | Solo `metadata.request`, a un plugin con la capacidad `metadata.provider` ([peticiones de metadatos](#metadata-requests) más abajo). Cualquier otro plugin no recibe ningún evento: responde `handled: false`. |
| `GetCapabilities()` | Está definido en el contrato; hoy el servidor no lo llama. |

### HostService: lo que puede llamar el plugin { #hostservice-what-the-plugin-can-call }

Una llamada cuyo permiso no declara el manifiesto se rechaza con
`PermissionDenied`. Los métodos marcados como *no servido* responden
`Unimplemented`; forman parte del contrato para una versión futura y se
enumeran para que un stub no te confunda.

| Método | Permiso | Notas |
|---|---|---|
| `Log(level, message, fields)` | ninguno | Se escribe en el registro del plugin, que se ve en **Registros** en su tarjeta. |
| `GetConfig(key)` / `SetConfig(key, value)` | ninguno | La configuración del plugin: los valores del panel **Configuración**, más las claves que el plugin escribe por su cuenta. Como mucho 256 claves, claves de 256 bytes, valores de 16 KiB. |
| `GetData`, `SetData`, `DeleteData`, `ListDataKeys` | ninguno | El [almacén de datos](#data-store) de abajo. |
| `EmitEvent(event_type, payload)` | ninguno | Se publica en el bus de eventos del servidor como `plugin.event.<event_type>`, al que se pueden suscribir los webhooks. |
| `GetMediaItem(id)`, `ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` o `media.read` | La ficha completa del medio, con `file_path`, `provider_ids` y `genres`. |
| `Fetch(url, method, headers)` | `network` | La única salida del sandbox: `http` o `https`, `GET` o `HEAD`, un host de `external_fetch`, las direcciones privadas rechazadas después de la resolución y a través de las redirecciones. El cuerpo se limita a 8 MiB (`truncated` lo indica) y la petición entera a 20 segundos. El servidor no añade ninguna credencial propia. |
| `SearchMedia`, `GetMediaStreams`, `GetChapters`, `GetSubtitles` | `api.library` o `media.read` | *No servido.* |
| `UpdateMetadata(media_item_id, fields, provider_ids)` | `api.metadata` o `media.write`, y la capacidad `metadata.provider` | Aporta tu texto para una película o una serie sin que te lo pidan, como lo haría una [respuesta de metadatos](#metadata-requests). `fields` lleva `locale` (obligatorio) y cualquiera de `title`, `overview`, `tagline`; el texto sustituye lo que aportaste para ese idioma, y un `fields` que solo lleva `locale` lo quita. `provider_ids` se rechaza: TMDB identifica el elemento. |
| `GetUserPreferences` | `api.user` | *No servido.* |
| `SendNotification` | `notification.sender` | *No servido.* |

### Peticiones de metadatos { #metadata-requests }

Cada vez que el servidor identifica o actualiza una película o una serie desde
TMDB, envía a cada plugin `metadata.provider` activado un `OnEvent` con
`event_type` `metadata.request`, un plugin tras otro, y espera hasta 15
segundos a cada uno. El contenido:

| Campo | Contenido |
|---|---|
| `media_item_id` | El id del elemento, para un `UpdateMetadata` posterior. |
| `type` | `movie` o `series`. |
| `title`, `original_title` | El título de TMDB en el idioma del servidor, y el original. |
| `year` | Ausente cuando se desconoce. |
| `provider_ids` | `tmdb`, `imdb` y los demás ids conocidos del elemento. |
| `language` | El idioma del texto de TMDB, por ejemplo `fr`. |
| `locales` | Todos los idiomas que muestra el servidor. |

Responde `handled: true` con un `result` de la forma:

```json
{
  "locale": "fr",
  "title": "...",
  "overview": "...",
  "tagline": "...",
  "translations": {
    "de": { "overview": "..." }
  }
}
```

Los campos de primer nivel están en el idioma `locale`, o en el `language`
de la petición cuando falta `locale`; `translations` añade otros idiomas. Omite un
campo que no cubras. Las etiquetas de idioma se reducen a su idioma (`fr-FR`
se lee `fr`), y un campo de más de 8 KiB se ignora. Una respuesta sustituye
todo lo que aportaste antes para el elemento; `handled: false`, o una
respuesta sin texto, indica que no conoces el elemento y borra lo que aportaste. Si hay un
error o se agota el tiempo, se conserva lo que aportaste.

El administrador elige cuáles de `title`, `overview` y `tagline` sustituye tu
texto, y puede consultarte por toda la biblioteca de una vez
([Plugins de metadatos](../plugins.md#metadata-plugins)). Las temporadas y los
episodios no se envían.

### Almacén de datos { #data-store }

Cada plugin tiene un almacén clave/valor persistente, sin ningún permiso que
declarar: cursores, cachés, la hora de la última sincronización, cualquier
cosa que haya que recordar entre reinicios. Los valores son cadenas opacas;
codifica tú el JSON. El id del plugin sale del socket autenticado, así que un
plugin solo lee y escribe en su propio espacio de nombres, y todo el almacén
se borra con el plugin.

| Método | Para qué |
|---|---|
| `SetData(key, value)` | Crear o sustituir un valor. |
| `GetData(key)` | Devuelve `value` y `found`. |
| `DeleteData(key)` | Quitar una clave. |
| `ListDataKeys()` | Las claves del plugin. |

Este almacén es global para el plugin. Los datos **por usuario** (una
preferencia de la persona que mira el widget) se guardan del lado de la interfaz,
con el permiso `prefs` que se explica más abajo.

## API de la interfaz { #frontend-api }

Un módulo de interfaz se ejecuta en un marco con un origen opaco, bajo una
política `default-src 'none'`: sin acceso a la página anfitriona, a sus
cookies ni a su almacenamiento, sin formularios, sin ventanas emergentes, y
con un `connect-src` limitado a los hosts de `external_fetch` por `https`.
Todo lo demás pasa por `window.evey`, inyectado antes de que el módulo se
ejecute. El mismo módulo funciona sin cambios en la aplicación web y en las
aplicaciones móvil y de televisión.

### `evey.onInit(callback)` { #eveyoninitcallback }

Se llama una vez con el contexto que envía la página anfitriona; un callback
registrado después de que llegue el contexto se llama en el acto.

```js
evey.onInit(function (data) {
  // data.config          - los valores de configuración del plugin (config_schema)
  // data.context         - el contexto del host
  // data.locale          - el idioma activo, p. ej. "en-US"
  // data.theme           - "dark" o "light"
  // data.surface         - "web", "mobile" o "tv"
  // data.suggestedHeight - solo home.section: la altura (px) que recomienda la página anfitriona
  // data.viewportHeight  - solo tv: la altura del panel en dp
});
```

### `evey.navigate(path)` { #eveynavigatepath }

Lleva la aplicación anfitriona a `path`. Solo se siguen los prefijos
`/home`, `/library`, `/media`, `/player`, `/settings` y `/search`; todo lo
demás (la zona de administración, el inicio de sesión, una URL absoluta) se
descarta sin avisar.

### `evey.toast(level, message)` { #eveytoastlevel-message }

Muestra un aviso emergente en la interfaz anfitriona. `level` es
`"success"`, `"error"` o `"info"`; `message` es texto plano, cortado a 200
caracteres; se muestran como mucho 3 avisos por segundo.

### `evey.resize(height)` { #eveyresizeheight }

Pide que el slot mida `height` píxeles de alto, limitado entre 50 y 800. `0`
pliega el slot: un widget sin nada que mostrar no ocupa sitio. Un widget
`home.section` se abre a `data.suggestedHeight` sin hacer nada: la página
anfitriona conoce la superficie en la que ha colocado tu widget y recomienda la altura
que tiene ahí un widget del inicio, así que todos los widgets salen del mismo
tamaño. Cambia el tamaño solo si tienes un motivo concreto.

### `evey.prefs.*` (permiso `prefs`) { #eveyprefs-permission-prefs }

Las preferencias del usuario actual en el espacio de nombres propio del
plugin, ligadas a (este plugin, este usuario) por el token que la página
anfitriona guarda para el marco. Cada llamada devuelve una promesa; si falta
el permiso, se rechaza.

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (permiso `public`) { #eveymedia-permission-public }

Una versión segura de los elementos multimedia: id, título, tipo, año,
resumen, URL del póster. Nunca una ruta de archivo, un tamaño, un id de
proveedor ni un flujo.

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });
const data  = await evey.media.poster(id);  // el póster como URL data:
```

`poster` devuelve una URL `data:` porque la política del marco prohíbe cargar
una imagen directamente desde el servidor; úsala como fuente de un `<img>` o
como textura.

### `evey.assets.read(path)` { #eveyassetsreadpath }

Lee uno de los archivos de `web_root` del propio plugin, por ruta relativa,
como URL `data:`: un archivo que el backend escribió mientras funcionaba se
puede usar en el marco sin problemas de origen cruzado. Solo se puede llegar
a los archivos del propio plugin.

### En un televisor { #on-a-television }

La aplicación de televisión ejecuta el módulo en un WebView, y algunas cosas
que son ciertas en un navegador dejan de serlo ahí:

- **No calcules el tamaño a partir de tu propia ventana.** Dentro del
  WebView, `window.innerHeight` es el slot que la página anfitriona acaba de
  darte. Calcula el tamaño a partir de `data.viewportHeight` o
  `data.suggestedHeight`.
- **Ni foco ni puntero.** El widget nunca recibe el foco de la cruceta y no
  hay efecto al pasar por encima; todo lo interactivo corresponde a las
  superficies web y móvil.
- **Aleja el texto de los bordes.** Los televisores recortan los bordes de la
  imagen (overscan): deja libre alrededor de un 5 % de la anchura y de la
  altura por cada lado.
- **Prueba WebGL en un aparato real.** Un shader se ejecuta en la GPU y el
  controlador del aparato, no en los de tu ordenador; mantén acotadas las
  coordenadas del ruido. `tests/plugins/weather-v2` es el widget de
  referencia para todo lo anterior.

## Slots { #slots }

| Slot | Dónde aparece |
|---|---|
| `home.section` | Una sección de contenido en la página de inicio |
| `home.hero` | La zona del banner principal de la página de inicio |
| `library.sidebar` | Un panel lateral en las vistas de biblioteca |
| `media.detail.sidebar` | Un panel lateral en la página de detalle de un medio |
| `player.overlay` | Una capa sobre el reproductor de vídeo |
| `settings.user.appearance` | La sección de apariencia de los ajustes del usuario |
| `sidebar.bottom` | La parte de abajo de la barra lateral de navegación principal |
| `admin.nav` | Una entrada en el menú de administración que abre la página propia del plugin |

`admin.nav` es el único slot de la zona de administración. No inyecta nada en
las pantallas de administración existentes: añade una entrada de menú que
abre la página del plugin en el mismo marco con sandbox, en una ruta
reservada a los administradores. Pedirlo aparece en la diferencia que se
muestra para dar el consentimiento. Cualquier otro slot `admin.*`, `login.*`,
`setup.*` y `profile.security.*` se rechaza al instalar.

## Capacidades { #capabilities }

Una capacidad declara el papel que el plugin dice cumplir:
`metadata.provider`, `media.read`, `media.write`, `webhook.receiver` o
`notification.sender`. El instalador comprueba el nombre, el administrador ve
la lista en la vista previa y cualquier añadido al actualizar, y el Panel de
seguridad la registra. El servidor encamina según una de ellas:
un `metadata.provider` recibe las [peticiones de metadatos](#metadata-requests)
y puede llamar a `UpdateMetadata`. Las demás no encaminan nada hoy; un
`webhook.receiver` no recibe ninguna petición entrante. Lo que un plugin
puede hacer de verdad lo deciden sus **permisos**.

## Permisos de la interfaz { #permissions-of-the-interface }

`public` y `prefs` se declaran en `permissions` como los demás. No conceden
nada al proceso del backend; desbloquean `evey.media.*` y `evey.prefs.*` en
el marco. El marco nunca tiene un token: la página anfitriona emite un token
de cinco minutos limitado a (plugin, usuario) y hace de intermediaria en las
llamadas, y el servidor rechaza por defecto cualquier método que no nombren
los alcances del token
([ADR-0012](https://github.com/eveyproject/evey/blob/master/docs/adr/0012-plugin-token-deny-by-default.md)).

!!! note "Lo que un plugin nunca puede pedir"
    `admin.*`, `auth.*`, `system.*`, `user.write` y `library.write` están
    bloqueados sin excepción: un manifiesto que los nombra se rechaza, un
    token nunca puede llevarlos y los métodos de administración rechazan un
    token de plugin, tenga los alcances que tenga.

## Ejemplos { #examples }

`examples/plugins/` contiene plugins que funcionan con el formato actual del
manifiesto: plantillas de inicio en Node.js y Python (`nodejs-example`,
`python-example`), un proveedor de metadatos (`metadata-example`), un
receptor de webhooks (`webhook-receiver`), un widget de inicio con un paso de
compilación (`weather`), el almacén de datos con los permisos `public` y
`prefs` y el slot `admin.nav` (`data-demo`), un recorrido por todos los tipos
de campo de `config_schema`, con una página `admin.nav` (`config-demo`), y dos temas (`theme-dark-blue`,
`open-workshop`). `make plugin-zips` los empaqueta todos.

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go ; interpreters: runner_start.go - knownInterpreters
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; frontend/packages/client/src/web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; frontend/packages/client/src/native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
