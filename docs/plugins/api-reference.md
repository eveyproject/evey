# Plugin API Reference

This document targets external developers writing an Evey plugin. For the security constraints enforced on plugins, see [`security-model.md`](security-model.md).

## Manifest schema

Every plugin ZIP must contain a `plugin.yaml` at its root.

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

A field marked `secret: true` (or of `type: password`) is stored and handed to the
plugin as usual, but every read route - the public one and the admin config panel -
returns it masked. The admin panel keeps the stored value when the mask is posted
back unchanged, so saving the other fields never wipes a secret.

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

The host authenticates the plugin process by verifying its PID via `SO_PEERCRED` immediately after `cmd.Start` - no token is issued to the process itself.

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

A separate, short-lived JWT (5 minutes, scoped to `plugin_id` + `user_id`) is issued on demand via the `IssuePluginToken` RPC - the plugin's *frontend iframe* fetches one to authenticate its own `public`/`prefs`-scoped calls. It has no relation to the backend socket above and is never handed to the plugin process at startup.

The gRPC service definitions are in `proto/plugin/v1/plugin_api.proto`. Language-specific SDK stubs are generated from that proto and can be included in the plugin ZIP or vendored alongside. Stubs ship pre-generated for several languages (`sdk/<lang>/`); to target one that isn't bundled, see [dev-setup.md → Generating stubs for another language](dev-setup.md#generating-stubs-for-another-language).

The host enforces permissions at the RPC level - calling an RPC your manifest did not declare results in `PermissionDenied`.

### Data store

Every plugin gets its own persistent key/value store, namespaced to the plugin -
**no permission required, a plugin owns its own data**. Use it for cursors,
caches, last-sync timestamps, anything the plugin needs to remember across
restarts. Values are opaque strings (encode JSON yourself if you need structure).

| `HostService` RPC | Purpose |
|---|---|
| `SetData(key, value)` | Upsert a value. |
| `GetData(key) → {value, found}` | Read a value. |
| `DeleteData(key)` | Remove a key. |
| `ListDataKeys() → {keys}` | List the plugin's keys. |

The plugin id is taken from the authenticated socket - a plugin can only ever
read/write its **own** namespace, never another plugin's. When the plugin is
uninstalled, its entire store is wiped automatically (`ON DELETE CASCADE`), so
no stale data survives removal.

This store is plugin-global. To persist data **per user** (e.g. a preference for
the currently-viewing user), see the `prefs` permission below - that surface is
reached from the plugin's iframe, where a user context exists.

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

Requests that the host resize the iframe slot to `height` pixels. Clamped to 50-800.

```js
evey.resize(300);
```

### `evey.onInit(callback)`

Registers a callback that fires once the host sends the plugin its initialization context. If the host has already sent `init` before `onInit` is called, the callback is invoked immediately with the cached data.

```js
evey.onInit(function(data) {
  // data.context  - plugin config values (from config_schema)
  // data.locale   - active locale string, e.g. "en-US"
  // data.theme    - active theme id
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
| `admin.nav` | One nav entry in the admin panel opening the plugin's own sandboxed page |

`admin.nav` is the **only** admin-area slot. It does not inject UI into existing
admin screens - it adds a single menu entry that opens your plugin's own page in
a sandboxed iframe on a dedicated admin route. Requesting it shows up in the
install consent diff.

All other slots under `admin.*`, `login.*`, `setup.*`, `profile.security.*` are
rejected at install time.

## Capabilities

Capabilities declare what server-side role the plugin fulfills. They affect which RPCs the host routes to the plugin and how it registers the plugin in its service registry.

### `metadata.provider`

The plugin can supply enriched metadata for media items. There is no dedicated enrichment RPC - the `PluginService` the host calls into only exposes `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). A `metadata.provider` plugin receives the relevant events through `OnEvent` and reports its capability via `GetCapabilities`; it does not implement a separate enrichment method.

Required permission: `api.metadata`.

### `media.read`

The plugin may read media records and streams. Maps to the `media.read` permission in the manifest.

### `media.write`

The plugin may create or update media records. Maps to the `media.write` permission.

### `notification.sender`

The plugin may publish notifications. The host routes outbound notification events through the plugin. Maps to the `notification.sender` permission.

### `webhook.receiver`

The plugin registers as a webhook endpoint. The host forwards matching inbound HTTP POST requests to the plugin via gRPC. No additional permission required beyond the capability declaration.

## Permissions

`public` and `prefs` are **permissions**, not capabilities - they don't change
which server-side role the plugin fulfills, they grant the plugin's iframe
extra read/write scopes. Declare them under `permissions` in the manifest.

### `public`

Grants the plugin's iframe read-only access to non-sensitive server data:
`GetPublicMediaItem` / `ListPublicMediaItems`, which return a **safe projection**
(id, title, type, year, overview, poster URL) - never file paths, sizes,
provider ids or streams. Server branding (name) is already public and needs no
permission. Declare `public` in `permissions` to request it.

### `prefs`

Grants the plugin's iframe read/write of the **current user's** preferences in
the plugin's own namespace, via `PluginUserDataService` (`GetPref` / `SetPref` /
`DeletePref` / `ListPrefs`). The plugin and user are bound by the iframe token,
so a plugin can only touch (its-own-plugin, the-current-user) data - never
another plugin's, never another user's, never identity/security fields. Rows
cascade-delete with the plugin or the user. Declare `prefs` in `permissions`.

> **What you can never request.** `admin.*`, `auth.*` (login/logout/session),
> `system.*`, and generic `user.write` / `library.write` are hard-locked: they
> are stripped from any token and rejected at the API boundary even if a
> manifest tries to declare them. A scoped plugin token is **deny-by-default** on
> the server API - it can only reach the methods the granted permissions above
> explicitly unlock.

## Examples

The `examples/plugins/` directory does not yet contain examples updated to the current two-kind model. New reference examples (`tmdb-enricher`, `discord-notify`, `library-stats`) are planned in Phase 8.5 of the theme/plugin redesign. Until then, the existing examples in `examples/plugins/` show the IPC wire format and gRPC stubs but use the legacy manifest format (add `kind: plugin` when adapting them).
