# Plugin API reference

The contract a plugin is written against: the fields of `plugin.yaml`, the
two gRPC services the backend and the server exchange, and the `window.evey`
bridge an interface module gets in its frame. The walkthrough that uses them
is [Writing a plugin](dev-setup.md); what each permission allows and what the
installer refuses is in [Permissions and slots](security-model.md).

## Manifest

Every ZIP carries a `plugin.yaml` (or `plugin.yml`) at its root, 1 MB at
most.

```yaml title="plugin.yaml"
name: my-plugin              # required; unique, kebab-case
version: 1.2.3               # required; semver, X.Y.Z or X.Y.Z-label
kind: plugin                 # required; "plugin" or "theme"
language: nodejs             # required for a plugin; picks the stubs copied
entrypoint: node index.js    # required for a plugin; the command line the server runs

display_name: My Plugin      # optional; a string, or a map {en: ..., fr: ...}
description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"    # optional; install/update refused when above the server version

permissions:                 # what the backend and the interface may call
  - network
  - media.read
external_fetch:              # required with "network"; bare hostnames only
  - api.themoviedb.org
capabilities:                # the roles the plugin says it fulfils
  - metadata.provider

web_root: public/            # required when frontend is declared; served as static assets
frontend:
  - slot: home.section       # one of the allowed slots
    src: public/front.js     # must be under web_root

config_schema:               # the fields of the Config panel on the plugin's card
  api_key:
    type: password           # never sent back to any client
    label: API key           # not rendered: the panel labels fields by key
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| Field | Rule |
|---|---|
| `name`, `version`, `kind` | Required. `version` must match `X.Y.Z` or `X.Y.Z-label`; an update must carry a higher version than the installed one. |
| `display_name` | The name people see; `name` stays the identifier an update is matched on. One string counts as English and stands for every language; a map is keyed by language tag (`en`, `fr`, `pt-BR`). Each value is 1 to 40 characters, without surrounding spaces or control characters. Today the theme picker shows it. |
| `language` | Required for `kind: plugin`, any value. It picks the stubs copied next to the entrypoint (those of `sdk/<language>/`). For a script started through its shebang (`entrypoint: main.py`), `python`, `nodejs`, `php`, `ruby`, `lua`, `bash` and `sh` also require that interpreter in the image. It does not launch anything. |
| `entrypoint` | Required for `kind: plugin`. The first token is a file of the package (a binary, with `native-binary`, or a script) or else a command of the image found on its `PATH` (`node`, `python3`, `java`, `dotnet`), the rest its arguments; a missing command refuses the install. The process runs in the plugin's directory. |
| `permissions` | Each value must be in the [permissions catalog](security-model.md#permissions-catalog). `network` requires a non-empty `external_fetch`. |
| `external_fetch` | Hostnames. A scheme and a trailing path are dropped at parse; hosts match exactly, a `*.example.com` wildcard is refused at install; an entry without a port allows ports 80 and 443 only, `host:port` allows that port only; an IP literal refuses the install ([rules](security-model.md#external-fetch-rules)). |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` or `notification.sender`; anything else is refused. A theme may declare none. |
| `web_root` | Required when `frontend` is non-empty; relative, no `..`, no backslash. `.` serves the whole plugin directory and only logs a warning. |
| `frontend[]` | `slot` must be in the [slot list](#slots); `src` must be under `web_root`. |
| `config_schema` | A map of field name to `{type, label, description, required, default, min, max, options}`; `label` is not rendered, the panel labels fields by key. Types: `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (with `options: [{value, label}]`). |

A theme (`kind: theme`) must not declare `language`, `entrypoint`,
`permissions` or `capabilities`; its format is in [Themes](../themes.md).

**Secrets.** A field of `type: password`, or with `secret: true`, is stored
and handed to the backend as usual but comes back masked from every read
route, the admin **Config** panel included. Saving the panel with the mask
unchanged keeps the stored value, so editing another field never wipes a
secret.

## Backend: the two services

The backend process and the server speak gRPC over two Unix sockets whose
paths arrive in the environment:

```
EVEY_PLUGIN_ID=<plugin id>
EVEY_PLUGIN_SOCK=<path>   # your process listens here and serves PluginService
EVEY_HOST_SOCK=<path>     # your process dials here to call HostService
```

The server authenticates the process on its socket by the PID the kernel
reports (`SO_PEERCRED`); no token is handed to the process. Calls are limited
to 600 per minute per plugin. `proto/plugin/v1/plugin_api.proto` is the
contract; stubs for the bundled languages are copied next to the entrypoint
when the plugin starts ([prerequisites](dev-setup.md#1-prerequisites)).

### PluginService: what the server calls

| Method | When |
|---|---|
| `Initialize(plugin_id, config, host_version)` | Once the process has bound its socket. `config` carries the values of `config_schema` the administrator set. Answer `success: false` with an `error` to refuse to start. |
| `HealthCheck()` | Periodically, to tell a live process from a hung one. Answer `healthy: true` while you can serve. |
| `Shutdown(reason)` | When the plugin is disabled, updated, uninstalled or the server stops. Exit soon after answering. |
| `OnEvent(event_type, payload, source)` | Only `metadata.request`, to a plugin with the `metadata.provider` capability ([metadata requests](#metadata-requests) below). Any other plugin receives no event: answer `handled: false`. |
| `GetCapabilities()` | Defined in the contract; the server does not call it today. |

### HostService: what the plugin can call

A call whose permission the manifest does not declare is refused with
`PermissionDenied`. Methods marked *not served* answer `Unimplemented`; they
are part of the contract for a later version and are listed so that a stub
does not mislead you.

| Method | Permission | Notes |
|---|---|---|
| `Log(level, message, fields)` | none | Written to the plugin's log, shown under **Logs** on its card. |
| `GetConfig(key)` / `SetConfig(key, value)` | none | The plugin's configuration: the values from the **Config** panel, plus keys the plugin writes itself. At most 256 keys, keys of 256 bytes, values of 16 KiB. |
| `GetData`, `SetData`, `DeleteData`, `ListDataKeys` | none | The [data store](#data-store) below. |
| `EmitEvent(event_type, payload)` | none | Published on the server's event bus as `plugin.event.<event_type>`, where webhooks can subscribe to it. |
| `GetMediaItem(id)`, `ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` or `media.read` | The full media record, including `file_path`, `provider_ids` and `genres`. |
| `Fetch(url, method, headers)` | `network` | The only way out of the sandbox: `http` or `https`, `GET` or `HEAD`, a host of `external_fetch`, private addresses refused after resolution and through redirects. The body is capped at 8 MiB (`truncated` says so) and the whole request at 20 seconds. The server attaches no credential of its own. |
| `SearchMedia`, `GetMediaStreams`, `GetChapters`, `GetSubtitles` | `api.library` or `media.read` | *Not served.* |
| `UpdateMetadata(media_item_id, fields, provider_ids)` | `api.metadata` or `media.write`, and the `metadata.provider` capability | Gives your text for one movie or series unasked, as a [metadata answer](#metadata-requests) would. `fields` carries `locale` (required) and any of `title`, `overview`, `tagline`; the text replaces what you gave for that locale, and `fields` holding only `locale` removes it. `provider_ids` is refused: TMDB identifies the item. What one plugin stores is bounded: 16 locales per item, 200 000 texts and 128 MiB in all. Past a bound the call answers `RESOURCE_EXHAUSTED`; replacing or removing a text you already gave stays possible. |
| `GetUserPreferences` | `api.user` | *Not served.* |
| `SendNotification` | `notification.sender` | *Not served.* |

### Metadata requests

Each time the server matches or refreshes a movie or a series from TMDB, it
sends every enabled `metadata.provider` plugin an `OnEvent` with
`event_type` `metadata.request`, one plugin after the other, and waits up to
15 seconds for each. The payload:

| Field | Content |
|---|---|
| `media_item_id` | The item's id, for a later `UpdateMetadata`. |
| `type` | `movie` or `series`. |
| `title`, `original_title` | TMDB's title in the server's language, and the original one. |
| `year` | Absent when unknown. |
| `provider_ids` | `tmdb`, `imdb` and the other ids known for the item. |
| `language` | The language of TMDB's text, for example `fr`. |
| `locales` | Every language the server shows. |

Answer `handled: true` with a `result` of:

```json
{
  "locale": "fr",
  "title": "...",
  "overview": "...",
  "tagline": "...",
  "translations": {
    "de": { "overview": "..." }
  }
}
```

The top-level fields are in `locale`, or in the request's `language` when
`locale` is absent; `translations` adds other languages. Leave out a field you
do not cover. Language tags are cut to their language (`fr-FR` reads `fr`),
and a field over 8 KiB is ignored. An answer replaces everything you gave for
the item before; `handled: false`, or an answer with no text, says you do not
know the item and clears what you gave for it. An error or a timeout keeps what you gave.

The administrator chooses which of `title`, `overview` and `tagline` your text
replaces, and can ask you about the whole library at once
([Metadata plugins](../plugins.md#metadata-plugins)). Seasons and episodes are
not sent.

### Data store

Every plugin owns a persistent key/value store, with no permission to
declare: cursors, caches, a last-sync timestamp, anything to remember across
restarts. Values are opaque strings; encode JSON yourself. The plugin id comes
from the authenticated socket, so a plugin reads and writes its own namespace
only, and the whole store is deleted with the plugin.

| Method | Purpose |
|---|---|
| `SetData(key, value)` | Create or replace a value. |
| `GetData(key)` | Returns `value` and `found`. |
| `DeleteData(key)` | Remove a key. |
| `ListDataKeys()` | The plugin's keys. |

This store is global to the plugin. Data **per user** (a preference of the
person looking at the widget) lives on the interface side, through the
`prefs` permission below.

## Frontend API

An interface module runs in a frame with an opaque origin, under a policy of
`default-src 'none'`: no access to the host page, its cookies or its storage,
no form, no popup, and a `connect-src` limited to the hosts of
`external_fetch` over `https`. Everything else goes through `window.evey`,
injected before the module runs. The same module runs unchanged in the web
app and in the mobile and television apps.

### `evey.onInit(callback)`

Called once with the context the host sends; a callback registered after the
context arrived is called immediately.

```js
evey.onInit(function (data) {
  // data.config          - the plugin's config values (config_schema)
  // data.context         - the host context
  // data.locale          - the active locale, e.g. "en-US"
  // data.theme           - "dark" or "light"
  // data.surface         - "web", "mobile" or "tv"
  // data.suggestedHeight - home.section only: the height (px) the host recommends
  // data.viewportHeight  - tv only: the panel height in dp
});
```

### `evey.navigate(path)`

Moves the host application to `path`. Only `/home`, `/library`, `/media`,
`/player`, `/settings` and `/search` prefixes are followed; anything else
(the admin area, sign-in, an absolute URL) is dropped silently.

### `evey.toast(level, message)`

Shows a toast in the host interface. `level` is `"success"`, `"error"` or
`"info"`; `message` is plain text, cut at 200 characters; at most 3 toasts
per second are shown.

### `evey.resize(height)`

Asks for the slot to be `height` pixels tall, clamped between 50 and 800.
`0` collapses the slot: a widget with nothing to show takes no room. A
`home.section` widget opens at `data.suggestedHeight` without doing anything:
the host knows the surface it put you on and recommends the height a home
widget has there, so every widget lands the same size. Resize only when you
know better.

### `evey.prefs.*` (permission `prefs`)

The current user's preferences in the plugin's own namespace, bound to
(this plugin, this user) by the token the host holds for the frame. Each
call returns a promise; a missing permission rejects it.

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (permission `public`)

A safe projection of media items: id, title, type, year, overview, poster
URL. Never a provider id or a stream, and no file path or size unless the
plugin also holds `media.files` (below).

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });  // { items, total }
const data  = await evey.media.poster(id);  // the poster as a data: URL
```

`list` returns one page of at most 500 items, every type mixed (series,
seasons, episodes...) unless `type` narrows it. A plugin that needs the whole
library asks for a type and walks the pages until it has `total` items:

```js
const page = await evey.media.list({ type: "MEDIA_ITEM_TYPE_MOVIE", page: 2, pageSize: 500 });
```

With the `media.files` permission, and only when the person looking is an
administrator, each item also carries `filePath`, `fileSize` (a string, as
every 64-bit number in the API) and `quality`: `width`, `height`,
`videoCodec`, `hdrType` (empty for SDR), `bitDepth`, `bitrate` (bits per
second), `durationMs`, `container`, `audioCodec` and `audioChannels`, from
the first video track and the default audio track. For anyone else the token
is minted without that permission and these fields stay empty, so a module
shown to everyone must not rely on them; the `admin.nav` slot is where they
belong.

`poster` returns a `data:` URL because the frame's policy forbids loading an
image from the server directly; use it as an `<img>` source or a texture.

### `evey.media.trash(ids)` (permission `media.trash`, slot `admin.nav` only)

Asks Evey to move media files to the trash of their library. The plugin only
asks: Evey shows its own confirmation, built from its own record of each item
(title, year, path, size, quality), never from what the plugin sent, and
nothing moves until the administrator says yes there.

```js
const res = await evey.media.trash([id1, id2]);
// { trashed: [{ id, trashedPath, pending }], failed: [{ id, message }] }
```

On yes, each file goes, with its subtitles and `.nfo`, to `.evey-trash` at
the root of its library under the same relative path, and the item leaves
Evey. Nothing is deleted from the disk: the administrator empties that folder,
and a file moved back where it was comes back with the next scan. A movie or
an episode only: a series is a folder. The promise is rejected with
`cancelled` when the administrator says no, and with `method not permitted`
anywhere but an `admin.nav` page of a plugin granted `media.trash` in the
browser (the phone and TV apps do not offer it yet). A refusal per file (a
read-only mount, a file already gone) comes back in `failed`. `pending` is
true when the file is being copied to a trash folder on another disk in the
background: the item stays in `evey.media.list` until the copy is whole, and
the administrators get a notification either way.

`evey.media.restore(paths)` undoes it, under the same rule: given the
`trashedPath` values, Evey moves each file and its subtitles back where it
was and scans the library, so the title comes back, as a new entry matched
again from its name (the play state of the old entry is not restored). It
resolves to `{ restored: [{ trashedPath, restoredPath }], failed: [...] }`;
Evey refuses any path outside a library trash, and a place taken again.
No confirmation: it destroys nothing.

### `evey.assets.read(path)`

Reads one of the plugin's own `web_root` files, by relative path, as a
`data:` URL: a file the backend wrote at run time becomes usable in the frame
without a cross-origin concern. Only the plugin's own assets are reachable.

### `evey.config.save(values)` (slot `admin.plugin.config` only)

Writes the plugin's server config, the values an administrator sets against
`config_schema`. The given keys are merged into the stored config, the other
keys stay as they are, and the server validates the result against the
schema exactly as it does the generated form. Values are strings, as in the
form. The promise resolves to `{ values }`, the config as stored (secret
fields masked), and is rejected with the server's message when a value is
refused. Anywhere but `admin.plugin.config` the call is rejected.

```js
await evey.config.save({ city: "Brest", lat: "48.39", lon: "-4.49" });
```

The current values arrive in `onInit` as `config`, as in every slot.

### On a television

The television app runs the module in a WebView, and a few things that are
true in a browser stop being true there:

- **Do not size from your own viewport.** Inside the WebView,
  `window.innerHeight` is the slot the host just granted you. Size from
  `data.viewportHeight` or `data.suggestedHeight`.
- **No focus, no pointer.** The widget never receives the D-pad focus and
  there is no hover; anything interactive belongs to the web and mobile
  surfaces.
- **Keep text away from the edges.** Televisions overscan: leave about 5% of
  the width and height free on every side.
- **Test WebGL on a real box.** A shader runs on the box's GPU and driver,
  not on your desktop's; keep noise coordinates bounded. [eveyproject/evey-plugin-weather](https://github.com/eveyproject/evey-plugin-weather)
  is the reference widget for all of the above.

## Slots

| Slot | Where it appears |
|---|---|
| `home.section` | A content section on the home page |
| `home.hero` | The hero banner area of the home page |
| `library.sidebar` | A sidebar panel in library views |
| `media.detail.sidebar` | A sidebar panel on a media detail page |
| `player.overlay` | An overlay layer on the video player |
| `settings.user.appearance` | The appearance section of the user settings |
| `sidebar.bottom` | The bottom of the main navigation sidebar |
| `admin.nav` | One entry in the admin menu, opening the plugin's own page |
| `admin.plugin.config` | The plugin's own config panel, in its card on the admin plugins screen |

`admin.nav` and `admin.plugin.config` are the only admin-area slots. Neither
injects anything into existing admin screens: `admin.nav` adds a menu entry
that opens the plugin's page in the same sandboxed frame, on an admin-gated
route; `admin.plugin.config` takes the place of the form generated from
`config_schema`, inside the plugin's own card, for a value a plain field
cannot pick well (a city among homonyms, say). Asking for either shows in the
consent diff. Every other `admin.*`, `login.*`, `setup.*` and
`profile.security.*` slot is refused at install.

## Capabilities

A capability declares the role the plugin says it fulfils:
`metadata.provider`, `media.read`, `media.write`, `webhook.receiver` or
`notification.sender`. The installer checks the name, the administrator sees
the list in the preview and any addition at update, and the Security
Dashboard records it. The server routes on one of them: a
`metadata.provider` receives the [metadata requests](#metadata-requests) and
may call `UpdateMetadata`. The others route nothing today; a
`webhook.receiver` receives no inbound request. What a plugin can actually
do is decided by its **permissions**.

## Permissions of the interface

`public` and `prefs` are declared under `permissions` like the others. They
grant nothing to the backend process; they unlock `evey.media.*` and
`evey.prefs.*` in the frame. The frame never holds a token: the host mints a
five-minute token scoped to (plugin, user) and proxies the calls, and the
server refuses by default every method the token's scopes do not name
([ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

!!! note "What a plugin can never ask for"
    `admin.*`, `auth.*`, `system.*`, `user.write` and `library.write` are
    hard-locked: a manifest that names them is refused, a token can never
    carry them, and admin methods refuse a plugin token whatever its scopes.

## Examples

`examples/plugins/` holds working plugins in the current manifest format:
starters in Node.js and Python (`nodejs-example`, `python-example`), a
metadata provider (`metadata-example`), a webhook receiver
(`webhook-receiver`), the data store with the `public` and `prefs` permissions
and the `admin.nav` slot (`data-demo`) and a tour of every `config_schema`
field type, with an `admin.nav` page (`config-demo`). `make plugin-zips`
packages them all. The project's own plugins and themes - the weather widget,
CShell, Dark Blue and the others - each live in their own repository under
[eveyproject](https://github.com/eveyproject).

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; display_name.go - DisplayName ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - CheckEntrypoint ; launchers: runner_start.go - resolveCommand ; launcher_resolve.go - lookupLauncher
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent, Ask ; runner_control.go - SendEvent (no caller), Ask
metadata requests: internal/app/plugin/metadata_source.go - MetadataRequestEvent, metadataAskTimeout, metadataRequestPayload, parseMetadataAnswer, maxAnswerField, normalizeLocale ; internal/app/metadata/enricher.go - enrichPluginTexts, takesPluginTexts ; internal/app/plugin/metadata_source_test.go
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata (maxPluginTextField, normalizePluginLocale, library.CheckPluginTextQuota: MaxPluginTextLocalesPerItem, MaxPluginTextRows, MaxPluginTextBytes), GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; frontend/packages/client/src/web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; frontend/packages/client/src/native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
