---
source_sha: 6095a8b2b48d1c8ec2613e48fa7ace4603ea6bf231dc4c39b598eb130cb452bc
reviewed: false
---
# Temas

Un tema cambia el estilo de la interfaz de Evey. Es un paquete de `kind: theme`
**sin proceso backend**, que contiene:

- `theme.css`, una hoja de estilos inyectada en todas las páginas tras sanearla;
- opcionalmente `theme.js`, un script ejecutado en un marco aislado con una
  pequeña API `evey.theme`.

Un tema no puede declarar `language`, `entrypoint` ni `permissions`: si lo hace,
se rechaza la instalación. Ambos archivos se comprueban al instalar. Los temas se
instalan como los plugins, desde **Admin → Plugins**.

## Esquema del manifiesto

```yaml
name: aurora               # obligatorio; nombre legible
version: 1.0.0             # obligatorio; semver
kind: theme                # obligatorio; debe ser "theme"

description: "..."         # opcional
author: "..."              # opcional
min_evey_version: "1.0.0"  # opcional

# NADA de language, entrypoint, permissions: no son válidos para los temas
```

El ZIP debe contener `theme.css` y/o `theme.js` en su raíz. Un paquete sin ninguno de los dos se acepta, pero el tema no tiene ningún efecto visible.

## theme.css

Se sirve en `/api/v1/themes/<id>/theme.css`. El archivo pasa por el saneador de CSS al instalar.

La hoja de estilos se inyecta como un `<link rel="stylesheet">` en `<head>`. Se aplica globalmente al documento anfitrión. Vale el CSS estándar: propiedades personalizadas, `@keyframes`, `@media`, `@font-face` (con fuentes `data:`), `@layer`, consultas de contenedor.

El saneador reescribe o elimina lo que un tema no puede hacer y registra un aviso
por cada cambio; no rechaza el archivo. Lo que se guarda y se sirve es la salida
saneada, así que puede diferir de lo que escribiste.

### Lo que elimina el saneador

**`@import`**: se elimina por completo.

Se quita cualquier directiva `@import`. Motivo: los temas deben ser autónomos; importar hojas de estilo externas es una vía de fuga por la red.

**`url()` externa**: se sustituye por `none`.

Los valores `url()` que apuntan a URL `http://`, `https://` o relativas al protocolo `//` se sustituyen. Las URI `data:` (`data:image/*`, `data:font/*`) están permitidas y pasan sin cambios.

**`javascript:` en `url()`**: se sustituye por `none`.

**`expression()`** (herencia de IE): se sustituye por `none`.

**`-moz-binding` y `behavior`**: se eliminan.

Antiguas vías de ejecución XBL/HTC, eliminadas por completo.

**Reglas con selectores sensibles**: se elimina el bloque de reglas entero.

Los bloques cuyo selector corresponde a cualquiera de los siguientes se quitan por completo:

- `[type="password"]` o `[type='password']` o `[type=password]`
- `[type="hidden"]`
- `.admin-<anything>`
- `.token-<anything>`
- `.secret-<anything>`

Motivo: los temas no deben poder dar estilo a los campos de contraseña ni a elementos privilegiados de la interfaz de formas que permitan un ataque de redress de la interfaz.

**Reglas de superposición a pantalla completa**: se elimina el bloque de reglas entero.

Un bloque se elimina cuando combina `position: fixed` o `position: sticky` con una caja que cubre la pantalla: `inset: 0`, los cuatro lados a `0`, o al menos dos de `top: 0`, `right: 0`, `bottom: 0`, `left: 0`, `width: 100vw`, `height: 100vh` incluido uno de los dos tamaños del viewport. Una cabecera fijada con `top: 0; left: 0` y dimensionada por su contenido se conserva. Las declaraciones de un mismo selector se suman entre reglas, así que repartir el patrón en varios bloques no lo esquiva. Este patrón es la superposición de phishing por CSS clásica.

**`pointer-events: none` en elementos interactivos**: la declaración se elimina del bloque.

Cuando el selector de una regla apunta a `button`, `[role="button"]`, `a[href*=logout]` o `[data-critical]`, se elimina cualquier declaración `pointer-events: none` de esa regla. El resto de la regla se conserva. Motivo: ocultar que se puede pulsar el cierre de sesión o un botón de acción crítica es un ataque de redress de la interfaz.

**z-index por encima de 999**: se limita a 999.

Cualquier valor de `z-index` mayor que 999 se reescribe a 999. Se registra un aviso. Los temas no pueden ponerse por encima de la propia capa de ventanas modales y superposiciones de la aplicación.

### Lo que puede hacer un tema en CSS

- Personalizar toda la disposición de la página de inicio, la zona destacada, la cuadrícula de tarjetas, la barra lateral
- Cambiar el estilo de los controles del reproductor de vídeo, la barra de progreso, las superposiciones
- Fuentes personalizadas (mediante `data:` o `@font-face` en línea con `src: url(data:...)`)
- Animaciones y transiciones (`@keyframes`, `transition`, `animation`)
- Propiedades personalizadas (`--my-color: #...`), anulaciones en `:root`
- Consultas de medios, consultas de contenedor
- Cualquier selector que no apunte a campos de contraseña, `.admin-*`, `.token-*`, `.secret-*`

