# Permissions and slots

For plugin authors: what a manifest may declare, what the installer checks,
and the rules that apply to a plugin at run time. The catalog below is the
list to pick from; how the server contains a plugin, and what that
containment cannot stop, is described in
[Plugin sandbox and threat model](../SECURITY.md).

## What is checked at install

A plugin is refused at install when:

- `kind` is missing, or is neither `plugin` nor `theme`;
- a `kind: plugin` lacks `language` or `entrypoint`, or a `kind: theme`
  declares them, or declares `permissions` or `capabilities`;
- a permission or a capability is unknown;
- a frontend module targets a slot outside the [slot list](#slot-whitelist);
- an `external_fetch` entry breaks the [fetch rules](#external-fetch-rules),
  or `network` is declared without any entry;
- the ZIP breaks the [extraction limits](../SECURITY.md#zip-extraction);
- the static analysis finds a critical pattern, or a warning the installer
  treats as one (unsafe deserialization, obfuscation). Compiled binaries,
  embedded archives and native code are refused unless the manifest declares
  `native-binary`.

The analysis also reads comments: a `# pip install foo` line in a comment is
reported like real code. Remove such lines before publishing. The categories
are listed in [Plugin sandbox and threat model](../SECURITY.md#static-analysis).

Before installing, the administrator sees the declared permissions, slots and
hosts, and what the analysis found. Nothing a plugin declares is granted
silently.

## What the plugin runs under

Whatever the manifest declares:

- the plugin sees its own directory, not the server's data;
- it has no direct network access: a plugin with the `network` permission
  reaches the hosts of its `external_fetch` list through the host's `Fetch`
  call, and nothing else;
- without `subprocess`, it cannot start child processes (threads work);
- it runs under resource limits the administrator can adjust: 256 MiB of
  memory, 64 processes and threads, 100 MiB of disk for its directory, and a
  lower CPU priority by default.

Signing is optional during development. How to sign a plugin, and the
signature policy a server can enforce: [Writing a plugin](dev-setup.md#7-sign-your-plugin).

## Permissions catalog

A permission is declared under `permissions`; any other value refuses the
install. Receiving webhooks (`webhook.receiver`) and providing metadata
(`metadata.provider`) are capabilities, declared under `capabilities`: see
[Plugin API reference](api-reference.md#capabilities).

| Permission | What it grants |
|---|---|
| `network` | The backend may call `Fetch`. Must be combined with a non-empty `external_fetch` list; refused at install without it. The process itself still has no socket. |
| `subprocess` | The backend may spawn child processes. Without it, the launcher's kernel filter refuses `fork`, `vfork` and `clone` without `CLONE_THREAD`, and the seccomp shim refuses `execve` for interpreted plugins. Threads keep working either way. The administrator sees a dedicated warning. |
| `native-binary` | The package may ship `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, embedded archives or native code the analysis cannot read. The administrator sees and must accept opaque code. Required for a `go` plugin. |
| `filesystem` | Declared intent to touch the filesystem beyond the plugin's directory. Recorded and shown to the administrator; the sandbox grants no file access outside the plugin's directory whatever is declared. |
| `media.read` | `GetMediaItem` and `ListMediaItems` on the host: the full media record. |
| `media.write` | Reserved for the metadata write calls, not served today. |
| `api.library` | The same media read calls as `media.read`. |
| `api.user` | Reserved for `GetUserPreferences`, not served today. |
| `api.metadata` | Reserved for `UpdateMetadata`, not served today. |
| `notification.sender` | Reserved for `SendNotification`, not served today. |
| `public` | The interface may read a safe projection of media items (id, title, type, year, overview, poster), never paths or streams. See [`evey.media.*`](api-reference.md#eveymedia-permission-public). |
| `prefs` | The interface may read and write the current user's preferences in the plugin's own namespace. See [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs). |
| `hardware.gpu` | The process gets the host's DRM render node (the real `renderD1xx`, never a card node). A kernel-driver attack surface and a GPU-memory boundary shared with the transcoder: a hung GPU can stall transcoding. Without it, GPU nodes are excluded from the Landlock allowlist entirely; with it, the launcher adds an `IOCTL_DEV` rule scoped to that one file (Landlock ABI 5 or later; on older kernels, read and write access alone permits command submission). The preview shows a critical warning. Grant it only to plugins you would run outside a sandbox. |

## External fetch rules

Declared in `external_fetch: [...]` and checked at install
(`validateExternalFetch`). All-or-nothing: one invalid entry refuses the
whole install.

An entry is a **bare hostname**. A scheme prefix is tolerated and discarded;
only the hostname is kept.

| Entry | Valid | Reason |
|---|---|---|
| `api.tmdb.org` | Yes | Exact hostname, the normal form |
| `https://api.tmdb.org` | Yes | Scheme parsed and dropped, hostname kept |
| `*.tmdb.org` | No | Wildcards are refused: list every subdomain you need |
| `*` | No | Same rule |
| `192.168.1.1`, `::1` | No | IP literals are refused, both families |

At run time the host broker adds its own limits, which the manifest cannot
widen: `http` and `https` only, `GET` and `HEAD` only, the target host matched
case-insensitively against your list (a `:port` suffix in an entry is
ignored), private or reserved addresses refused after DNS resolution, a body
capped at 8 MiB and a request at 20 seconds. Redirects are re-checked against
both the list and the address rule.

The same host list is the `connect-src` of the plugin's frame, prefixed with
`https://`: a widget may call those hosts directly, over `https` only.

## Slot whitelist

Plugins may inject UI only in explicitly allowed slots. Critical paths are
intentionally absent.

| Slot | Auth required | Notes |
|---|---|---|
| `home.section` | Yes | Main page content area |
| `home.hero` | Yes | Hero banner area |
| `library.sidebar` | Yes | Library view sidebar |
| `media.detail.sidebar` | Yes | Media detail sidebar |
| `player.overlay` | Yes | Player overlay UI |
| `settings.user.appearance` | Yes | Appearance section in user settings |
| `sidebar.bottom` | Yes | Bottom area of the navigation tray |
| `admin.nav` | Yes | One nav entry in the admin panel opening the plugin's own sandboxed page; it does not inject into existing admin screens |

**Never allowed:** `admin.*` (except `admin.nav`), `login.*`, `setup.*`,
`profile.security.*`. These paths are authentication- or privilege-critical,
and injection there is a privilege-escalation or phishing vector.

`admin.nav` content is sandboxed like any other slot: it renders in an
`<iframe sandbox="allow-scripts">` without `allow-same-origin`, in a page
whose policy is `default-src 'none'`, so it is third-party content displayed
in the admin area, not an admin capability granted to the plugin.

The bridge limits what a frame can ask of the page: `evey.toast` at most 3
messages per second, 200 characters, no HTML; `evey.navigate` only `/home`,
`/library`, `/media`, `/player`, `/settings` and `/search`; `evey.resize`
clamped to 50-800 px. The frame holds no token: the host mints a five-minute
token scoped to (plugin, user) for the `public` and `prefs` calls, and the
server refuses by default every method that token does not name.

## Update workflow

1. The administrator uploads a new ZIP. `PreviewPluginUpdate` runs the static
   analysis and the signature check and returns the difference in
   permissions, capabilities, slots and `external_fetch` against the installed
   version.
2. If the difference is not empty, the administrator must acknowledge it
   before the update is applied (`requires_ack = true`; through the upload
   route, `accept_permission_changes=true`).
3. The hash of the package is captured at preview and verified at apply: the
   ZIP that was analysed is the ZIP that gets installed.
4. Under `EVEY_PLUGINS_COSIGN_POLICY=enforce`, a signature failure at the
   apply step blocks the update even if the preview passed.

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
analysis reads comments: docs/SECURITY.md (Static analysis)
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
