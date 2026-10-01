---
source_sha: bb7b744c4f03cee0ce9a6cdfdd4b96cd6e99f6d71eee21c50f601fc4595ae619
reviewed: false
---
# Referencia de la API de plugins

Este documento está dirigido a desarrolladores externos que escriben un plugin para Evey. Para las restricciones de seguridad que se aplican a los plugins, consulta [`security-model.md`](security-model.md).

## Esquema del manifiesto

Todo ZIP de plugin debe contener un `plugin.yaml` en su raíz.

```yaml
name: my-plugin              # obligatorio; nombre legible
version: 1.2.3               # obligatorio; semver
kind: plugin                 # obligatorio; debe ser "plugin" (usa "theme" para temas solo CSS/JS)
language: nodejs             # obligatorio para plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # obligatorio para plugins; la línea de comandos completa que ejecuta el intérprete:
                              # el primer elemento es el intérprete, buscado en el PATH; los
                              # demás son el script y sus argumentos, confinados al ZIP.
                              # "language" solo sirve para comprobar que el entorno existe y para
                              # copiar los stubs del SDK: NO lanza el entrypoint.

description: "..."           # opcional
author: "..."                # opcional
min_evey_version: "1.0.0"   # opcional; versión mínima del anfitrión en semver

permissions:
  - network                  # ver la sección Permisos
  - media.read

external_fetch:              # obligatorio cuando se declara el permiso "network"
  - api.themoviedb.org       # ver las reglas de acceso externo en security-model.md
  - api.example.com          # solo nombre de host: sin esquema, sin comodín, sin IP literal

capabilities:
  - metadata.provider        # ver la sección Capacidades

# Opcional: declara un subdirectorio cuyo contenido se sirve como recursos estáticos.
# Obligatorio cuando se declaran módulos frontend.
web_root: public/

# Opcional: módulos frontend inyectados en la interfaz anfitriona.
frontend:
  - slot: home.section       # ver la lista de slots en security-model.md
    src: public/front.js     # ruta relativa a la raíz del ZIP; debe estar bajo web_root

config_schema:               # opcional; objeto JSON Schema que describe las claves de configuración del plugin
  api_key:
    type: string
    secret: true             # o `type: password`: nunca se devuelve a ningún cliente, administrador incluido
    description: "TMDB API key"
```

Un campo marcado `secret: true` (o de `type: password`) se guarda y se entrega al
plugin como de costumbre, pero todas las rutas de lectura, la pública y el panel
de configuración de administración, lo devuelven enmascarado. El panel de
administración conserva el valor guardado cuando la máscara vuelve sin cambios,
así que guardar los demás campos nunca borra un secreto.

**Reglas de validación aplicadas al instalar:**

- `kind: plugin` exige `language` y `entrypoint`.
- `kind: theme` NO debe declarar `language`, `entrypoint` ni `permissions`.
- `web_root` es obligatorio cuando `frontend` no está vacío.
- `frontend[].src` debe estar bajo `web_root`.
- `frontend[].slot` debe estar en la lista de slots permitidos del servidor.
- El permiso `network` exige una lista `external_fetch` no vacía.
- Todas las entradas de `external_fetch` deben pasar el validador de hosts permitidos.
- Todas las capacidades y permisos deben ser conocidos por el servidor.

## IPC del backend

El proceso del plugin y el anfitrión Evey hablan gRPC por dos sockets de dominio
Unix, cuyas rutas se pasan en el entorno al arrancar:

```
EVEY_PLUGIN_ID=<plugin_id>
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/plugin.sock   # tu proceso escucha aquí y sirve PluginService
EVEY_HOST_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock       # tu proceso se conecta aquí para llamar a HostService
```

El anfitrión autentica el proceso del plugin en su socket por el PID que indica
el kernel (`SO_PEERCRED`): no se emite ningún token para el propio proceso.

Un JWT aparte y de vida corta (5 minutos, limitado a `plugin_id` + `user_id`) se emite bajo demanda mediante el RPC `IssuePluginToken`: el *iframe frontend* del plugin pide uno para autenticar sus propias llamadas con ámbito `public`/`prefs`. No tiene relación con el socket del backend de arriba y nunca se entrega al proceso del plugin al arrancar.

