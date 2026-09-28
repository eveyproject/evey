# Theme API Reference

This document targets external developers writing an Evey theme. For the security constraints enforced on themes, see [`security-model.md`](security-model.md).

## What a theme is

A theme is a `kind: theme` package. It has **no backend process**. It may contain:

- `theme.css` - injected into `<head>` on every page after sanitization
- `theme.js` - optional JavaScript loaded inside an isolated context with a constrained `evey.theme.*` API

Themes cannot declare `language`, `entrypoint`, or `permissions`. Attempting to do so causes install to be refused.

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

Served at `/api/v1/themes/<id>/theme.css`. The file is passed through the CSS sanitizer at install time (see [`security-model.md`](security-model.md) for blocked constructs). The sanitized output is what gets stored and served - what you write and what is injected may differ if any rules were triggered.

The stylesheet is injected as a `<link rel="stylesheet">` in `<head>`. It applies globally to the host document. Standard CSS applies: custom properties, `@keyframes`, `@media`, `@font-face` (with `data:` sources), `@layer`, container queries.

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

### Example: custom player theme

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

Served at `/api/v1/themes/<id>/theme.js` (authentication required). Loaded inside an iframe with `sandbox="allow-scripts"` and a locked Content Security Policy - the script has no access to the host page, cannot make network requests, and cannot read storage.

The only surface available is `window.evey.theme`, injected by the host bridge before `theme.js` runs.

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

## V2 API (roadmap)

The following methods are planned and not yet available in V1:

### `evey.theme.registerComponent(slot, render)` (V2)

Registers a render function for a named slot. The host calls `render(props, container)` when the slot is mounted. `container` is a DOM node scoped to the theme's shadow context - writes to `container` are visible in the host page inside a ShadowRoot.

```js
// V2 - not yet available
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)` (V2)

Subscribes to read-only host state updates. The callback is called whenever the observed key changes.

```js
// V2 - not yet available
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)` (V2)

Applies a named animation to a host element matched by `selector` via the bridge (no direct DOM access).

## Examples

Two themes ship with the plugin examples: `examples/plugins/theme-dark-blue` (a short `theme.css` that only overrides colour variables) and `examples/plugins/open-workshop` (a complete restyle). Copy either as a starting point - both already pass the sanitizer.
