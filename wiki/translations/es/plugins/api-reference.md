---
source_sha: 2d10a959b8d809ae197a485f1214dbb6d86d37c8658c1c4d4c12d0222f6593f8
---
# Referencia de la API de extensiones

Este documento se dirige a desarrolladores externos que escriben una extensión de Evey. Las restricciones de seguridad que se aplican a las extensiones están en [`security-model.md`](security-model.md).

## Esquema del manifiesto

Todo ZIP de extensión debe contener un `plugin.yaml` en su raíz.

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # required for plugins; the full command line the interpreter runs -
                              # the first token is the interpreter, looked up on PATH; the
                              # remaining tokens are the script/args, path-confined to the ZIP.
                              # "language" is only used to check the runtime is available and to
                              # copy SDK stubs - it does NOT launch the entrypoint.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - api.example.com          # bare hostname only - no scheme, no wildcard, no literal IP

capabilities:
  - metadata.provider        # see Capabilities section

# Optional: declare a sub-directory whose contents are served as static assets.
# Required when frontend modules are declared.
web_root: public/

# Optional: frontend modules injected into the host UI.
frontend:
  - slot: home.section       # see Slot whitelist in security-model.md
    src: public/front.js     # path relative to ZIP root; must be under web_root

config_schema:               # optional; JSON Schema object describing plugin config keys
  api_key:
    type: string
    secret: true             # or `type: password`: never sent back to any client, admin included
    description: "TMDB API key"