### Ganchos de estilo estables (contrato de selectores)

Evey marca un pequeño conjunto de elementos estéticos y no sensibles con **clases
estables** a las que puedes apuntar sin depender de las clases utilitarias
(volátiles). Son un contrato: no se renombrarán en las refactorizaciones.

| Selector | Elemento | Seguro de reestilizar |
|---|---|---|
| `.evey-app-header` | la barra superior | colores, altura, espaciado, desenfoque; **nunca** ocultar ni mover los controles que contiene |
| `.evey-media-grid` | las cuadrículas de tarjetas de medios (biblioteca, filas) | columnas, `gap`, alineación |
| `.evey-media-card` | una tarjeta de medio (póster + datos) | ancho, espaciado, hover |
| `.evey-poster` | la imagen del póster (también en el detalle) | `aspect-ratio`, `border-radius`, sombra, ajuste |
| `.evey-media-detail` | la raíz de la página de detalle de un medio | disposición del cuerpo del detalle |

Todo lo que prohíbe el saneador sigue aplicándose (sin `@import`, sin
superposiciones a pantalla completa, sin `pointer-events:none` en los controles,
sin `[type=password]` / `.admin-*` / `.token-*` / `.secret-*`, `z-index`
limitado). Un tema puede reestilizar libremente estos ganchos; **no** puede
desviar la cabecera para ocultar o suplantar los controles de confianza (cierre
de sesión, perfil, notificaciones reales).

### Ejemplo: pósteres más grandes, cuadrícula más densa, esquinas más rectas

```css
/* Pósteres más grandes en una cuadrícula más apretada */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Ejemplo: otro color de acento

La interfaz dibuja los botones, los enlaces, los estados activos y la barra de
progreso del reproductor a partir de unas pocas propiedades personalizadas;
`examples/plugins/theme-dark-blue/theme.css` las lista. Anularlas cambia el
estilo de todas las pantallas a la vez:

```css
:root {
  --accent: #e8a33d;
  --accent-hover: #f0b95c;
}
```

## theme.js

Se sirve en `/api/v1/themes/<id>/theme.js` (requiere autenticación). Se carga dentro de un iframe con `sandbox="allow-scripts"` y una Content Security Policy cerrada: el script no tiene acceso a la página anfitriona, no puede hacer peticiones de red y no puede leer el almacenamiento.

La única superficie disponible es `window.evey.theme`, que el puente del anfitrión inyecta antes de que se ejecute `theme.js`.

Incluso un script que pasó el analizador de la instalación no puede llegar a la
página anfitriona, a su almacenamiento ni a la red: el marco no tiene acceso de
mismo origen y su política fija `connect-src 'none'`.

### `evey.theme.css(rules)`

Inyecta una cadena CSS en la superficie de estilos del anfitrión en tiempo de ejecución. Úsalo para estilos dinámicos que dependen de valores calculados o de la interacción del usuario dentro del tema.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Emite un mensaje de registro en la consola del navegador, con el prefijo `[evey-theme]`. No llega a los usuarios finales.

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: cadena de texto plano

```js
evey.theme.log("info", "Aurora theme loaded.");
```

### Lo que rechaza el analizador

Una sola coincidencia rechaza la instalación, con el motivo. El analizador es
deliberadamente prudente: un tema no tiene ningún motivo legítimo para usar estas
API.

#### Cadenas literales

Si cualquiera de estas subcadenas aparece en cualquier parte del código, se rechaza la instalación:

- `document.cookie`
- `localStorage`
- `sessionStorage`
- `indexedDB`
- `XMLHttpRequest`
- `navigator.sendBeacon`
- `navigator.serviceWorker`
- `WebSocket`
- `EventSource`
- `importScripts`
- `document.write`
- `Element.prototype`
- `Object.defineProperty`
- `globalThis`
- `window.parent`
- `window.top`
- `window.opener`
- `Reflect.get`
- `Reflect.apply`
- `Reflect.construct`
- `window.frames`
- `frameElement`
- `postMessage`

#### Patrones

| Patrón | Lo que detecta |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Llamadas directas a objetos globales prohibidos |
| `\bimport\s*\(` | Importación dinámica |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Acceso por corchetes a los objetos globales del anfitrión (detecta `window["fetch"]`) |
| `\.cookie\b` | cualquier acceso a `.cookie`, por descriptor de propiedad o calculado |
| `\.constructor\s*\.\s*constructor` | escape `[].constructor.constructor` |
| `new\s+Function\b` | construcción de código con `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lectura de propiedades de objetos globales prohibidos |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Asignación de propiedades a los objetos globales del anfitrión |
| `Object\s*\.\s*prototype` | Contaminación de prototipos |
| `\.register\s*\(\s*['"]\S+\.js` | Registro de un service worker |

## Ejemplos

Con los ejemplos de plugins vienen dos temas: `examples/plugins/theme-dark-blue` (un `theme.css` corto que solo anula variables de color) y `examples/plugins/open-workshop` (un cambio de estilo completo). Copia cualquiera de los dos como punto de partida: ambos pasan ya el saneador.
