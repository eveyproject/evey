---
source_sha: 72324d88dd91ef30b9ee280c903c4879dad626512f8f23deee264b8281e0538f
---

# Referencia de la API de temas

Este documento está dirigido a quien desarrolla un tema de Evey desde fuera del proyecto. Las restricciones de seguridad que se aplican a los temas están en [`security-model.md`](security-model.md).

## Qué es un tema

Un tema es un paquete `kind: theme`. **No tiene proceso de backend.** Puede contener:

- `theme.css` - se inserta en el `<head>` de todas las páginas, una vez saneado
- `theme.js` - JavaScript opcional, que se carga dentro de un contexto aislado con una API `evey.theme.*` muy acotada

Un tema no puede declarar `language`, `entrypoint` ni `permissions`. Si lo intenta, la instalación se rechaza.

## Esquema del manifiesto

```yaml
name: aurora               # obligatorio; nombre legible
version: 1.0.0             # obligatorio; semver
kind: theme                # obligatorio; tiene que ser "theme"

description: "..."         # opcional
author: "..."              # opcional
min_evey_version: "1.0.0"  # opcional

# NADA de language, entrypoint ni permissions: no son válidos en un tema
```

El ZIP tiene que llevar `theme.css`, `theme.js` o ambos en su raíz. Un paquete sin ninguno de los dos se acepta, pero el tema no tendrá efecto visible alguno.

## theme.css

Se sirve en `/api/v1/themes/<id>/theme.css`. El archivo pasa por el saneador de CSS al instalarse (las construcciones que se bloquean están en [`security-model.md`](security-model.md)). Lo que se almacena y se sirve es la salida ya saneada: si alguna regla ha saltado, lo que usted escribió y lo que acaba insertándose no coinciden.

La hoja de estilos se inserta como un `<link rel="stylesheet">` dentro de `<head>` y se aplica de forma global al documento anfitrión. Vale el CSS de siempre: propiedades personalizadas, `@keyframes`, `@media`, `@font-face` (con fuentes `data:`), `@layer` y consultas de contenedor.

### Puntos de enganche estables para temas (contrato de selectores)

Evey marca un puñado de elementos cosméticos y sin datos sensibles con **clases
estables** a las que usted puede apuntar sin depender de las clases utilitarias,
que sí son volátiles. Son un contrato: no se les cambiará el nombre por mucho que
se refactorice alrededor.

| Selector | Elemento | Qué se puede reestilizar |
|---|---|---|
| `.evey-app-header` | la barra superior | colores, altura, espaciado, desenfoque; **nunca** ocultar ni desplazar los controles que contiene |
| `.evey-media-grid` | las cuadrículas de fichas (biblioteca, filas) | columnas, `gap`, alineación |
| `.evey-media-card` | una ficha de contenido (cartel más datos) | anchura, espaciado, comportamiento al pasar el puntero |
| `.evey-poster` | el cartel o la ilustración (también en la ficha de detalle) | `aspect-ratio`, `border-radius`, sombra, encaje |
| `.evey-media-detail` | la raíz de la página de detalle | disposición del cuerpo de la ficha |

Todo lo que el saneador prohíbe sigue prohibido (nada de `@import`, ninguna capa
superpuesta a pantalla completa, ningún `pointer-events:none` sobre un control,
nada de `[type=password]`, `.admin-*`, `.token-*` ni `.secret-*`, y el `z-index`
acotado). Un tema puede reestilizar estos enganches a su gusto; lo que **no**
puede es reaprovechar la cabecera para ocultar o suplantar los controles de
confianza (cerrar sesión, perfil, las notificaciones de verdad).

### Ejemplo: carteles más grandes, cuadrícula más densa, esquinas más marcadas

```css
/* Carteles más grandes sobre una cuadrícula más apretada */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Ejemplo: tema propio para el reproductor

```css
.player-controls {
  background: linear-gradient(to top, rgba(0,0,0,.8), transparent);
  border-radius: 0 0 8px 8px;
}

.player-progress-bar {
  accent-color: var(--color-accent);
  height: 4px;
}
```

## theme.js

Se sirve en `/api/v1/themes/<id>/theme.js` (previa autenticación). Se carga dentro de un iframe con `sandbox="allow-scripts"` y una Content-Security-Policy cerrada a cal y canto: el script no tiene acceso a la página anfitriona, no puede hacer peticiones de red y no puede leer el almacenamiento.

La única superficie a su alcance es `window.evey.theme`, que el puente del anfitrión inyecta antes de que `theme.js` empiece a ejecutarse.

### `evey.theme.css(rules)`

Inserta una cadena de CSS en la superficie de estilos del anfitrión en tiempo de ejecución. Sirve para los estilos dinámicos que dependen de valores calculados o de lo que el usuario haga dentro del propio tema.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Emite un mensaje a la consola del navegador, con el prefijo `[evey-theme]`. No llega a los usuarios finales.

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: una cadena de texto sin formato

```js
evey.theme.log("info", "Aurora theme loaded.");
```

## API V2 (hoja de ruta)

Los métodos que siguen están previstos, pero todavía no existen en la V1:

### `evey.theme.registerComponent(slot, render)` (V2)

Registra una función de dibujado para un slot con nombre. El anfitrión llama a `render(props, container)` cuando ese slot se monta. `container` es un nodo del DOM acotado al contexto shadow del tema: lo que se escriba en `container` se ve en la página anfitriona, dentro de un ShadowRoot.

```js
// V2 - todavía no disponible
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)` (V2)

Se suscribe a las actualizaciones de estado del anfitrión, en solo lectura. La función de retorno se invoca cada vez que cambia la clave observada.

```js
// V2 - todavía no disponible
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)` (V2)

Aplica una animación con nombre a un elemento del anfitrión que case con `selector`, siempre a través del puente y sin acceso directo al DOM.

## Ejemplos

Dos temas acompañan a los ejemplos de extensiones: `examples/plugins/theme-dark-blue` (un `theme.css` corto que solo sustituye variables de color) y `examples/plugins/open-workshop` (un rediseño completo). Copie cualquiera de los dos como punto de partida - ambos pasan ya el saneador.
