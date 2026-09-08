# Theme Security Model

## Theme vs plugin: key distinction

A **theme** is front-end only. It has no backend process, no language runtime, no IPC socket, and no server-side permissions. It consists of:

- `theme.css` - a stylesheet injected into `<head>` on every page
- `theme.js` (optional) - JavaScript executed inside an isolated context with a constrained `evey.theme.*` API

A **plugin** always has a backend process (`kind: plugin`, `language`, `entrypoint` required). Themes that attempt to declare `language`, `entrypoint`, or `permissions` are refused at install.

Both CSS and JS are statically analyzed at install time before the theme is accepted.

## CSS sanitizer rules

`theme.css` is passed through `SanitizeCSS` at install. The sanitizer rewrites or removes dangerous constructs and returns a list of warnings. It does **not** reject the entire file - clean CSS passes through unchanged.

### Blocked constructs

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

A rule block is removed when it contains `position: fixed` or `position: sticky` combined with at least two of: `top: 0`, `left: 0`, `width: 100vw`, `height: 100vh`. This pattern is the canonical CSS phishing overlay.

**`pointer-events: none` on interactive elements** - declaration removed from rule block.

When a rule's selector targets `button`, `[role="button"]`, `a[href*=logout]`, or `[data-critical]`, any `pointer-events: none` declaration in that rule is removed. The rest of the rule is kept. Rationale: hiding clickability on logout or critical action buttons is a UI redress attack.

**z-index above 999** - clamped to 999.

Any `z-index` value greater than 999 is rewritten to 999. A warning is recorded. Themes may not stack themselves above the application's own modal/overlay layer.

### What themes CAN do in CSS

- Customize the entire homepage layout, hero area, card grid, sidebar
- Restyle the video player controls, progress bar, overlays
- Custom fonts (via `data:` or inline `@font-face` with `src: url(data:...)`)
- Animations and transitions (`@keyframes`, `transition`, `animation`)
- Custom properties (`--my-color: #...`), `:root` overrides
- Media queries, container queries
- Any selector that does not target password fields, `.admin-*`, `.token-*`, `.secret-*`

## Theme JS scanner rules

`theme.js` is scanned at install by `internal/pkg/themejs/scan.go`. A single match causes install to be refused with a descriptive error. There are no warnings - the scanner is conservative by design (false positives are acceptable because themes have no legitimate reason to reference these APIs).

### Blocked literal strings

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

### Blocked regex patterns

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

### Theme JS runtime isolation

At runtime, `theme.js` executes inside an iframe with `sandbox="allow-scripts"` and a locked CSP `srcdoc`. Even if the install-time scanner were bypassed, the runtime iframe has no same-origin access to the host document, no access to host `window`/`localStorage`/cookies, and no outbound network (CSP `connect-src 'none'`).

The only communication channel is the `evey.theme.*` API injected by the host bridge. See [`api-reference.md`](api-reference.md) for available methods.

## What you CAN do in theme.js

- Call `evey.theme.css(rules)` to inject CSS into the host's style surface
- Call `evey.theme.log(level, msg)` to emit debug messages to the browser console
- Register component render functions via `evey.theme.registerComponent(slot, render)` (V2 - see below)
- Observe read-only player/app state via `evey.theme.observe(stateKey, cb)` (V2)

## What you CANNOT do

- Fetch data from any URL (`fetch`, `XMLHttpRequest`, `WebSocket`, `navigator.sendBeacon`)
- Read user credentials or tokens (`document.cookie`, `localStorage`, `sessionStorage`)
- Escape the iframe context (`window.parent`, `window.top`, `frameElement`)
- Render a full-screen overlay covering the entire viewport
- Target password inputs or `.admin-*`/`.token-*`/`.secret-*` selectors with CSS
- Make logout or critical-action buttons un-clickable (`pointer-events: none`)
- Stack above the application's own modals (`z-index > 999`)
