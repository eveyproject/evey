---
source_sha: 53779971ebd8b5f41b11b97d89f9f1102e0d283c08d999b9cc028f3b92f630ca
reviewed: true
---
# Temas

Un tema cambia el aspecto de la interfaz de Evey: es un paquete de
`kind: theme` sin proceso de servidor, hecho de una hoja de estilos y,
si quieres, de un script aislado en un marco oculto. Esta página es para
quien crea temas; explica lo que pueden contener los dos archivos y lo que el
instalador reescribe o rechaza. Los temas se instalan como los plugins, desde
**Administración → Plugins**, y cada usuario elige el suyo en
**Ajustes → Apariencia → Tema**.

## Manifiesto

```yaml title="plugin.yaml"
name: aurora               # obligatorio; nombre legible
version: 1.0.0             # obligatorio; semver
kind: theme                # obligatorio

description: "..."         # opcional
author: "..."              # opcional

# sin language, entrypoint, permissions ni capabilities: si no, se rechaza la instalación
```

El ZIP lleva `theme.css` y/o `theme.js` en su raíz, junto al manifiesto. Un
paquete sin ninguno de los dos se acepta y no tiene ningún efecto visible.
`examples/plugins/theme-dark-blue` (un `theme.css` corto que cambia las
variables de color) y `examples/plugins/open-workshop` (un cambio de aspecto
completo) pasan los dos el saneador y son buenos puntos de partida.

## theme.css

La hoja de estilos se inyecta como un `<link rel="stylesheet">` en la cabecera
de cada página y se aplica a todo el documento: las propiedades
personalizadas, `@keyframes`, `@media`, `@font-face` con fuentes `data:`,
`@layer` y las container queries funcionan. Se sirve en
`/api/v1/themes/<id>/theme.css` sin autenticación, así que un tema que el
navegador recuerda se aplica también antes de iniciar sesión.

### Empieza por las propiedades personalizadas

La interfaz toma sus superficies, su texto, su color de acento, sus bordes y
sus radios de un conjunto de propiedades personalizadas en `:root`;
`examples/plugins/theme-dark-blue/theme.css` las enumera, cada una con un
comentario. Cambiarlas cambia el aspecto de todas las pantallas a la vez:

```css title="theme.css"
:root {
  --bg: #0b1120;
  --surface: #162032;
  --text: #e2e8f0;
  --accent: #3b82f6;
  --accent-hover: #60a5fa;
  --radius: 8px;
}
```

### Selectores estables para temas

Algunos elementos decorativos llevan clases estables a las que puedes
apuntar sin depender de las clases utilitarias, que cambian de una versión a
otra. Son un contrato: no se van a renombrar.

| Selector | Elemento | Se puede cambiar sin riesgo |
|---|---|---|
| `.evey-app-header` | la barra superior | colores, altura, espaciado, desenfoque; nunca ocultes ni muevas los controles que contiene |
| `.evey-media-grid` | las cuadrículas de tarjetas de medios (biblioteca, filas) | columnas, `gap`, alineación |
| `.evey-media-card` | una tarjeta de medio (póster y datos) | anchura, espaciado, efecto al pasar el ratón |
| `.evey-poster` | el póster o la imagen (también en la página de detalle) | `aspect-ratio`, `border-radius`, sombra, ajuste |
| `.evey-media-detail` | la raíz de la página de detalle de un medio | disposición del cuerpo de la página |

