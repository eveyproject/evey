# Plugin System

> Writing your first plugin? Jump to **[Plugin developer setup](plugins/dev-setup.md)** — install deps, scaffold, sign, troubleshoot.

## Overview

Evey plugins extend the server with metadata providers, media processing, webhooks, notifications, UI widgets, and CSS themes. Plugins run as isolated subprocesses communicating with the host via gRPC over Unix domain sockets.

### Architecture

```
+------------------+      gRPC (Unix socket)      +------------------+
|   Evey Server    | <--------------------------> |   Plugin Process  |
|                  |   /tmp/evey/plugins/{id}/    |                  |
|  HostService     |   plugin.sock                |  PluginService   |
|  (serves RPCs)   |                              |  (implements)    |
+------------------+                              +------------------+
        |                                                  |
        v                                                  v
+------------------+                              +------------------+
|  Permission      |                              |  SDK stubs       |
|  Checker         |                              |  (Python/Node.js)|
+------------------+                              +------------------+
        |
        v
+------------------+     iframe sandbox           +------------------+
|  Frontend        | <--------------------------> |  Widget JS       |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)     |
+------------------+                              +------------------+
```

**What plugins can do:**

| Capability | Description |
|---|---|
| `metadata.provider` | Provide metadata for media items |
| `media.read` | Read media items, streams, chapters, subtitles |
| `media.write` | Update metadata fields and provider IDs |
| `webhook.receiver` | Receive and handle webhook events |
| `notification.sender` | Send notifications to users |
| `ui.extension` | Render widgets in the frontend via iframe slots |
| `ui.theme` | Inject CSS themes (no subprocess needed) |

## Creating a Plugin

### Manifest (`plugin.yaml`)

Every plugin ZIP must contain a `plugin.yaml` (or `plugin.yml`) at root level.

```yaml
name: my-plugin
version: 1.2.0
description: Short description of what this plugin does
author: Your Name
language: python3       # python3, node, go, rust, css
entrypoint: python3 main.py
min_evey_version: 0.1.0

capabilities:
  - metadata.provider
  - ui.extension

permissions:
  - media.read
  - api.library
  - network

config_schema:
  api_key:
    type: password
    description: API key for the external service
    required: true
  language:
    type: select
    description: Preferred language
    default: en
    options:
      - en
      - fr
      - de
  max_results:
    type: integer
    description: Maximum results per query
    default: 25
    min: 1
    max: 100
  enable_cache:
    type: boolean
    description: Enable response caching
    default: true

ui_widgets:
  - slot: media.detail.sidebar
    src: widget.bundle.js
```

### Required fields

| Field | Description |
|---|---|
| `name` | Unique plugin identifier |
| `version` | Semver (X.Y.Z or X.Y.Z-label) |
| `entrypoint` | Command to start the plugin process |
| `language` | Runtime language identifier |

### Optional fields

| Field | Description |
|---|---|
| `description` | Human-readable description |
| `author` | Plugin author |
| `min_evey_version` | Minimum compatible Evey version |
| `capabilities` | List of declared capabilities |
| `permissions` | List of required permissions |
| `config_schema` | Map of config field definitions |
| `ui_widgets` | List of UI widget declarations |

### Supported Languages

The `language:` field is informational — the runner doesn't validate it.
What matters is the `entrypoint:` command actually runs. Examples under
`examples/plugins/` cover `python`, `nodejs`, and `css`. Other interpreter
languages (`bash`, `sh`, `lua`, `perl`, `php`, `ruby`) are not bundled as
examples but are supported by the runner — invoke the interpreter directly
in `entrypoint:`. Compiled languages (Go, Rust, etc.) work too but require
the `native-binary` permission (see below).

- **Python** (`python`) — pre-generated gRPC stubs provided via SDK
- **Node.js** (`nodejs`) — proto file loaded at runtime via `@grpc/proto-loader`
- **Shell / interpreter languages** (`bash`, `sh`, `lua`, `perl`, `php`, `ruby`) — supported but no bundled examples; must invoke the interpreter in `entrypoint:`
- **CSS** (`css`) — static theme, no subprocess (runs as `ui.theme` capability)
- **Compiled binaries** (Go / Rust / anything with native code) — require the `native-binary` permission because the static analyzer can't inspect them

