---
source_sha: 2ec48afdaf5dd44ee661048d21aa84c7ac2b22e7b6454b4270a203bd473922b4
---

# Modelo de seguridad de los temas

## Tema y extensión: la diferencia que cuenta

Un **tema** vive únicamente en la interfaz. No tiene proceso de backend, ni entorno de ejecución de ningún lenguaje, ni socket IPC, ni permisos del lado del servidor. Se compone de:

- `theme.css` - una hoja de estilos que se inserta en el `<head>` de todas las páginas
- `theme.js` (opcional) - JavaScript que se ejecuta dentro de un contexto aislado con una API `evey.theme.*` muy acotada

Una **extensión** siempre tiene un proceso de backend (`kind: plugin`, con `language` y `entrypoint` obligatorios). Un tema que intente declarar `language`, `entrypoint` o `permissions` se rechaza al instalarlo.

Tanto el CSS como el JS se analizan de forma estática al instalar, antes de dar por bueno el tema.

## Reglas del saneador de CSS

`theme.css` pasa por `SanitizeCSS` al instalarse. El saneador reescribe o elimina las construcciones peligrosas y devuelve una lista de avisos. **No** rechaza el archivo entero: un CSS limpio pasa intacto.

### Construcciones bloqueadas

**`@import`**: se elimina por completo.

Cualquier directiva `@import` se suprime. Motivo: un tema tiene que bastarse a sí mismo, y la importación de hojas de estilo externas es una vía de fuga hacia la red.

**`url()` externa**: se sustituye por `none`.

Los valores de `url()` que apunten a `http://`, a `https://` o a una URL relativa al protocolo (`//`) se sustituyen. Los URI `data:` (`data:image/*`, `data:font/*`) sí se admiten y pasan intactos.

**`javascript:` dentro de `url()`**: se sustituye por `none`.

**`expression()`** (herencia de IE): se sustituye por `none`.

**`-moz-binding` y `behavior`**: se eliminan.

Son vectores de ejecución heredados de XBL y HTC, y se suprimen por completo.

**Reglas con selectores sensibles**: se elimina el bloque de reglas entero.

Se suprimen por completo los bloques cuyo selector case con alguno de estos:

- `[type="password"]`, `[type='password']` o `[type=password]`
- `[type="hidden"]`
- `.admin-<lo que sea>`
- `.token-<lo que sea>`
- `.secret-<lo que sea>`

Motivo: un tema no puede quedar en condiciones de dar estilo a los campos de contraseña ni a los elementos de interfaz privilegiados de una forma que sirva para suplantar visualmente la interfaz.

**Reglas de capa superpuesta a pantalla completa**: se elimina el bloque de reglas entero.

Un bloque se elimina cuando contiene `position: fixed` o `position: sticky` junto con al menos dos de estas: `top: 0`, `left: 0`, `width: 100vw`, `height: 100vh`. Es el patrón canónico de la capa de phishing hecha con CSS.

**`pointer-events: none` sobre elementos interactivos**: se elimina esa declaración del bloque.

Cuando el selector de una regla apunta a `button`, `[role="button"]`, `a[href*=logout]` o `[data-critical]`, se elimina de esa regla cualquier declaración `pointer-events: none`. El resto de la regla se conserva. Motivo: dejar sin efecto la pulsación sobre el botón de cerrar sesión o sobre las acciones críticas es un ataque de suplantación de la interfaz.

**`z-index` por encima de 999**: se acota a 999.

Todo valor de `z-index` mayor que 999 se reescribe a 999 y queda registrado un aviso. Un tema no puede colocarse por encima de la capa de ventanas modales y superposiciones de la propia aplicación.

### Lo que un tema SÍ puede hacer en CSS

- Rehacer por completo la maquetación de la portada, la zona destacada, la cuadrícula de fichas y la barra lateral
- Reestilizar los controles del reproductor de vídeo, la barra de progreso y las superposiciones
- Usar tipografías propias (con `data:` o con un `@font-face` en línea que use `src: url(data:...)`)
- Animaciones y transiciones (`@keyframes`, `transition`, `animation`)
- Propiedades personalizadas (`--my-color: #...`) y anulaciones en `:root`
- Consultas de medios y consultas de contenedor
- Cualquier selector que no apunte a campos de contraseña ni a `.admin-*`, `.token-*` o `.secret-*`

## Reglas del analizador del JS de los temas

`theme.js` se analiza al instalar, en `internal/pkg/themejs/scan.go`. Una sola coincidencia basta para rechazar la instalación con un error explicativo. No hay avisos: el analizador es conservador a propósito (los falsos positivos son asumibles, porque un tema no tiene ningún motivo legítimo para mencionar estas API).

### Cadenas literales bloqueadas

Si alguna de estas subcadenas aparece en cualquier punto del código fuente, la instalación se rechaza:

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

### Patrones bloqueados por expresión regular

| Patrón | Qué atrapa |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Llamadas directas a globales prohibidas |
| `\bimport\s*\(` | Importación dinámica |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Acceso con notación de corchetes a las globales del anfitrión (atrapa `window["fetch"]`) |
| `\.cookie\b` | Cualquier acceso a `.cookie`, por descriptor de propiedad o calculado |
| `\.constructor\s*\.\s*constructor` | La escapada `[].constructor.constructor` |
| `new\s+Function\b` | Construcción de código con `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lectura de propiedad sobre globales prohibidas |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Asignación de propiedad a las globales del anfitrión |
| `Object\s*\.\s*prototype` | Contaminación de prototipos |
| `\.register\s*\(\s*['"]\S+\.js` | Registro de un service worker |

### Aislamiento del JS de un tema en ejecución

En tiempo de ejecución, `theme.js` se ejecuta dentro de un iframe con `sandbox="allow-scripts"` y un `srcdoc` con la CSP cerrada. Aunque se lograra burlar el analizador de la instalación, el iframe de ejecución no tiene acceso de mismo origen al documento anfitrión, ni acceso al `window`, al `localStorage` ni a las cookies del anfitrión, ni salida de red alguna (`connect-src 'none'` en la CSP).

El único canal de comunicación es la API `evey.theme.*` que inyecta el puente del anfitrión. Los métodos disponibles están en [`api-reference.md`](api-reference.md).

## Lo que SÍ puede hacer en theme.js

- Llamar a `evey.theme.css(rules)` para inyectar CSS en la superficie de estilos del anfitrión
- Llamar a `evey.theme.log(level, msg)` para emitir mensajes de depuración a la consola del navegador
- Registrar funciones de dibujado de componentes con `evey.theme.registerComponent(slot, render)` (V2, véase más abajo)
- Observar en solo lectura el estado del reproductor y de la aplicación con `evey.theme.observe(stateKey, cb)` (V2)

## Lo que NO puede hacer

- Descargar datos de ninguna URL (`fetch`, `XMLHttpRequest`, `WebSocket`, `navigator.sendBeacon`)
- Leer las credenciales o los tokens del usuario (`document.cookie`, `localStorage`, `sessionStorage`)
- Escapar del contexto del iframe (`window.parent`, `window.top`, `frameElement`)
- Dibujar una capa superpuesta que cubra toda la ventana
- Apuntar con el CSS a los campos de contraseña o a los selectores `.admin-*`, `.token-*` o `.secret-*`
- Dejar sin pulsar los botones de cerrar sesión o de acción crítica (`pointer-events: none`)
- Colocarse por encima de las ventanas modales de la propia aplicación (`z-index > 999`)