```css title="theme.css - pósteres más grandes, cuadrícula más densa, esquinas más rectas"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Lo que reescribe el saneador

Al instalar, `theme.css` pasa por un saneador que reescribe o quita lo que un
tema no puede hacer, anota un aviso por cada cambio y guarda el resultado:
lo que se sirve es el archivo saneado, que puede ser distinto de lo que
escribiste. Nunca rechaza el paquete.

| Lo que escribiste | Lo que pasa | Por qué |
|---|---|---|
| `@import ...` | Se quita | Un tema es autónomo; un import es una fuga hacia la red. |
| `url(http://...)`, `url(https://...)`, `url(//...)`, un candidato externo en `image-set()`, una propiedad personalizada que contiene una URL así | Se sustituye por `none` o se neutraliza | El mismo motivo. Las URI `data:` pasan sin cambios. |
| `url(javascript:...)`, `expression(...)`, `-moz-binding`, `behavior` | Se quita | Vías antiguas para ejecutar código. |
| Una regla cuyo selector coincide con `[type=password]`, `[type=hidden]`, `.admin-*`, `.token-*` o `.secret-*` | Se quita el bloque entero de la regla | Un tema no debe cambiar el aspecto de los campos de contraseña ni de los elementos con privilegios. |
| Una regla con `position: fixed` o `sticky` que cubre la pantalla: `inset: 0`, los cuatro lados a `0`, o al menos dos de `top`/`right`/`bottom`/`left: 0`, `width: 100vw`, `height: 100vh` con al menos un tamaño de ventana | Se quita el bloque entero de la regla | Es la capa de phishing clásica. Una cabecera fijada con `top: 0; left: 0` y del tamaño de su contenido se conserva. Las declaraciones de un mismo selector se suman entre reglas. |
| `pointer-events: none` en una regla que apunta a `button`, `[role=button]`, `a[href*=logout]` o `[data-critical]` | Se quita esa declaración y se conserva el resto | Hacer que un control crítico no responda al clic es un engaño de la interfaz (UI redress). |
| `z-index` por encima de 999 | Se limita a 999 | Un tema se queda por debajo de la capa de ventanas modales de la propia aplicación. |

Los caracteres escapados se decodifican antes de estas comprobaciones, así
que una secuencia de escape no sirve para eludirlas.

## theme.js

El script es opcional. Se sirve en `/api/v1/themes/<id>/theme.js` a los
usuarios conectados y se ejecuta en un `<iframe sandbox="allow-scripts">`
oculto cuya política fija `connect-src 'none'`: no tiene acceso a la página
anfitriona, a su almacenamiento ni a la red, aunque el script haya pasado el
análisis de la instalación. Lo único a lo que tiene acceso es `window.evey.theme`,
inyectado antes de que el script se ejecute.

### `evey.theme.css(declarations)`

Envía declaraciones de propiedades personalizadas a la página anfitriona, que
las aplica en `:root`. Es la única forma aceptada: una lista `--name: value`,
hasta 200 declaraciones, cada valor de 500 caracteres como máximo, sin
`url()`, `image-set`, `expression()`, `@import`, `javascript:`, `data:`,
`http(s):`, `//`, comentarios ni llaves. Todo lo demás se descarta y se
indica en la consola. Una propiedad solo tiene efecto donde la hoja de
estilos saneada la usa.

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

Envía una línea a la consola del navegador. `level` es `"debug"`, `"info"`,
`"warn"` o `"error"`; el mensaje se corta a 500 caracteres. Al usuario no se
le muestra nada.

`evey.theme.registerComponent` y `evey.theme.observe` existen en el objeto,
pero no están implementados: la página anfitriona responde con un error.

### Lo que rechaza el analizador

Al instalar, `theme.js` se analiza como texto, y una sola coincidencia
rechaza el paquete, con el motivo. El analizador es prudente a propósito: un
tema no tiene ningún motivo legítimo para mencionar estos nombres, ni
siquiera en un comentario.

Cualquiera de estas cadenas, en cualquier parte del código:

`document.cookie`, `localStorage`, `sessionStorage`, `indexedDB`,
`XMLHttpRequest`, `navigator.sendBeacon`, `navigator.serviceWorker`,
`WebSocket`, `EventSource`, `importScripts`, `document.write`,
`Element.prototype`, `Object.defineProperty`, `globalThis`, `window.parent`,
`window.top`, `window.opener`, `Reflect.get`, `Reflect.apply`,
`Reflect.construct`, `window.frames`, `frameElement`, `postMessage`.

Cualquiera de estos patrones:

| Patrón | Lo que detecta |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Llamadas directas a globales prohibidos |
| `\bimport\s*\(` | Import dinámico |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Acceso con corchetes a los globales de la página anfitriona, como `window["fetch"]` |
| `\.cookie\b` | Cualquier acceso a `.cookie` |
| `\.constructor\s*\.\s*constructor` | La escapatoria `[].constructor.constructor` |
| `new\s+Function\b` | Construcción de código |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lectura de la propiedad de un global prohibido |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Asignación a un global de la página anfitriona |
| `Object\s*\.\s*prototype` | Contaminación del prototipo |
| `\.register\s*\(\s*['"]\S+\.js` | Registro de un service worker |

<!-- sources
manifest rules: internal/adapters/driven/pluginrunner/manifest.go - validateManifest (KindTheme)
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme ; GET /api/v1/plugins/themes
routes, auth: internal/adapters/driving/http/theme_asset_handler.go (theme.css open, theme.js authMw)
link injection, hidden iframe, connect-src none: frontend/apps/web/src/features/themes/ThemeLoader.tsx
sanitizer: internal/adapters/driven/pluginrunner/css_sanitizer.go - SanitizeCSS, reImport, reURLExternal, reURLExternalUnterminated, reImageSetCandidateExternal, reCustomPropExternalURL, reURLJavascript, reExpression, reMozBinding, reBehavior, reSensitiveSelector, overlaySignals.isOverlay, stripPointerEventsOnInteractive, reZIndex ; written back at install: internal/app/plugin/install.go - validateStagedPlugin
custom properties: examples/plugins/theme-dark-blue/theme.css
stable hooks: frontend/apps/web/src (evey-app-header, evey-media-grid, evey-media-card, evey-poster, evey-media-detail)
theme.js bridge: frontend/apps/web/src/features/themes/theme-bridge.ts - THEME_BRIDGE_SCRIPT ; policy: theme-css-policy.ts - applyThemeCSSPolicy, MAX_DECLARATIONS, MAX_VALUE_LENGTH, FORBIDDEN_IN_VALUE
scanner: internal/pkg/themejs/scan.go - blockedLiterals, blockedRegex, Scan
-->