### Config Schema Types

| Type | Extra fields |
|---|---|
| `string` | `min`, `max` (length), `default` |
| `text` | Multi-line string. `min`, `max` |
| `password` | Masked input. `min`, `max` |
| `boolean` | `default` (true/false) |
| `integer` | `min`, `max`, `default` |
| `number` | `min`, `max`, `default` |
| `select` | `options` (list of values), `default` |

All types support: `description`, `required`, `default`.

### UI Widgets

Widgets are JavaScript bundles rendered inside sandboxed iframes. Declare them in `ui_widgets`:

```yaml
ui_widgets:
  - slot: home.section
    src: widget.bundle.js
```

The widget JS is served at `GET /api/v1/plugins/{id}/widget.js` (requires auth). Inside the iframe, the `window.evey` bridge is injected automatically.

### CSS Themes

Themes use `language: css` and capability `ui.theme`. Place a `theme.css` file in the ZIP root. The CSS is sanitized at install time and again at serve time. Theme CSS is loaded into the page as a `<link>` stylesheet.

```yaml
name: my-dark-theme
version: 1.0.0
language: css
entrypoint: theme.css
capabilities:
  - ui.theme
```

The theme overrides CSS custom properties (variables) on `:root`. All `:root` variables from the host app are forwarded into widget iframes.

**Allowed:** standard CSS properties, custom properties, selectors, media queries, animations.

**Blocked by sanitizer:**
- `@import` -- removed entirely
- `url()` with external URLs (`http://`, `//`) -- replaced with `none`
- `url()` with `javascript:` -- replaced with `none`
- `expression()` -- replaced with `none`
- `-moz-binding` -- removed
- `behavior` -- removed

## Plugin Versioning

### Semver requirement

Versions must match `X.Y.Z` or `X.Y.Z-label` (e.g. `1.2.3`, `2.0.0-beta`). The manifest parser rejects invalid versions.

### SHA256 hash verification

Every plugin ZIP is hashed with SHA256 at install time. The hash is stored in the database (`source_hash` field) and returned in the preview endpoint. Admins can verify integrity by comparing the hash against the original distribution:

```bash
sha256sum my-plugin.zip
```

### Cosign signature verification

