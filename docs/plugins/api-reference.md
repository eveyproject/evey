# Plugin API Reference

This document targets external developers writing an Evey plugin. For the security constraints enforced on plugins, see [`security-model.md`](security-model.md).

## Manifest schema

Every plugin ZIP must contain a `plugin.yaml` at its root.

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash
entrypoint: index.js         # required for plugins; path relative to ZIP root

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - "*.example.com"

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
    description: "TMDB API key"
```

**Validation rules enforced at install:**

- `kind: plugin` requires `language` and `entrypoint`.
- `kind: theme` must NOT declare `language`, `entrypoint`, or `permissions`.
- `web_root` is required when `frontend` is non-empty.
- `frontend[].src` must be under `web_root`.
- `frontend[].slot` must be in the server-side slot whitelist.
- `network` permission requires a non-empty `external_fetch` list.
- All `external_fetch` entries must pass the host-whitelist validator.
- All capabilities and permissions must be known to the server.

## Backend IPC

The plugin process communicates with the Evey host via gRPC over a Unix domain socket. The socket path is passed in the `EVEY_PLUGIN_SOCK` environment variable at startup.

The host authenticates the plugin by verifying the process's PID via `SO_PEERCRED` immediately after `cmd.Start`. The plugin must not fork before connecting.

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_PLUGIN_TOKEN=<jwt>        # short-lived token scoped to this plugin + user session
```

The gRPC service definitions are in `proto/evey/v1/`. Language-specific SDK stubs are generated from those protos and can be included in the plugin ZIP or vendored alongside.

The host enforces permissions at the RPC level — calling an RPC your manifest did not declare results in `PermissionDenied`.

## Frontend API

When a plugin declares a `frontend` module, the HTML for the iframe `srcdoc` includes a bridge script that exposes `window.evey` to the module's JavaScript.

The frontend module has no direct access to the host page's DOM, `window`, `fetch`, `localStorage`, or any other browser global beyond what is explicitly provided via `window.evey`.

### `evey.navigate(path)`

Navigates the host application to `path`. The path must be a whitelisted route; requests to `/admin/*`, `/auth/*`, `/setup/*`, and `/profile/security/*` are silently dropped.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Displays a toast notification in the host UI.

- `level`: `"success"` | `"error"` | `"info"`
- `message`: plain text, max 200 characters, HTML is stripped
- Rate-limited to 3 toasts per second

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Requests that the host resize the iframe slot to `height` pixels. Clamped to 50–800.

```js
evey.resize(300);
```

### `evey.onInit(callback)`

Registers a callback that fires once the host sends the plugin its initialization context. If the host has already sent `init` before `onInit` is called, the callback is invoked immediately with the cached data.

```js
evey.onInit(function(data) {
  // data.context  — plugin config values (from config_schema)
  // data.locale   — active locale string, e.g. "en-US"
  // data.theme    — active theme id
});
```

The host waits up to 5 seconds for the plugin to signal `ready` (done automatically by the bridge script). If the timeout is reached, the iframe is unmounted and an error is reported.

## Slots

| Slot | Where it appears |
|---|---|
| `home.section` | Content section on the home page |
| `home.hero` | Hero banner area on the home page |
| `library.sidebar` | Sidebar panel in library views |
| `media.detail.sidebar` | Sidebar panel on media detail pages |
| `player.overlay` | Overlay layer on the video player |
| `settings.user.appearance` | Appearance sub-section in user settings |
| `sidebar.bottom` | Bottom area of the main navigation sidebar |

Slots not listed above (including all `admin.*`, `login.*`, `setup.*`, `profile.security.*`) are rejected at install time.

## Capabilities

Capabilities declare what server-side role the plugin fulfills. They affect which RPCs the host routes to the plugin and how it registers the plugin in its service registry.

### `metadata.provider`

The plugin can supply enriched metadata for media items. The host will call the plugin's `EnrichMedia(MediaEnrichRequest) → MediaEnrichResponse` gRPC method when metadata is needed.

Required permission: `api.metadata`.

### `media.read`

The plugin may read media records and streams. Maps to the `media.read` permission in the manifest.

### `media.write`

The plugin may create or update media records. Maps to the `media.write` permission.

### `notification.sender`

The plugin may publish notifications. The host routes outbound notification events through the plugin. Maps to the `notification.sender` permission.

### `webhook.receiver`

The plugin registers as a webhook endpoint. The host forwards matching inbound HTTP POST requests to the plugin via gRPC. No additional permission required beyond the capability declaration.

## Examples

The `examples/plugins/` directory does not yet contain examples updated to the current two-kind model. New reference examples (`tmdb-enricher`, `discord-notify`, `library-stats`) are planned in Phase 8.5 of the theme/plugin redesign. Until then, the existing examples in `examples/plugins/` show the IPC wire format and gRPC stubs but use the legacy manifest format (add `kind: plugin` when adapting them).
