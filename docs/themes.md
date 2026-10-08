# Themes

A theme restyles Evey's interface: a package of `kind: theme` with no
backend process, made of a stylesheet and, optionally, a script confined to
a hidden frame. This page is for theme authors; it says what the two files
may contain and what the installer rewrites or refuses. Themes are installed
like plugins, from **Admin → Plugins**, and each user picks one under
**Settings → Appearance → Theme**.

## Manifest

```yaml title="plugin.yaml"
name: aurora               # required; human-readable name
version: 1.0.0             # required; semver
kind: theme                # required

description: "..."         # optional
author: "..."              # optional

# no language, entrypoint, permissions or capabilities: the install is refused
```

The ZIP holds `theme.css` and/or `theme.js` at its root, next to the
manifest. A package with neither is accepted and has no visible effect.
`examples/plugins/theme-dark-blue` (a short `theme.css` overriding the colour
variables) and `examples/plugins/open-workshop` (a complete restyle) both pass
the sanitizer and are good starting points.

## theme.css

The stylesheet is injected as a `<link rel="stylesheet">` in the head of
every page and applies to the whole document: custom properties,
`@keyframes`, `@media`, `@font-face` with `data:` sources, `@layer` and
container queries all work. It is served at
`/api/v1/themes/<id>/theme.css` without authentication, so a theme the
browser remembers applies before sign-in too.

### Start from the custom properties

The interface draws its surfaces, text, accent, borders and radii from a set
of custom properties on `:root`; `examples/plugins/theme-dark-blue/theme.css`
lists them with a comment each. Overriding them restyles every screen at
once:

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

### Stable theming hooks

A few cosmetic elements carry stable classes you can target without
depending on the utility classes, which change between releases. These are a
contract: they will not be renamed.

| Selector | Element | Safe to restyle |
|---|---|---|
| `.evey-app-header` | the top bar | colours, height, spacing, blur; never hide or move the controls inside it |
| `.evey-media-grid` | the media card grids (library, rows) | columns, `gap`, alignment |
| `.evey-media-card` | one media card (poster and meta) | width, spacing, hover |
| `.evey-poster` | the poster or artwork visual (also on the detail page) | `aspect-ratio`, `border-radius`, shadow, fit |
| `.evey-media-detail` | the media detail page root | layout of the detail body |

```css title="theme.css - bigger posters, denser grid, sharper corners"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### What the sanitizer rewrites

At install, `theme.css` goes through a sanitizer that rewrites or removes
what a theme may not do, records a warning for each change, and writes the
result back: what is served is the sanitized file, which can differ from what
you wrote. It never refuses the package.

| What you wrote | What happens | Why |
|---|---|---|
| `@import ...` | Removed | A theme is self-contained; an import is a network leak. |
| `url(http://...)`, `url(https://...)`, `url(//...)`, an external candidate in `image-set()`, a custom property holding such a URL | Replaced with `none` or neutralized | Same reason. `data:` URIs pass unchanged. |
| `url(javascript:...)`, `expression(...)`, `-moz-binding`, `behavior` | Removed | Legacy execution vectors. |
| A rule whose selector matches `[type=password]`, `[type=hidden]`, `.admin-*`, `.token-*` or `.secret-*` | The whole rule block removed | A theme must not restyle password fields or privileged elements. |
| A rule with `position: fixed` or `sticky` covering the screen: `inset: 0`, the four sides at `0`, or at least two of `top`/`right`/`bottom`/`left: 0`, `width: 100vw`, `height: 100vh` including one viewport size | The whole rule block removed | The canonical phishing overlay. A header pinned with `top: 0; left: 0` and sized by its content is kept. Declarations for the same selector are added up across rules. |
| `pointer-events: none` in a rule targeting `button`, `[role=button]`, `a[href*=logout]` or `[data-critical]` | That declaration removed, the rest kept | Hiding clickability on a critical control is a UI redress. |
| `z-index` above 999 | Clamped to 999 | A theme stays below the application's own modal layer. |

Escaped characters are decoded before these checks, so an escape sequence
does not get around them.

## theme.js

A script is optional. It is served at `/api/v1/themes/<id>/theme.js` to
signed-in users and runs in a hidden `<iframe sandbox="allow-scripts">`
whose policy sets `connect-src 'none'`: no access to the host page, its
storage or the network, even for a script that passed the install scan. The
only surface is `window.evey.theme`, injected before the script runs.

### `evey.theme.css(declarations)`

Sends custom property declarations to the host page, which applies them on
`:root`. This is the only shape accepted: a `--name: value` list, up to 200
declarations, each value at most 500 characters, with no `url()`,
`image-set`, `expression()`, `@import`, `javascript:`, `data:`, `http(s):`,
`//`, comment or brace. Anything else is dropped and reported in the console.
A property only acts where the sanitized stylesheet references it.

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

Forwards a line to the browser console. `level` is `"debug"`, `"info"`,
`"warn"` or `"error"`; the message is cut at 500 characters. Nothing is shown
to the user.

`evey.theme.registerComponent` and `evey.theme.observe` exist on the object
but are not implemented: the host answers them with an error.

### What the scanner refuses

At install, `theme.js` is scanned as text, and a single match refuses the
package with the reason. The scanner is deliberately conservative: a theme
has no legitimate reason to mention these names, even in a comment.

Any of these substrings, anywhere in the source:

`document.cookie`, `localStorage`, `sessionStorage`, `indexedDB`,
`XMLHttpRequest`, `navigator.sendBeacon`, `navigator.serviceWorker`,
`WebSocket`, `EventSource`, `importScripts`, `document.write`,
`Element.prototype`, `Object.defineProperty`, `globalThis`, `window.parent`,
`window.top`, `window.opener`, `Reflect.get`, `Reflect.apply`,
`Reflect.construct`, `window.frames`, `frameElement`, `postMessage`.

Any of these patterns:

| Pattern | What it catches |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Direct calls to forbidden globals |
| `\bimport\s*\(` | Dynamic import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Bracket access on host globals, such as `window["fetch"]` |
| `\.cookie\b` | Any `.cookie` access |
| `\.constructor\s*\.\s*constructor` | The `[].constructor.constructor` escape |
| `new\s+Function\b` | Code construction |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Property read of a forbidden global |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Assignment to a host global |
| `Object\s*\.\s*prototype` | Prototype pollution |
| `\.register\s*\(\s*['"]\S+\.js` | Service worker registration |

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