```

Un campo marcado con `secret: true` (o de `type: password`) se guarda y se entrega
a la extensión como de costumbre, pero toda ruta de lectura - la pública y el
panel de configuración de administración - lo devuelve enmascarado. El panel de
administración conserva el valor guardado cuando la máscara se reenvía sin
cambios, así que guardar los demás campos nunca borra un secreto.

**Reglas de validación aplicadas en la instalación:**

- `kind: plugin` requiere `language` y `entrypoint`.
- `kind: theme` NO debe declarar `language`, `entrypoint` ni `permissions`.
- `web_root` es obligatorio cuando `frontend` no está vacío.
- `frontend[].src` debe estar bajo `web_root`.
- `frontend[].slot` debe figurar en la lista blanca de ranuras del servidor.
- El permiso `network` requiere una lista `external_fetch` no vacía.
- Todas las entradas de `external_fetch` deben pasar el validador de la lista blanca de hosts.
- Todas las capacidades y permisos deben ser conocidos por el servidor.

## IPC del backend

El proceso de la extensión se comunica con el anfitrión Evey por gRPC sobre un socket de dominio Unix. La ruta del socket se pasa al arrancar en la variable de entorno `EVEY_PLUGIN_SOCK`.

El anfitrión autentica el proceso de la extensión comprobando su PID mediante `SO_PEERCRED` justo después de `cmd.Start` - no se emite ningún token al propio proceso.

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

Un JWT aparte y de corta duración (5 minutos, limitado a `plugin_id` + `user_id`) se emite bajo demanda mediante la RPC `IssuePluginToken` - el *iframe frontal* de la extensión obtiene uno para autenticar sus propias llamadas de ámbito `public`/`prefs`. No tiene relación con el socket del backend de arriba y nunca se entrega al proceso de la extensión al arrancar.

Las definiciones de los servicios gRPC están en `proto/plugin/v1/plugin_api.proto`. Los stubs del SDK de cada lenguaje se generan a partir de ese proto y pueden incluirse en el ZIP de la extensión o acompañarlo. Los stubs vienen ya generados para varios lenguajes (`sdk/<lang>/`); para uno que no venga incluido, vea [dev-setup.md → Generar stubs para otro lenguaje](dev-setup.md#generating-stubs-for-another-language).

El anfitrión aplica los permisos a nivel de RPC - llamar a una RPC que su manifiesto no declaró da `PermissionDenied`.

### Almacén de datos

Cada extensión recibe su propio almacén clave-valor persistente, en el espacio de
nombres de la extensión - **sin permiso, una extensión es dueña de sus propios
datos**. Úselo para cursores, cachés, marcas de la última sincronización, todo lo
que la extensión deba recordar entre reinicios. Los valores son cadenas opacas
(codifique usted mismo en JSON si necesita estructura).

| RPC de `HostService` | Propósito |
|---|---|
| `SetData(key, value)` | Crear o sustituir un valor. |
| `GetData(key) → {value, found}` | Leer un valor. |
| `DeleteData(key)` | Quitar una clave. |
| `ListDataKeys() → {keys}` | Listar las claves de la extensión. |

El id de la extensión se toma del socket autenticado - una extensión solo puede
leer y escribir en su **propio** espacio de nombres, nunca en el de otra. Cuando
se desinstala la extensión, todo su almacén se borra automáticamente
(`ON DELETE CASCADE`), así que ningún dato obsoleto sobrevive a la retirada.

Este almacén es global para la extensión. Para guardar datos **por usuario**
(p. ej. una preferencia del usuario que está mirando), vea el permiso `prefs`
abajo - esa superficie se alcanza desde el iframe de la extensión, donde existe
un contexto de usuario.

## API frontal

Cuando una extensión declara un módulo `frontend`, el HTML del `srcdoc` del iframe incluye un script puente que expone `window.evey` al JavaScript del módulo.

El módulo frontal no tiene acceso directo al DOM de la página anfitriona, a `window`, `fetch`, `localStorage` ni a ningún otro global del navegador más allá de lo que se proporciona explícitamente a través de `window.evey`.

### `evey.navigate(path)`

Hace que la aplicación anfitriona navegue a `path`. La ruta tiene que estar en la lista blanca; las peticiones a `/admin/*`, `/auth/*`, `/setup/*` y `/profile/security/*` se descartan en silencio.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Muestra una notificación emergente en la interfaz del anfitrión.

- `level`: `"success"` | `"error"` | `"info"`
- `message`: texto plano, 200 caracteres como máximo, el HTML se elimina
- Limitado a 3 notificaciones por segundo

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Pide al anfitrión que ajuste la ranura del iframe a `height` píxeles. Limitado a 50-800. `0` pliega la ranura: una extensión que no tiene nada que mostrar no ocupa sitio en la página.

```js
evey.resize(300);
evey.resize(0); // nothing to say: the slot disappears
```

### `evey.onInit(callback)`

Registra una función que se ejecuta una vez cuando el anfitrión envía a la extensión su contexto de inicialización. Si el anfitrión ya había enviado `init` antes de llamar a `onInit`, la función se invoca de inmediato con los datos en caché.

```js
evey.onInit(function(data) {
  // data.context         - plugin config values (from config_schema)
  // data.locale          - active locale string, e.g. "en-US"
  // data.theme           - active theme id
  // data.surface         - "web", "mobile" or "tv": where the widget landed
  // data.suggestedHeight - home.section only: the height (px) the host recommends there
  // data.viewportHeight  - tv only: the panel height in dp
});
```

El anfitrión espera hasta 5 segundos a que la extensión señale `ready` (el script puente lo hace automáticamente). Si se agota el plazo, el iframe se desmonta y se informa de un error.

#### Altura

Un widget `home.section` se abre a `data.suggestedHeight` sin hacer nada: el anfitrión conoce la superficie en la que le ha colocado - una página de escritorio, un teléfono, un panel de televisión - y recomienda la altura que tiene allí un widget de inicio, para que todos los widgets de una página de inicio queden del mismo tamaño. Llame a `evey.resize()` solo cuando sepa algo mejor; el anfitrión nunca redimensiona ni escala su contenido a sus espaldas.

#### En una televisión

La aplicación de TV ejecuta su widget en una WebView, y algunas cosas que son ciertas en un navegador dejan de serlo allí:

- **No se dimensione a partir de su propio viewport.** Dentro de la WebView, `window.innerHeight` es la ranura que el anfitrión acaba de concederle - un círculo vicioso. Si se redimensiona, hágalo a partir de `data.viewportHeight` (el panel) o de `data.suggestedHeight`, nunca de `innerHeight`.
- **Ni foco ni puntero.** El widget nunca recibe el foco de la cruceta, y no hay hover. Todo lo interactivo pertenece a las superficies web y móvil.
- **Aleje el texto de los bordes.** Las televisiones recortan los bordes (overscan): deje libre cerca de un 5 % del ancho y del alto en cada lado.
- **Pruebe WebGL en un aparato real, ampliado.** Un shader se ejecuta en la GPU del aparato y en su controlador, no en los de su ordenador. Un hash de rejilla que multiplicaba su entrada por unos cientos antes de `fract()` dibujaba polígonos planos en lugar de nubes en una GPU Mali, mientras se veía bien en un navegador; mantenga acotadas las coordenadas del ruido (`mod()`) y aplique el hash a valores acotados. `tests/plugins/weather-v2` es el widget de referencia para todo lo anterior.

## Ranuras

| Ranura | Dónde aparece |
|---|---|
| `home.section` | Sección de contenido en la página de inicio |
| `home.hero` | Zona del banner principal en la página de inicio |
| `library.sidebar` | Panel lateral en las vistas de biblioteca |
| `media.detail.sidebar` | Panel lateral en las páginas de detalle de los medios |
| `player.overlay` | Capa superpuesta al reproductor de vídeo |
| `settings.user.appearance` | Subsección Apariencia en los ajustes del usuario |
| `sidebar.bottom` | Zona inferior de la barra lateral de navegación |
| `admin.nav` | Una entrada en el menú de administración que abre la página aislada propia de la extensión |

`admin.nav` es la **única** ranura del área de administración. No inyecta
interfaz en las pantallas de administración existentes - añade una sola entrada
de menú que abre la página propia de su extensión en un iframe aislado, en una
ruta de administración dedicada. Pedirla aparece en el resumen de consentimiento
de la instalación.

Todas las demás ranuras bajo `admin.*`, `login.*`, `setup.*`, `profile.security.*`
se rechazan en la instalación.

## Capacidades

Las capacidades declaran qué papel del lado del servidor cumple la extensión. Influyen en qué RPC le encamina el anfitrión y en cómo la registra en su registro de servicios.

### `metadata.provider`

La extensión puede aportar metadatos enriquecidos para los elementos multimedia. No hay una RPC de enriquecimiento dedicada - el `PluginService` al que llama el anfitrión solo expone `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). Una extensión `metadata.provider` recibe los eventos pertinentes por `OnEvent` e informa de su capacidad mediante `GetCapabilities`; no implementa un método de enriquecimiento aparte.

Permiso necesario: `api.metadata`.

### `media.read`

La extensión puede leer los registros multimedia y los flujos. Corresponde al permiso `media.read` del manifiesto.

### `media.write`

La extensión puede crear o actualizar registros multimedia. Corresponde al permiso `media.write`.

### `notification.sender`

La extensión puede publicar notificaciones. El anfitrión encamina los eventos de notificación salientes a través de la extensión. Corresponde al permiso `notification.sender`.

### `webhook.receiver`

La extensión se registra como endpoint de webhook. El anfitrión reenvía por gRPC a la extensión las peticiones HTTP POST entrantes que coinciden. No se necesita ningún permiso más allá de declarar la capacidad.

## Permisos

`public` y `prefs` son **permisos**, no capacidades - no cambian el papel del lado
del servidor que cumple la extensión, sino que conceden al iframe de la extensión
ámbitos adicionales de lectura/escritura. Declárelos en `permissions` en el
manifiesto.

### `public`

Concede al iframe de la extensión acceso de solo lectura a datos no sensibles del
servidor: `GetPublicMediaItem` / `ListPublicMediaItems`, que devuelven una
**proyección segura** (id, título, tipo, año, sinopsis, URL del cartel) - nunca
rutas de archivos, tamaños, ids de proveedores ni flujos. La marca del servidor
(nombre) ya es pública y no necesita permiso. Declare `public` en `permissions`
para pedirlo.

### `prefs`

Concede al iframe de la extensión lectura/escritura de las preferencias del
**usuario actual** en el espacio de nombres propio de la extensión, mediante
`PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`). La
extensión y el usuario quedan ligados por el token del iframe, así que una
extensión solo puede tocar datos (de-su-propia-extensión, del-usuario-actual) -
nunca los de otra extensión, nunca los de otro usuario, nunca campos de
identidad o de seguridad. Las filas se borran en cascada con la extensión o con
el usuario. Declare `prefs` en `permissions`.

> **Lo que nunca puede pedir.** `admin.*`, `auth.*` (inicio/cierre de sesión,
> sesión), `system.*` y los genéricos `user.write` / `library.write` están
> bloqueados de forma rígida: se eliminan de cualquier token y se rechazan en la
> frontera de la API aunque un manifiesto intente declararlos. Un token de
> extensión con ámbito está **denegado por defecto** en la API del servidor - solo
> alcanza los métodos que abren explícitamente los permisos concedidos arriba.

## Ejemplos

`examples/plugins/` contiene extensiones que funcionan con el formato de manifiesto actual: puntos de partida en Node.js y Python (`nodejs-example`, `python-example`), una fuente de metadatos (`metadata-example`), un receptor de webhooks (`webhook-receiver`), un widget de inicio (`weather`), la configuración y los secretos de una extensión (`config-demo`), el almacén de datos con los permisos `public` / `prefs` y la ranura `admin.nav` (`data-demo`), y dos temas. [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) explica cómo copiar uno.