Evey supports optional [Sigstore cosign](https://docs.sigstore.dev/cosign/overview/) key-based signature verification for plugin ZIPs. Three policies are available, set via environment variables:

| Variable | Description |
|---|---|
| `EVEY_PLUGINS_COSIGN_POLICY` | `off` (default) \| `warn` \| `enforce` |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | Path to PEM public key file (PKIX format) |

**Policies:**
- `off` — signature check is skipped entirely (default)
- `warn` — verification runs; failure is logged but install proceeds
- `enforce` — install is rejected if the signature is missing or invalid

#### Generating a key pair

```bash
cosign generate-key-pair          # produces cosign.key (private) + cosign.pub (public)
# or with the bundled Go generator:
go run scripts/gen_cosign_fixtures.go   # writes to tests/plugins/cosign/
```

Keep the private key (`cosign.key`) secret. Distribute `cosign.pub` to anyone who needs to verify. Point `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` at the public key file on the server.

#### Signing a plugin ZIP

```bash
cosign sign-blob --key cosign.key --output-signature my-plugin.zip.sig my-plugin.zip
```

The signature file (`my-plugin.zip.sig`) is a base64-encoded ASN.1 ECDSA signature over `SHA256(zip)`.

#### Installing a signed plugin

**Via admin UI:** Upload the `.zip` in the plugin file field and the `.zip.sig` in the signature field (optional input shown below the ZIP picker).

**Via API upload:**
```bash
curl -X POST /api/v1/admin/plugins/upload \
  -H "Authorization: Bearer <token>" \
  -F "plugin=@my-plugin.zip" \
  -F "sig=@my-plugin.zip.sig"
```

When `EVEY_PLUGINS_COSIGN_POLICY=enforce` and no `.sig` is provided (or the signature doesn't match), the install is rejected.

#### Notes

- Evey calls `cosign verify-blob --key <pub> --signature <sig> --insecure-ignore-tlog <zip>`. The `--insecure-ignore-tlog` flag is required for key-based local verification — it skips the Rekor transparency log, which is only relevant for keyless/OIDC signing flows.
- `cosign` must be in `$PATH` on the server. If it is not found and policy is `enforce`, the install is blocked. Under `warn`, a warning is logged.
- Test fixtures (valid, tampered, wrong-key) are in `tests/plugins/cosign/`. Regenerate them with `go run scripts/gen_cosign_fixtures.go` (overwrites everything except the committed public key).

### Update flow

Upload a ZIP with the **same `name`** and a **higher semver version**. The installer:
1. Compares versions -- rejects if new version <= current
2. Uninstalls the old version (stops process, removes files, deletes DB record)
3. Installs the new version from scratch

## Security Model

### Backend process isolation

The isolation strategy is selected by `EVEY_PLUGINS_SANDBOX_MODE` (default `auto`):

| Mode | Filesystem isolation | Needs | Where it fits |
|------|----------------------|-------|---------------|
| `auto` | best available | — | **default**: probes at boot and picks `bubblewrap` > `landlock` > `basic` |
| `landlock` | Landlock LSM allowlist + `LD_PRELOAD` seccomp | kernel ≥5.13 with Landlock in the active LSM list | the official Docker image: runs under Docker's **default** seccomp+AppArmor, no host changes |
| `bubblewrap` | mount-namespace pivot | `bwrap` binary + the `mount` syscall | _advanced_ — bare-metal / relaxed-security hosts (see below) |
| `strict` | bubblewrap + `LD_PRELOAD` seccomp | as bubblewrap | _advanced_ — hardened bare-metal |
| `basic` | none (per-path) | — | last-resort fallback for old locked-down kernels |

In practice, every official deployment is the prebuilt Docker image, where `auto` resolves to `landlock`. The `bubblewrap`/`strict` modes are kept for advanced/bare-metal setups (see *Advanced: bubblewrap mode* below); `auto` selects bubblewrap automatically **only** if a throwaway sandbox actually builds on the host (it never does under Docker's default AppArmor, so it falls through to Landlock).

0. **Landlock filesystem allowlist** (modes `landlock`/`auto`) -- a tiny pre-exec shim, `evey-plugin-launcher`, applies a [Landlock](https://docs.kernel.org/userspace-api/landlock.html) ruleset before exec'ing the plugin: read+execute on system directories (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`), read+write on the plugin's own directory, its socket directory and the container's pseudo-devices (`/dev` — `/dev/null`, `/dev/urandom`…), and **everything else denied** -- `/data` (media, JWT key, DB socket) and sibling plugin directories are unreachable. Landlock is enforced by the kernel *before* DAC, so it confines the plugin even when it shares the server's UID. On Landlock ABI v4 (kernel ≥6.7) the launcher also governs **TCP egress**: a plugin without the `network` permission is denied all TCP connect/bind, and one with it may connect only to ports 80/443 plus any explicit `:port` in `external_fetch` (filtered by port, not host). It needs no mount/unshare, so it works under Docker's baseline container security with no relaxed seccomp profile, no added capability, and no privileged mode. The launcher refuses to start (`-require-landlock`) rather than run a plugin unconfined.

1. **Seccomp BPF** -- A shared library (`libevey-sandbox.so`) is injected via `LD_PRELOAD`. The BPF filter blocks `execve`/`execveat` (no child processes) plus `setns`, `bpf`, `kexec_*`, `finit_module`, `keyctl`, `add_key`, `process_vm_readv`/`writev` and `perf_event_open`, returning `EPERM`. It is applied in `basic`, `strict` **and `landlock`** modes (Landlock confines the filesystem/network; this denylist closes the syscall/process-escape vectors Landlock does not cover). Plugins with the `subprocess` permission skip this filter.

2. **Privilege dropping** -- When Evey runs as root, plugin processes are started as `evey-plugin` (UID/GID 1001) via `SysProcAttr.Credential`. The `PR_SET_NO_NEW_PRIVS` flag prevents privilege escalation.

3. **Process group isolation** -- Each plugin runs in its own process group (`Setpgid: true`) with `Pdeathsig: SIGTERM` so it dies if the host crashes.

4. **Socket isolation** -- Each plugin gets its own socket directory at `/tmp/evey/plugins/{id}/`, chowned to the plugin user.

5. **Entrypoint validation** -- Resolved via `EvalSymlinks`, checked against allowed plugin directories, must be a file with execute permission, path traversal (`..`) is rejected.

6. **Minimal environment** -- Plugins receive a stripped `PATH` and only Evey-specific env vars (`EVEY_PLUGIN_ID`, `EVEY_PLUGIN_SOCK`, `EVEY_HOST_SOCK`). `HOME`/`TMPDIR` point at `/tmp` for namespace modes, or at the plugin's own directory under `landlock` (the only writable location on the allowlist).

### Advanced: bubblewrap mode

`bubblewrap` (and `strict`) wrap the plugin in full Linux namespaces (user, pid,
uts, ipc, net, mount) via a `pivot_root` into a private filesystem view — a
stronger, "see nothing" form of isolation than Landlock's "see-but-deny". It is
**not** the default and most operators never need it; Landlock already confines
filesystem access on any modern kernel with no host changes. Reach for it only
if you run the server **bare-metal from source** (not the Docker image) on a
host where unprivileged user namespaces are enabled, and you want namespace-level
isolation (separate PID/network views).

Requirements and trade-offs:

- The `bwrap` binary must be in `$PATH` (the image bundles it; bare-metal:
  `apt install bubblewrap` / `apk add bubblewrap`).
- bwrap needs the `mount`/`pivot_root` syscalls. **Inside Docker these are
  blocked by the default AppArmor profile** (`docker-default`). Making bwrap work
  in a container therefore requires the bundled relaxed seccomp profile, which
  re-opens `mount`/`unshare` — a deliberate **relaxation of Docker's baseline
  container security**. This is exactly what `landlock` avoids; do not enable it
  unless you understand and accept that trade-off.

To opt in bare-metal:

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap   # or "strict" to add the LD_PRELOAD seccomp filter
```

To opt in inside Docker (relaxes container security — not recommended):

```yaml
# docker-compose.yml, under the evey service:
environment:
  EVEY_PLUGINS_SANDBOX_MODE: bubblewrap
security_opt:
  - seccomp=./docker/seccomp-evey.json   # Docker default + mount/unshare re-opened
```

### Permission model

Permissions are declared in the manifest and enforced at runtime via a gRPC interceptor on the HostService. Each RPC method maps to one or more required permissions. The `PermissionChecker` caches permissions in memory and verifies them on every call.

### Static analysis

Before installation, the analyzer scans all non-binary files for suspicious patterns:

**Critical (blocks install):**
- Shell execution (`os.system`, `subprocess`, `exec()`, `child_process`, PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby backtick/`Kernel.exec`/`open('|cmd')`)
- Reverse/bind shells
- Sensitive file access (`/etc/passwd`, `/etc/shadow`, `.ssh/`)
- Dynamic code execution (`eval()`, `exec()`, `compile()`)
- Privilege escalation (`setuid`, `chmod 4777`, `sudo`)
- Crypto mining indicators (`stratum+tcp`, `xmrig`, `coinhive`)
- Runtime package installation (`pip install`, `npm install`, `curl | sh`)
- Dynamic imports / sandbox escape (`__import__`, `vm.runIn`, `new Function`, Python `__subclasses__()`/`__class__.__bases__`/`__mro__`)
- Unsafe deserialization (`pickle.loads`, `marshal.loads`, `__reduce__`/`__reduce_ex__`)
- Native code execution (`ctypes.CDLL`, `ffi.Library`)
- Obfuscated shell payloads (shell `base64 -d | bash`, `echo <blob> | openssl`)
- PHP obfuscation (`assert(base64_decode(...))`)
- String concatenation evasion (obfuscated imports)
- Signals to PID 1

**Warning (logged but allowed):**
- Outbound network connections (`requests.get`, `fetch`, `net.Dial`)
- Recursive file deletion
- Symlink creation
- Base64 decoding
- Hex-encoded strings
- Unvendored dependency manifests
- Non-HTTP URL schemes (`wss://`, `ssh://`, `ftp://`)
- JWT/private key file references, `.env.*` references
- `yaml.load()` without explicit Loader (Python deserialization risk)
- PHP `str_rot13()` and `chr()` concatenation obfuscation
- Go `unsafe.Pointer` / `unsafe.Slice` / `unsafe.SliceData` / `unsafe.StringData`

The analyzer also extracts all URLs (literal, template, concatenated) for audit.

### CSS sanitizer

Theme CSS is sanitized both at install time and at serve time. See "Blocked by sanitizer" in the CSS Themes section above.

### Frontend iframe sandbox

UI widgets run inside `<iframe sandbox="allow-scripts">`. This means:
- No access to parent DOM
- No navigation of the top frame
- No form submission
- No popups
- No same-origin access
- Scripts are allowed (needed for widget logic)

Communication uses `postMessage` via the bridge API.

### Trusted mode

Plugins that declare the `subprocess` permission skip the seccomp BPF filter (`LD_PRELOAD` is not set). This is an escape hatch for plugins that need to spawn child processes (e.g., calling FFmpeg). Use with caution -- only install trusted plugins with this permission.

### Known limits of the sandbox

The sandbox is defense-in-depth, not a hard security boundary. Each layer is bypassable on its own; the combination raises the cost of a malicious plugin but does not eliminate it. Admins installing third-party plugins **must review the code manually** before approval.

- **Static analyzer (regex-based)** -- pattern matching on source text. Bypassable by obfuscation: string concatenation (`"o"+"s"+".system"`), `getattr(__builtins__, "e"+"xec")`, base64/hex-encoded payloads decoded at runtime, unicode homoglyphs, bytecode loaded via `marshal`/`dis`, or code fetched from the network after install (when `network` permission is granted).
- **Seccomp BPF filter** -- blocklist, not allowlist. It blocks `execve`/`execveat` only. Other syscalls (including `ptrace`, `clone`, raw socket ops, filesystem writes in the plugin dir) pass through. It does **not** contain a compromised process that stays within allowed syscalls.
- **`LD_PRELOAD` injection** -- only effective against dynamically-linked interpreters (Python, Node). A statically-linked Go or Rust binary, or any process ignoring `LD_PRELOAD` (via `musl` static build, or re-exec without the env var), is **not** subject to the seccomp filter even without the `subprocess` permission.
- **UID drop (`evey-plugin`, UID 1001)** -- effective against host-FS writes outside the plugin dir, but only applies when Evey runs as root. Note that under `landlock` mode the filesystem allowlist denies everything outside the plugin's directory regardless of UID or world-readability, so it does not rely on UID separation.
- **Landlock filesystem allowlist** -- strong on kernels that enforce it, but it is a *filesystem* boundary only: it does not restrict syscalls (that's the seccomp layer), nor network, nor inter-process visibility within a shared PID namespace. It also requires kernel ≥5.13 with Landlock in the active LSM list; older hosts fall back to weaker modes (which `auto` logs at boot).
- **Process group / `Pdeathsig`** -- cleanup, not confinement. It prevents zombie processes, not exfiltration or resource abuse during the plugin's lifetime.
- **Permission checker (gRPC interceptor)** -- only gates HostService RPCs. A plugin granted `network` can contact any external host; there is no egress filter. A plugin without `network` can still read anything the host provides via RPC (respecting declared permissions).
- **Network egress is port-based, not host-based** -- under `landlock` on Landlock ABI v4 (kernel ≥6.7), TCP is denied by default: a plugin without the `network` permission cannot `connect()` at all, and one with it may only connect to ports 80/443 plus any explicit `:port` in its `external_fetch` list (it can never `bind`/listen). Landlock filters by **port, not host**, so the per-host `external_fetch` allowlist is *not* enforced by this layer (a `network` plugin can still reach any host on an allowed port); host filtering would need a DNS/proxy layer. On kernels below v4 the TCP filter silently degrades to a no-op (filesystem confinement still applies). UDP is not covered.
- **Resource limits are best-effort** -- per-plugin CPU/RAM/PIDs are capped via cgroup v2 when the host exposes a writable cgroup tree (default 256 MiB / 50% CPU / 64 PIDs); when cgroup v2 is unavailable the runner warns and runs without limits.
- **CSS sanitizer** -- conservative, but browsers evolve. New CSS features and URL schemes may slip through until the sanitizer is updated.
- **iframe sandbox (`allow-scripts`)** -- same-origin is disabled, but the iframe shares the browser process with the host page on some engines; speculative-execution side channels are not mitigated.

### Recommended review checklist for third-party plugins

Before installing a plugin you did not write:

1. Verify the SHA256 hash against the source you trust.
2. Read `plugin.yaml` -- check `permissions` and `capabilities` match the described function. A theme plugin does not need `network` or `subprocess`.
3. Read the source. Look for: dynamic `import`/`eval`, string-concatenated syscalls, base64 blobs, runtime downloads, unusual `Initialize` behavior.
4. Prefer plugins that vendor their dependencies (no `pip install`/`npm install` at runtime).
5. Reject any plugin requesting `subprocess` unless you audited the spawned commands.
6. Run the plugin in a staging install first, watch logs and outbound connections.

## Host API Reference

Plugins call HostService RPCs via gRPC over Unix socket (`EVEY_HOST_SOCK`). The host authenticates the caller via `SO_PEERCRED` on the Unix socket — the kernel-supplied PID is matched against the PID the runner spawned for that plugin. Each plugin gets its own dedicated host socket (`/tmp/evey/plugins/<id>/host.sock`); a plugin trying to connect to another plugin's socket is rejected with `Unauthenticated: PID mismatch`. No `x-plugin-id` header is required or honored.

### Media RPCs

| RPC | Description | Required permission |
|---|---|---|
| `GetMediaItem` | Get a single media item by ID | `api.library` or `media.read` |
| `ListMediaItems` | List media items (filter by library, type, parent) | `api.library` or `media.read` |
| `SearchMedia` | Full-text search across media | `api.library` or `media.read` |
| `GetMediaStreams` | Get video/audio/subtitle streams for an item | `api.library` or `media.read` |
| `GetChapters` | Get chapters for a media item | `api.library` or `media.read` |
| `GetSubtitles` | Get subtitle tracks for a media item | `api.library` or `media.read` |

### Metadata RPCs

| RPC | Description | Required permission |
|---|---|---|
| `UpdateMetadata` | Update fields and provider IDs on a media item | `api.metadata` or `media.write` |

### User RPCs

| RPC | Description | Required permission |
|---|---|---|
| `GetUserPreferences` | Get user/profile preferences | `api.user` |

### Notification RPCs

| RPC | Description | Required permission |
|---|---|---|
| `SendNotification` | Send a notification to a user (title, body, url, image) | `notification.sender` |

### Always-allowed RPCs (no permission check)

| RPC | Description |
|---|---|
| `EmitEvent` | Emit a custom event to the host event bus |
| `Log` | Write a log entry (debug, info, warn, error) |
| `GetConfig` | Read a plugin config value by key |
| `SetConfig` | Write a plugin config value |

### Host-to-Plugin RPCs (PluginService)

These are implemented by the plugin and called by the host:

| RPC | Description |
|---|---|
| `Initialize` | Called after process start with plugin ID, config, host version |
| `Shutdown` | Called before stopping the plugin process |
| `OnEvent` | Delivers events the plugin subscribed to |
| `HealthCheck` | Periodic health check |
| `GetCapabilities` | Query plugin capabilities and API version |

## Installation

### Enabling the plugin subsystem

The plugin system is **on by default**. Plugins always run inside a sandbox
(by default `auto`, which selects the strongest the host supports -- see
*Backend process isolation* above) and installing is admin-only and gated by
the static analyzer, cosign and a permission-diff acknowledgement. To turn the
subsystem off entirely, set
`EVEY_PLUGINS_ENABLED=false` — then all `AdminPluginService` RPCs return
`FailedPrecondition` and the runner is not constructed (no ambient surface).

### Upload via admin UI

`POST /api/v1/admin/plugins/upload` (admin-only, multipart form, field `plugin`).

Max upload size: 100 MB.

### Install from URL

The install endpoint also accepts an HTTPS URL as the `source` field instead of a file upload:

```
POST /api/v1/admin/plugins/install
Content-Type: application/json
{ "source": "https://example.com/my-plugin-1.2.0.zip#sha256:abc123…" }
```

The `#sha256:<hex>` fragment is **optional but strongly recommended**. When present, the installer verifies the downloaded ZIP matches the expected digest before extracting. If the hashes differ the install is aborted with an error.

| Behaviour | Detail |
|---|---|
| Max download size | 100 MB — larger responses are rejected |
| Context propagation | The HTTP request inherits the admin request context (cancelled if the admin disconnects) |
| Temp file | Written to a system temp dir; deleted automatically after install (success or failure) |
| Hash fragment | `#sha256:<64-hex-chars>` — stripped from the URL before the request is sent |
| Local path | A local file path (no scheme) passes through unchanged — hash fragment is also accepted |

### ZIP extraction limits

The installer rejects archives that try to exhaust resources:

| Limit | Value | Rationale |
|---|---|---|
| File count | 5 000 entries | Blocks "many small files" DoS |
| Total uncompressed size | 500 MB | Blocks disk-fill |
| Per-file compression ratio | 100× | Blocks zip bombs |
| File mode | masked to `0o755` | Strips `setuid` / `setgid` / sticky bits on extracted files |
| Symlinks | extracted, but analyzer flags them | Handled in the static analysis report |

### Supported format

ZIP archive with `plugin.yaml` at root level.

### Preview endpoint

`POST /api/v1/admin/plugins/preview` (admin-only, same format as upload).

Returns manifest info + static analysis report without installing. Response includes:
- name, version, description, author, language, entrypoint
- permissions, capabilities
- `source_hash` (SHA256)
- `analysis` object: `safe`, `issues[]`, `total_files_scanned`

### Install sequence

1. Parse `plugin.yaml` from ZIP
2. Check if plugin with same name exists -- if so, require higher version and uninstall old
3. Compute SHA256 hash of ZIP
4. Extract ZIP to `/data/plugins/{plugin-id}/`
5. If `ui.theme` capability: sanitize `theme.css`
6. Copy SDK stubs for the plugin language
7. Chown plugin directory to `evey-plugin` user (if root)
8. Run static analysis -- **block install if any CRITICAL finding**
9. Save plugin record to database
10. Start plugin subprocess (or mark as Running for CSS-only plugins)
11. Connect IPC and call `Initialize` RPC
12. Load permissions into the checker cache
13. Publish `PluginInstalled` event

### Uninstall

1. Stop the plugin process (gRPC `Shutdown` then SIGTERM, fallback SIGKILL after 5s)
2. Invalidate cached permissions
3. Remove plugin directory
4. Delete database record
5. Publish `PluginUninstalled` event

## Configuration

### Config schema in manifest

Define config fields under `config_schema` in `plugin.yaml`. Each key is a config field name, the value is an object with `type`, `description`, `required`, `default`, and type-specific constraints.

### Dynamic forms in admin UI

The admin UI renders a form based on the config schema. Field types map to input components (text input, password, checkbox, number input, select dropdown, textarea).

### Getting/setting config via Host API

From the plugin process:

```
GetConfig(key="api_key") -> { value: "sk-xxx", found: true }
SetConfig(key="cache_ttl", value="3600") -> {}
```

Config is stored per-plugin in the database and passed to the plugin at `Initialize`.

## Available Plugin Slots

| Slot name | Location | Description |
|---|---|---|
| `home.section` | Home page | Section below existing content |
| `player.overlay` | Video player | Overlay on top of the player |
| `sidebar.bottom` | Sidebar | Widget at sidebar bottom |
| `media.detail.sidebar` | Media detail page | Sidebar widget on detail view |

Usage in frontend: `<PluginSlot name="home.section" context={{ mediaId }} />`

The `PluginSlot` component queries the plugin list, finds frontend modules matching the slot name, and renders each in a sandboxed `PluginIframe`.

### Forbidden slots

The installer rejects any manifest whose `frontend[].slot` targets a privileged area. These are off-limits by design — a compromised plugin must never reach admin tooling, the auth flow, the initial setup wizard, or per-profile security settings:

- `admin.*`
- `login.*` and `auth.*`
- `setup.*`
- `profile.security.*`

The single source of truth for the allow-list is `internal/domain/plugin/slots.go`. Any new slot must be declared there before it can be used in a manifest.

## SDK

### Available SDKs

| Language | Directory | Contents |
|---|---|---|
| Python | `sdk/python/` | `plugin_api_pb2.py`, `plugin_api_pb2_grpc.py` (pre-generated stubs) |
| Node.js | `sdk/nodejs/` | `plugin_api.proto` (loaded at runtime via `@grpc/proto-loader`) |
| Dart | `sdk/dart/` | Proto stubs |
| Java | `sdk/java/` | Proto stubs |
| PHP | `sdk/php/` | Proto stubs |
| Ruby | `sdk/ruby/` | Proto stubs |
| Swift | `sdk/swift/` | Proto stubs |

SDK stubs are auto-copied into the plugin directory at install time.

### Python example

```python
import grpc
import plugin_api_pb2 as pb
import plugin_api_pb2_grpc as rpc
import os

sock = os.environ["EVEY_HOST_SOCK"]
channel = grpc.insecure_channel(f"unix://{sock}")
host = rpc.HostServiceStub(channel)

resp = host.GetMediaItem(pb.GetMediaItemRequest(id="mid-123"))
print(resp.title)
```

### Node.js example

```js
const protoLoader = require("@grpc/proto-loader");
const grpc = require("@grpc/grpc-js");
const pkgDef = protoLoader.loadSync("plugin_api.proto", { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const sock = process.env.EVEY_HOST_SOCK;
const client = new proto.HostService(`unix://${sock}`, grpc.credentials.createInsecure());

client.GetMediaItem({ id: "mid-123" }, (err, resp) => {
  console.log(resp.title);
});
```

### Widget bridge API (frontend)

Inside widget iframes, `window.evey` is available:

```typescript
interface EveyBridge {
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  resize(height: number): void;
  onInit(callback: (data: { context: Record<string, unknown>; locale: string; theme: string }) => void): void;
}
```

**Lifecycle:**
1. Widget loads, bridge script sends `ready` message to parent
2. Parent responds with `init` message containing `context`, `locale`, `theme`
3. Widget calls `window.evey.onInit(cb)` to receive init data

**Example widget:**

```js
window.evey.onInit(({ context, locale, theme }) => {
  const div = document.createElement("div");
  div.textContent = `Media: ${context.mediaId}, Theme: ${theme}`;
  document.body.appendChild(div);
  window.evey.resize(div.offsetHeight);
});
```

### Known Capabilities

`metadata.provider`, `media.read`, `media.write`, `webhook.receiver`, `notification.sender`, `ui.extension`, `ui.theme`

### Examples

Working plugins covering every capability live under
`examples/plugins/`. Pre-built uploadable ZIPs are in `tests/plugins/`
— zip names mirror the example directory name
(`examples/plugins/<foo>/` → `tests/plugins/<foo>.zip`). Rebuild the
zips after editing an example with:

```bash
make plugin-zips
```

The script is at `scripts/build-test-plugins.sh`. Security-test and
malicious-theme ZIPs under `tests/plugins/` have no example source;
they're hand-built fixtures kept for negative testing.

### Known Permissions

`network`, `filesystem`, `media.read`, `media.write`, `api.library`, `api.user`, `api.metadata`, `notification.sender`, `subprocess`, `native-binary`

The `native-binary` permission is required when the ZIP contains any
compiled executable, shared library, or embedded archive — anything the
static analyzer can't inspect. Without it, install is blocked with
`plugin blocked: bundles compiled binaries or archives`. This is an
explicit opt-in so the admin sees the elevated risk at preview time.
