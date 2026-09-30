# Permissions and slots

This page is for plugin authors: what a manifest may declare, what is checked
when a plugin is installed, and the rules that apply to it at run time. How
the server contains a plugin, and what that containment cannot stop, is
described in [Security model and plugin sandbox](../SECURITY.md).

## What is checked at install

A plugin is refused at install when:

- `kind` is missing, or is neither `plugin` nor `theme`;
- a `kind: plugin` lacks `language` or `entrypoint`, or a `kind: theme`
  declares them or declares `permissions`;
- a permission or capability is unknown;
- a frontend module targets a slot outside the [slot list](#slot-whitelist);
- an `external_fetch` entry breaks the [fetch rules](#external-fetch-rules);
- the ZIP breaks the extraction limits: more than 5,000 files, a file over
  100 MB, more than 500 MB uncompressed, a compression ratio above 100:1, a path
  that leaves the archive, or a symbolic link;
- the static analysis finds a critical pattern (shell execution, obfuscation,
  dynamic import, sandbox escape, privilege change, mining...). Compiled
  binaries and embedded archives are refused unless the manifest declares
  `native-binary`.

The analysis also reads comments: a `# pip install foo` line in a comment is
reported like real code. Remove such lines before publishing. The full list of
categories is in [Security model and plugin sandbox](../SECURITY.md#static-analysis).

Before installing, the administrator sees the declared permissions, slots and
hosts, and what the analysis found.

## What the plugin runs under

Whatever the manifest declares:

- the plugin sees its own directory, not the server's data;
- it has no direct network access: a plugin with the `network` permission
  reaches the hosts of its `external_fetch` list through the host's `Fetch`
  call;
- without `subprocess`, it cannot start child processes (threads work);
- it runs under resource limits the administrator can adjust: 256 MiB of
  memory, 64 processes and threads, 100 MiB of disk for its directory, and a
  lower CPU priority by default.

Signing is optional during development. How to sign a plugin, and the
signature policy a server can enforce: [Writing a plugin](dev-setup.md).

## Permissions catalog

A permission is declared under `permissions`; any other value refuses the install. Receiving webhooks (`webhook.receiver`) and providing metadata (`metadata.provider`) are capabilities, declared under `capabilities`: see [Plugin API reference](api-reference.md#capabilities).

| Permission | What it grants |
|---|---|
| `network` | Plugin may make outbound network connections. **Must** be combined with a non-empty `external_fetch` list - refused at install without it. |
| `subprocess` | Plugin may spawn child processes. Without it, two layers apply: the LD_PRELOAD seccomp shim blocks `execve`/`execveat` for interpreted plugins, and the launcher's `-deny-fork` blocks process creation itself (`fork`/`vfork`/`clone` without `CLONE_THREAD`) in the kernel. Threads keep working either way. (The launcher's kernel seccomp does not deny `execve` - it must exec the plugin - but any child inherits the kernel filter + the Landlock domain.) |
| `native-binary` | Plugin may ship `.so`/`.dll`/`.exe`/`.wasm`/`.jar` or embedded archives. Admin sees and must accept the opaque code at install preview. |
| `filesystem` | Declared intent to access the filesystem beyond the plugin's own directory. Recorded and shown to the administrator; the sandbox does not grant file access outside the plugin's directory whatever is declared. |
| `media.read` | Plugin may call `MediaRead` RPCs: read media file metadata and stream access. |
| `media.write` | Plugin may call `MediaWrite` RPCs: create/update media records. |
| `api.library` | Plugin may call library management RPCs: list/search libraries, trigger scans. |
| `api.user` | Plugin may call user-scoped RPCs (read own profile, preferences). |
| `api.metadata` | Plugin may call metadata RPCs: search and write enrichment data. |
| `notification.sender` | Plugin may publish notification events (web push, email, webhook). |
| `public` | The plugin's interface may read a safe projection of media items (id, title, type, year, overview, poster), never paths or streams. See [Plugin API reference](api-reference.md#public). |
| `prefs` | The plugin's interface may read and write the current user's preferences in the plugin's own namespace. See [Plugin API reference](api-reference.md#prefs). |
| `hardware.gpu` | **Highest-risk permission - admin-only, first-party tier.** Grants the host's DRM render node (`-render-dev`, the real `renderD1xx`, never a card node, never `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`): a kernel-driver attack surface and a GPU-memory boundary SHARED with the transcoder (cross-process memory leak possible on AMD/Apple/Qualcomm GPUs; a hung GPU can stall transcoding). Without this permission, GPU device nodes are excluded from the Landlock device allowlist entirely (denied regardless of DAC). With it granted, the launcher adds an explicit `IOCTL_DEV` Landlock rule scoped to that one file (ABI v5+); on older Landlock ABIs, ioctls aren't governed by Landlock at all, so read+write access alone already permits GPU command submission. The installer surfaces a loud CRITICAL warning at preview; grant only to trusted, first-party plugins. |

## External fetch rules

Declared in `external_fetch: [...]` in the manifest and checked at install by
`validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`).
All-or-nothing: one invalid entry refuses the whole install.

An entry is a **bare hostname**. A scheme prefix is tolerated and discarded -
only the hostname is kept.

| Entry | Valid | Reason |
|---|---|---|
| `api.tmdb.org` | Yes | Exact hostname, the normal form |
| `https://api.tmdb.org` | Yes | Scheme parsed and dropped, hostname kept |
| `*.tmdb.org` | No | Wildcards are refused - list every subdomain you need |
| `*` | No | Same rule |
| `192.168.1.1`, `::1` | No | IP literals are refused, both families |

At run time the host broker adds its own limits, which the manifest cannot
widen: `http` and `https` only, `GET` and `HEAD` only, the target host matched
case-insensitively against your list, and private or reserved addresses refused
after DNS resolution - redirects are re-checked against both rules.

> The broker's matcher also understands a `*.example.com` form, but the
> installer refuses such an entry, so it can never reach it. Write explicit
> hostnames.

## Slot whitelist

Plugins may inject UI only in explicitly allowed slots. Critical paths are intentionally absent.

| Slot | Auth required | Notes |
|---|---|---|
| `home.section` | Yes | Main page content area |
| `home.hero` | Yes | Hero banner area |
| `library.sidebar` | Yes | Library view sidebar |
| `media.detail.sidebar` | Yes | Media detail sidebar |
| `player.overlay` | Yes | Player overlay UI |
| `settings.user.appearance` | Yes | Appearance section in user settings |
| `sidebar.bottom` | Yes | Bottom area of the navigation tray |
| `admin.nav` | Yes | One nav entry in the admin panel opening the plugin's own sandboxed page - does not inject into existing admin screens |

**Never allowed:** `admin.*` (except `admin.nav`), `login.*`, `setup.*`, `profile.security.*` - these paths are authentication/privilege-critical and plugin injection there is a privilege-escalation or phishing vector.

`admin.nav` content is still sandboxed like any other slot: it renders inside an `<iframe sandbox="allow-scripts">` (no `allow-same-origin`) with CSP `default-src 'none'`, so it is third-party content displayed in the admin area, not an admin capability granted to the plugin.

Source: `internal/domain/plugin/slots.go`.

Frontend modules run in a sandboxed `<iframe>` with no access to the host
page. The bridge limits what they can do: `evey.toast` at most 3 messages per
second, 200 characters, no HTML; `evey.navigate` refuses `/admin/*`, `/auth/*`,
`/setup/*` and `/profile/security/*`; `evey.resize` is clamped to 50-800 px.

## Update workflow

1. Admin uploads a new ZIP → `PreviewPluginUpdate` runs the static analyzer + cosign check and returns a diff of changed permissions/slots/`external_fetch` vs the installed version.
2. If the diff is non-empty (new permissions, new slots, changed external hosts), the admin must explicitly acknowledge before the update is applied (`requires_ack = true`).
3. The source hash is captured at preview and verified at apply (anti-TOCTOU: the ZIP that was analyzed is the ZIP that gets installed).
4. If cosign policy is `enforce`, a signature mismatch at the apply step blocks the update even if the preview passed.