Las definiciones de los servicios gRPC están en `proto/plugin/v1/plugin_api.proto`. Los stubs del SDK de cada lenguaje se generan a partir de ese proto y pueden incluirse en el ZIP del plugin o distribuirse junto a él. Los stubs vienen generados para varios lenguajes (`sdk/<lang>/`); para uno que no está incluido, consulta [dev-setup.md → Generar stubs para otro lenguaje](dev-setup.md#generating-stubs-for-another-language).

El anfitrión aplica los permisos a nivel de RPC: llamar a un RPC que tu manifiesto no declaró da `PermissionDenied`.

### Almacén de datos

Cada plugin tiene su propio almacén clave/valor persistente, con el espacio de
nombres del plugin: **no hace falta ningún permiso, un plugin es dueño de sus
propios datos**. Úsalo para cursores, cachés, marcas de tiempo de la última
sincronización, cualquier cosa que el plugin deba recordar entre reinicios. Los
valores son cadenas opacas (codifica tú el JSON si necesitas estructura).

| RPC de `HostService` | Propósito |
|---|---|
| `SetData(key, value)` | Crea o actualiza un valor. |
| `GetData(key) → {value, found}` | Lee un valor. |
| `DeleteData(key)` | Elimina una clave. |
| `ListDataKeys() → {keys}` | Lista las claves del plugin. |

El id del plugin se toma del socket autenticado: un plugin solo puede leer y
escribir en su **propio** espacio de nombres, nunca en el de otro plugin. Cuando
se desinstala el plugin, todo su almacén se borra automáticamente
(`ON DELETE CASCADE`), así que no sobrevive ningún dato obsoleto.

Este almacén es global para el plugin. Para guardar datos **por usuario** (por
ejemplo una preferencia del usuario que está mirando), consulta el permiso
`prefs` más abajo: esa superficie se alcanza desde el iframe del plugin, donde
existe un contexto de usuario.

## API del frontend

Cuando un plugin declara un módulo `frontend`, el HTML del `srcdoc` del iframe incluye un script puente que expone `window.evey` al JavaScript del módulo.

El módulo frontend se ejecuta en un marco en sandbox con un origen opaco: no tiene acceso al DOM, las cookies ni el almacenamiento de la página anfitriona, y solo llega a Evey a través de `window.evey`. Su propio acceso a la red está limitado por el `connect-src` del marco a los hosts de su lista `external_fetch`, por `https://`; un plugin sin esa lista no puede abrir ninguna conexión.

### `evey.navigate(path)`

Lleva la aplicación anfitriona a `path`. La ruta debe empezar por `/home`, `/library`, `/media`, `/player`, `/settings` o `/search`; cualquier otra (la zona de administración, el inicio de sesión, la configuración inicial, una URL absoluta o relativa al protocolo) se descarta en silencio.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Muestra una notificación emergente en la interfaz anfitriona.

- `level`: `"success"` | `"error"` | `"info"`
- `message`: texto plano, 200 caracteres como máximo, se elimina el HTML
- Limitado a 3 notificaciones por segundo

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Pide al anfitrión que ajuste el slot del iframe a `height` píxeles. Limitado a 50-800. `0` pliega el slot: un plugin sin nada que mostrar no ocupa espacio en la página.

```js
evey.resize(300);
evey.resize(0); // nada que decir: el slot desaparece
```

### `evey.onInit(callback)`

Registra una función que se ejecuta cuando el anfitrión envía al plugin su contexto de inicialización. Si el anfitrión ya envió `init` antes de llamar a `onInit`, la función se invoca de inmediato con los datos guardados.

```js
evey.onInit(function(data) {
  // data.context         - valores de configuración del plugin (de config_schema)
  // data.locale          - cadena del idioma activo, por ejemplo "en-US"
  // data.theme           - id del tema activo
  // data.surface         - "web", "mobile" o "tv": dónde ha aparecido el widget
  // data.suggestedHeight - solo home.section: la altura (px) que recomienda ahí el anfitrión
  // data.viewportHeight  - solo tv: la altura del panel en dp
});
```

El anfitrión espera hasta 5 segundos a que el plugin señale `ready` (lo hace automáticamente el script puente). Si se alcanza el plazo, el iframe se desmonta y se informa de un error.

#### Altura

Un widget `home.section` se abre a `data.suggestedHeight` sin hacer nada: el anfitrión conoce la superficie en la que te ha puesto (una página de escritorio, un teléfono, un panel de televisor) y recomienda la altura que tiene ahí un widget de inicio, para que todos los widgets de un inicio tengan el mismo tamaño. Llama a `evey.resize()` solo cuando sepas algo mejor; el anfitrión nunca redimensiona ni escala tu contenido a tus espaldas.

#### En un televisor

La aplicación de televisor ejecuta tu widget en un WebView, y algunas cosas que son ciertas en un navegador dejan de serlo ahí:

- **No calcules el tamaño a partir de tu propio viewport.** Dentro del WebView, `window.innerHeight` es el slot que el anfitrión acaba de concederte: es circular. Si te redimensionas, parte de `data.viewportHeight` (el panel) o de `data.suggestedHeight`, nunca de `innerHeight`.
- **Sin foco ni puntero.** El widget nunca recibe el foco del mando direccional, y no hay hover. Todo lo interactivo pertenece a las superficies web y móvil.
- **Aleja el texto de los bordes.** Los televisores recortan los bordes (overscan): deja libre alrededor de un 5 % del ancho y del alto en cada lado.
- **Prueba WebGL en un aparato real, con zoom.** Un shader se ejecuta en la GPU del aparato y con su controlador, no en los de tu ordenador. Un hash de retícula que multiplicaba su entrada por unos cientos antes de `fract()` dibujaba polígonos planos en lugar de nubes en una GPU Mali mientras se veía bien en un navegador; mantén acotadas las coordenadas del ruido (`mod()`) y aplica el hash a valores acotados. `tests/plugins/weather-v2` es el widget de referencia para todo lo anterior.

## Slots

| Slot | Dónde aparece |
|---|---|
| `home.section` | Sección de contenido en la página de inicio |
| `home.hero` | Zona del banner principal en la página de inicio |
| `library.sidebar` | Panel lateral en las vistas de biblioteca |
| `media.detail.sidebar` | Panel lateral en las páginas de detalle de un medio |
| `player.overlay` | Capa superpuesta en el reproductor de vídeo |
| `settings.user.appearance` | Subsección Apariencia en los ajustes de usuario |
| `sidebar.bottom` | Zona inferior de la barra de navegación principal |
| `admin.nav` | Una entrada de navegación en el panel de administración que abre la página propia y en sandbox del plugin |

`admin.nav` es el **único** slot de la zona de administración. No inyecta
interfaz en las pantallas de administración existentes: añade una sola entrada de
menú que abre la página propia de tu plugin en un iframe en sandbox, en una ruta
de administración dedicada. Pedirlo aparece en la diferencia de consentimiento de
la instalación.

Todos los demás slots bajo `admin.*`, `login.*`, `setup.*`, `profile.security.*`
se rechazan al instalar.

## Capacidades

Las capacidades declaran qué función del lado del servidor cumple el plugin. Influyen en qué RPC le dirige el anfitrión y en cómo lo registra en su registro de servicios.

### `metadata.provider`

El plugin puede aportar metadatos enriquecidos para los medios. No hay un RPC de enriquecimiento dedicado: el `PluginService` al que llama el anfitrión solo expone `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). Un plugin `metadata.provider` recibe los eventos pertinentes por `OnEvent` e informa de su capacidad mediante `GetCapabilities`; no implementa un método de enriquecimiento aparte.

Permiso necesario: `api.metadata`.

### `media.read`

El plugin puede leer los registros y los flujos de los medios. Corresponde al permiso `media.read` del manifiesto.

### `media.write`

El plugin puede crear o actualizar registros de medios. Corresponde al permiso `media.write`.

### `notification.sender`

El plugin puede publicar notificaciones. El anfitrión dirige a través del plugin los eventos de notificación salientes. Corresponde al permiso `notification.sender`.

### `webhook.receiver`

El plugin se registra como endpoint de webhook. El anfitrión reenvía al plugin por gRPC las peticiones HTTP POST entrantes que correspondan. No hace falta ningún permiso aparte de la declaración de la capacidad.

## Permisos

`public` y `prefs` son **permisos**, no capacidades: no cambian la función del
lado del servidor que cumple el plugin, conceden al iframe del plugin ámbitos de
lectura y escritura adicionales. Decláralos bajo `permissions` en el manifiesto.

### `public`

Concede al iframe del plugin acceso de solo lectura a datos no sensibles del
servidor: `GetPublicMediaItem` / `ListPublicMediaItems`, que devuelven una
**proyección segura** (id, título, tipo, año, resumen, URL del póster), nunca
rutas de archivos, tamaños, ids de proveedores ni flujos. La identidad del
servidor (su nombre) ya es pública y no necesita permiso. Declara `public` en
`permissions` para pedirlo.

### `prefs`

Concede al iframe del plugin la lectura y escritura de las preferencias del
**usuario actual** en el espacio de nombres propio del plugin, mediante
`PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`). El
plugin y el usuario están ligados por el token del iframe, así que un plugin solo
puede tocar los datos (de su propio plugin, del usuario actual): nunca los de
otro plugin, nunca los de otro usuario, nunca los campos de identidad o
seguridad. Las filas se borran en cascada con el plugin o con el usuario. Declara
`prefs` en `permissions`.

> **Lo que nunca puedes pedir.** `admin.*`, `auth.*` (inicio y cierre de sesión,
> sesión), `system.*` y los genéricos `user.write` / `library.write` están
> bloqueados: se eliminan de cualquier token y se rechazan en la frontera de la
> API aunque un manifiesto intente declararlos. Un token de plugin con ámbito
> **deniega por defecto** en la API del servidor: solo llega a los métodos que los
> permisos concedidos de arriba desbloquean de forma explícita.

## Ejemplos

`examples/plugins/` contiene plugins que funcionan con el formato de manifiesto actual: plantillas en Node.js y Python (`nodejs-example`, `python-example`), un proveedor de metadatos (`metadata-example`), un receptor de webhooks (`webhook-receiver`), un widget de inicio (`weather`), configuración y secretos de plugin (`config-demo`), el almacén de datos con los permisos `public` / `prefs` y el slot `admin.nav` (`data-demo`), y dos temas. [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) explica cómo copiar uno.
