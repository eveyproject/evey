# Themes

A theme restyles Evey's interface. It is a package of `kind: theme` with **no
backend process**, containing:

- `theme.css`, a stylesheet injected into every page after sanitization;
- optionally `theme.js`, a script run in an isolated frame with a small
  `evey.theme` API.

A theme cannot declare `language`, `entrypoint` or `permissions`: the install
is refused if it does. Both files are checked at install. Themes are installed
like plugins, from **Admin → Plugins**.

## Manifest schema

```yaml
name: aurora               # required; human-readable name
version: 1.0.0             # required; semver
kind: theme                # required; must be "theme"

description: "..."         # optional
author: "..."              # optional
min_evey_version: "1.0.0"  # optional

# NO language, entrypoint, permissions - these are invalid for themes
```

The ZIP must contain `theme.css` and/or `theme.js` at its root. A package with neither is accepted but the theme has no visible effect.

## theme.css

Served at `/api/v1/themes/<id>/theme.css`. The file is passed through the CSS sanitizer at install time.

The stylesheet is injected as a `<link rel="stylesheet">` in `<head>`. It applies globally to the host document. Standard CSS applies: custom properties, `@keyframes`, `@media`, `@font-face` (with `data:` sources), `@layer`, container queries.

The sanitizer rewrites or removes what a theme may not do and records a
warning for each change; it does not refuse the file. What is stored and
served is the sanitized output, so it can differ from what you wrote.

### What the sanitizer removes

**`@import`** - removed entirely.

Any `@import` directive is stripped. Rationale: themes must be self-contained; external stylesheet imports are a network leak vector.

**External `url()`** - replaced with `none`.

`url()` values pointing to `http://`, `https://`, or protocol-relative `//` URLs are replaced. `data:` URIs (`data:image/*`, `data:font/*`) are allowed and pass through unchanged.

**`javascript:` in `url()`** - replaced with `none`.

**`expression()`** (IE legacy) - replaced with `none`.

**`-moz-binding` and `behavior`** - removed.

Legacy XBL/HTC execution vectors, removed entirely.

**Sensitive selector rules** - entire rule block removed.

Rule blocks whose selector matches any of the following are stripped completely:

- `[type="password"]` or `[type='password']` or `[type=password]`
- `[type="hidden"]`
- `.admin-<anything>`
- `.token-<anything>`
- `.secret-<anything>`

Rationale: themes must not be able to style password inputs or privileged UI elements in ways that could be used for UI redressing.

**Full-screen overlay rules** - entire rule block removed.

A rule block is removed when it combines `position: fixed` or `position: sticky` with a box that covers the screen: `inset: 0`, the four sides at `0`, or at least two of `top: 0`, `right: 0`, `bottom: 0`, `left: 0`, `width: 100vw`, `height: 100vh` including one of the two viewport sizes. A header pinned with `top: 0; left: 0` and sized by its content is kept. Declarations for the same selector are added up across rules, so splitting the pattern over several blocks does not escape it. This pattern is the canonical CSS phishing overlay.

**`pointer-events: none` on interactive elements** - declaration removed from rule block.

When a rule's selector targets `button`, `[role="button"]`, `a[href*=logout]`, or `[data-critical]`, any `pointer-events: none` declaration in that rule is removed. The rest of the rule is kept. Rationale: hiding clickability on logout or critical action buttons is a UI redress attack.

**z-index above 999** - clamped to 999.

Any `z-index` value greater than 999 is rewritten to 999. A warning is recorded. Themes may not stack themselves above the application's own modal/overlay layer.

### What a theme can do in CSS

- Customize the entire homepage layout, hero area, card grid, sidebar
- Restyle the video player controls, progress bar, overlays
- Custom fonts (via `data:` or inline `@font-face` with `src: url(data:...)`)
- Animations and transitions (`@keyframes`, `transition`, `animation`)
- Custom properties (`--my-color: #...`), `:root` overrides
- Media queries, container queries
- Any selector that does not target password fields, `.admin-*`, `.token-*`, `.secret-*`

### Stable theming hooks (selector contract)

Evey marks a small set of cosmetic, non-sensitive elements with **stable
classes** you can target without depending on the (volatile) utility classes.
These are a contract: they will not be renamed across refactors.

| Selector | Element | Safe to restyle |
|---|---|---|
| `.evey-app-header` | the top bar | colours, height, spacing, blur - **never** hide/move the controls inside it |
| `.evey-media-grid` | the media card grids (library, rows) | columns, `gap`, alignment |
| `.evey-media-card` | one media card (poster + meta) | width, spacing, hover |
| `.evey-poster` | the poster/artwork visual (also on detail) | `aspect-ratio`, `border-radius`, shadow, fit |
| `.evey-media-detail` | the media detail page root | layout of the detail body |

Anything the sanitizer forbids still applies (no `@import`, no full-screen
overlays, no `pointer-events:none` on controls, no `[type=password]` /
`.admin-*` / `.token-*` / `.secret-*`, `z-index` clamped). A theme may restyle
these hooks freely; it may **not** repurpose the header to hide or impersonate
the trusted controls (logout, profile, real notifications).

### Example: bigger posters, denser grid, sharper corners

```css
/* Larger posters on a tighter grid */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Example: a different accent colour

The interface draws buttons, links, active states and the player's progress
bar from a few custom properties; `examples/plugins/theme-dark-blue/theme.css`
lists them. Overriding them restyles every screen at once:

```css
:root {
  --accent: #e8a33d;
  --accent-hover: #f0b95c;
}
```

## theme.js

Served at `/api/v1/themes/<id>/theme.js` (authentication required). Loaded inside an iframe with `sandbox="allow-scripts"` and a locked Content Security Policy - the script has no access to the host page, cannot make network requests, and cannot read storage.

The only surface available is `window.evey.theme`, injected by the host bridge before `theme.js` runs.

Even a script that passed the install scanner cannot reach the host page, its
storage or the network: the frame has no same-origin access and its policy
sets `connect-src 'none'`.

### `evey.theme.css(rules)`

Injects a CSS string into the host's style surface at runtime. Use for dynamic styling that depends on computed values or user interaction within the theme.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Emits a log message to the browser console, prefixed `[evey-theme]`. Does not surface to end users.

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: plain text string

```js
evey.theme.log("info", "Aurora theme loaded.");
```

### What the scanner refuses

A single match refuses the install, with the reason. The scanner is
deliberately conservative: a theme has no legitimate reason to use these APIs.

#### Literal strings

If any of the following substrings appears anywhere in the source, install is refused:

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

#### Patterns

| Pattern | What it catches |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Direct calls to forbidden globals |
| `\bimport\s*\(` | Dynamic import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Bracket-notation access on host globals (catches `window["fetch"]`) |
| `\.cookie\b` | any `.cookie` access, property descriptor or computed |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` escape |
| `new\s+Function\b` | `new Function(...)` code construction |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Property read of forbidden globals |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Property assignment to host globals |
| `Object\s*\.\s*prototype` | Prototype pollution |
| `\.register\s*\(\s*['"]\S+\.js` | Service worker registration |

## Examples

Two themes ship with the plugin examples: `examples/plugins/theme-dark-blue` (a short `theme.css` that only overrides colour variables) and `examples/plugins/open-workshop` (a complete restyle). Copy either as a starting point - both already pass the sanitizer.
