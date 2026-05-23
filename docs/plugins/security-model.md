# Plugin Security Model

Evey plugins are third-party code that runs as separate OS processes alongside the server. This document explains what a malicious plugin could attempt, how Evey defends against each vector, and the honest limits of the current implementation.

## Threat model

| Threat | Description |
|---|---|
| Server RCE | Plugin escapes its sandbox and executes arbitrary commands as the server process |
| Secret exfiltration | Plugin reads JWT keys, database credentials, or user tokens from the filesystem |
| Horizontal escalation | Plugin impersonates another plugin over IPC to call RPCs it has no permission for |
| Network exfiltration | Plugin opens outbound connections to send captured data to an attacker-controlled host |
| DoS / resource abuse | Plugin exhausts CPU, memory, PIDs, or disk (fork bombs, crypto mining, disk fill) |
| Frontend phishing | Plugin injects a full-screen overlay in the user's browser to steal credentials |
| Supply chain | Plugin author ships obfuscated or compiled code that static analysis cannot inspect |

## Defense layers

### 1. Manifest validation (install-time)

Every `plugin.yaml` is validated before extraction. Install is refused if:

- `kind` is absent or not `plugin`/`theme`
- `kind: plugin` lacks `language` or `entrypoint`
- `kind: theme` declares `language`, `entrypoint`, or `permissions`
- `frontend[].slot` is not in the server-side slot whitelist (see [Slot whitelist](#slot-whitelist))
- `external_fetch` entries violate the host-whitelist rules (see [External fetch rules](#external-fetch-rules))
- Any declared capability or permission is unknown

Source of truth: `internal/domain/plugin/model.go`, `internal/adapters/driven/pluginrunner/capabilities.go`.

### 2. ZIP safe extraction

Before any file is written to disk:

- Entry count cap: 5,000 files maximum
- Per-file size cap: 100 MB
- Total uncompressed size cap: 500 MB
- Compression ratio cap: 100:1 (zip-bomb detection)
- Path traversal check: entries with `..` in the resolved path are rejected
- Symlinks: rejected unconditionally (`ExternalAttrs` Unix type bits checked)
- setuid/setgid/sticky bits: stripped from all extracted files

Source: `internal/adapters/driven/pluginrunner/zip.go`.

### 3. Static analyzer (install-time)

Every text file in the ZIP is scanned by a regex-based multi-language analyzer before the plugin is accepted into the database. A normalize pre-pass defuses common obfuscation (alias resolution, string concat fold, f-string expansion).

| Category | Severity | Examples |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `shell_exec`/`proc_open`/`preg_replace /e`, Ruby backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING/CRITICAL | `requests.get`, `fetch(`, `net.Dial`, reverse/bind shell patterns |
| `filesystem` | WARNING/CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink ops, `/proc/<pid>/environ` |
| `obfuscation` | CRITICAL | `eval(`, `base64.b64decode`, hex sequences, `String.fromCharCode` chains, PHP `chr()` chains, `str_rot13`, `assert(base64_decode(...))` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, `os.kill(1, ...)` |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module`, `getattr(__builtins__, ...)` |
| `sandbox_escape` | CRITICAL | `vm.runInNewContext`, `process.binding`, `new Function(`, Python `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL/WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` without Loader |
| `native_code` | CRITICAL/WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` — opaque to scanner |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` — may contain unanalyzed code |
| `mime_mismatch` | WARNING (treated critical) | file extension claims image/font, content is executable |
| `suspicious_url` | WARNING | non-HTTP scheme in URL literal (`ftp://`, `wss://`, `ssh://`) |
| `unvendored_deps` | WARNING | `requirements.txt`/`package.json` with declared deps but no vendored modules |

CRITICAL findings or `mime_mismatch` set `Safe = false` and block install.

`compiled_binary` and `embedded_archive` block install unless the manifest declares the `native-binary` permission. The admin sees the warning and the permission at the Preview step before accepting.

Source: `internal/adapters/driven/pluginrunner/analyzer.go`, `analyzer_normalize.go`.

#### Best-effort, not enforcement

The analyzer is regex- and pattern-based. It defuses well-known obfuscation classes but **cannot guarantee detection of every adversarial bypass**. Class-level coverage examples:

| Class | Detected | Notes |
|---|---|---|
| PHP variable-callable from string concat | `$f = "sys" . "tem"; $f($cmd)` (single-line) and `$x="sys"; $y="tem"; $f=$x.$y; $f($cmd)` (multi-step, fixed-point resolution) | Up to 5 fixed-point iterations, cycles `$a=$b; $b=$a` terminate cleanly |
| Python builtin lookup via name | `getattr(__builtins__, "exec")` and `__builtins__.__dict__["exec"]` (literal or via resolved variable) | Dict bracket indexer triggers Critical when literal is dangerous |
| JS `Function.constructor` escape | `(fn).constructor("...")()` immediate call and `const F = (fn).constructor; F("...")` (stored ref tracked by binding rewrite) | Calls with variable arg `F(code)` rewritten to `.constructor(code)` and matched by the variable-arg pattern |
| Multi-step fragmented concat | `$a="s"; $b="y"; $c="s"; $d="t"; $e="e"; $f="m"; $g=$a.$b.$c.$d.$e.$f; $g(...)` | Innocent fragments tracked separately from "dangerous" values; the danger check fires on the final concatenated string only |

Each bypass class has a permanent regression test in `tests/fixtures/plugins/malicious/`. Adding a new class requires: (1) fixture committed, (2) analyzer rule or normalize-pass extended, (3) `installer_security_test.go` flips `workerANeeded:true → false` once detection is in.

**Out of scope (sandbox enforces):** AST-level dynamic transforms — `strrev("metsys")`, `base64.b64decode(...)`, `eval(open("/encoded").read())`, runtime regex.replace. The regex analyzer cannot resolve these; the runtime sandbox (the Landlock allowlist or the bwrap mount whitelist, plus the kernel seccomp denylist) + userspace resource monitor + IPC peer-cred guarantee the plugin cannot escape, exfiltrate `/data`, or escalate beyond declared permissions.

**Known false positives:** the analyzer matches inside source comments and docstrings. A `# pip install foo` line in a comment will trip the `runtime_install` rule. Acceptable: instructing operators to strip such comments before publishing is cheaper than per-language comment stripping in the regex analyzer; the bwrap mount whitelist + userspace resource monitor make the runtime block independent.

#### Empirical runtime confirmation

The mount whitelist was verified end-to-end against a running stack via a canary plugin (`tests/fixtures/plugins/canary-sandbox-runtime/`) that probes the sandbox view at startup:

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```

`/data` itself is mounted but contains only the plugin's own subdirectory (`/data/plugins`); no `jwt.key`, `backups`, `artworks` are visible. `/proc/1/environ` IS readable but PID 1 inside the namespace is the plugin itself (`--unshare-pid`), so reading its own environ is not a leak.

Re-run the canary after any change to `runner_bwrap.go` to confirm the mount layout still blocks.

**Enforcement boundary is the runtime sandbox (Landlock or bubblewrap) + kernel seccomp + userspace resource monitor + IPC peer-cred, not the analyzer.** A plugin that slips a CRITICAL pattern past the analyzer still cannot read `/data` (Landlock denies it / bwrap never mounts it), call a denied syscall, fork-bomb past the PID cap, or invoke gRPC methods it has no permission for. The analyzer is one layer in defense-in-depth, useful to surface obvious malicious code at preview time and reduce the runtime attack surface — not the perimeter.

### 4. Cosign signature verification

Configurable via `evey.yaml` key `cosign_policy`:

| Policy | Behavior |
|---|---|
| `off` (default) | Signature not checked; `Plugin.Signature` is nil |
| `warn` | Verification failure is recorded and shown to admin but does not block install |
| `enforce` | Verification failure blocks install with an error |

Source: `internal/adapters/driven/pluginrunner/cosign.go`.

### 5. CSS sanitizer

Applied to `theme.css` at install time. See [`docs/themes/security-model.md`](../themes/security-model.md) for the full rule set. Sanitization removes or rewrites dangerous constructs rather than rejecting the entire file (warnings are recorded).

### 6. Theme JS scanner

Applied to `theme.js` at install time. A pattern that matches causes install to be refused. See [`docs/themes/security-model.md`](../themes/security-model.md) for the blocked pattern list.

### 7. Runtime sandbox (Landlock / bubblewrap + seccomp + userspace resource monitor)

> **Default is `landlock`** (`sandbox_mode=auto` resolves to it on the official
> Docker image): a Landlock LSM filesystem allowlist + Landlock v4 TCP egress
> filter + the kernel + `LD_PRELOAD` seccomp denylist + `-deny-inet`, all under
> Docker's default seccomp+AppArmor with zero host changes. See
> [`docs/plugins.md`](../plugins.md) for the map and
> [`docs/SECURITY.md`](../SECURITY.md#sandbox-modes) for the full per-mode table.

The confinement layers that apply across **every** mode (subject to host
capability):

- **UID separation** — each plugin drops to uid/gid 1001 (`evey-plugin`),
  distinct from the server's uid 1000, via `setcap cap_setuid,cap_setgid+ep`
  on the (non-root) server. `jwt.key` (`0600`) and `/data/{keys,logs,backups}`
  (`chmod 700`) are then DAC-unreadable to a plugin even without Landlock.
- **Kernel seccomp denylist** — installed by `evey-plugin-launcher` per build
  arch, pre-`execve`, so it constrains **static/compiled plugin binaries too**
  (which never load `libevey-sandbox.so`). Blocks `ptrace`, `mount`, `unshare`,
  `setns`, `bpf`, `kexec`, module load, the setuid family, `capset`, keyring,
  signalling PID 1, etc. Inherited across `execve` into the plugin and its
  children. The `LD_PRELOAD` `libevey-sandbox.so` shim applies the same denylist
  (plus `execve`/`execveat`, so a non-`subprocess` interpreted plugin cannot
  spawn children) to dynamic plugins as belt-and-suspenders.
- **`-deny-inet`** — for plugins **without** the `network` permission the
  launcher EPERMs `socket(AF_INET/AF_INET6)` at the kernel: no IP egress, holding
  even on Landlock ABI <4. `AF_UNIX` stays allowed for the HostService gRPC
  socket.
- **Server `PR_SET_DUMPABLE=0`** — the server marks itself non-dumpable at
  startup, so a same-uid plugin cannot read `/proc/<server>/{mem,environ,maps}`
  (the in-memory JWT key, `DB_PASSWORD`, `MEILI_MASTER_KEY`).
- **Userspace resource monitor** — see below; cgroups are not used.

What the **filesystem/network backend** adds depends on the mode:

| Mode | FS confinement | Network | Selected when |
|---|---|---|---|
| `landlock` | Landlock LSM allowlist (system dirs ro; plugin dir + sock dir rw; `/data` & siblings denied) | Landlock v4 TCP filter (declared ports only) + `-deny-inet` | `sandbox_mode=landlock` (fails closed if absent); `auto` on a Landlock kernel — the official image's normal case |
| `bubblewrap` | mount whitelist in unshared namespaces (see below) | `--unshare-net` unless `network` perm | `sandbox_mode=bubblewrap`; `auto` where Landlock is absent but the bwrap probe passes (bare-metal / privileged) |
| `strict` | same as `bubblewrap` + LD_PRELOAD seccomp | same as `bubblewrap` | `sandbox_mode=strict` |
| `basic` | none (DAC-only; `auto`'s last resort) | `-deny-inet` only | `sandbox_mode=basic`; `auto` when neither Landlock nor bubblewrap is usable |

`auto` resolves **landlock → bubblewrap → basic** once at first plugin start and
logs the chosen mode. `basic` is **not** "no isolation": it still applies UID
separation, the kernel + LD_PRELOAD seccomp, `-deny-inet`, best-effort Landlock,
and the resource monitor — it only loses *per-path FS confinement* when the
kernel has no Landlock (a plugin can then read non-sensitive world-readable
files, and `/proc` process *listing* is visible; `/proc/<server>/{mem,environ}`
stay protected).

When `sandbox_mode` is `bubblewrap` or `strict` (opt-in via `evey.yaml`), each plugin process runs inside a bubblewrap namespace with:

**Namespace isolation (5 unshared):**
- `--unshare-user` — dedicated user namespace, UID mapping
- `--unshare-pid` — isolated PID namespace
- `--unshare-uts` — isolated hostname
- `--unshare-ipc` — isolated IPC
- `--unshare-cgroup` — isolated cgroup namespace
- `--unshare-net` — network namespace isolated **unless** the plugin has the `network` permission

**Mount whitelist:**

The sandbox exposes read-only system paths needed for runtime (libraries, SSL, DNS) while blocking access to data paths:

| Path | Access | Rationale |
|---|---|---|
| `/` (full ro-bind) | Read-only | Provides `/usr`, `/lib`, `/bin`, `/sbin`, ssl certs, `resolv.conf` etc. |
| `<plugin_dir>` | Read-write | Plugin's own install directory |
| `<plugin_sock_dir>` | Read-write | Per-plugin IPC socket directory |
| `/proc` | Proc-mount | Required by runtimes |
| `/dev` | Synthetic | Minimal device set (null, zero, random, tty) — no physical devices |
| `/tmp` | tmpfs | Ephemeral scratch; host `/tmp` is not visible |

Paths **not** mounted: `/data`, `/var`, `/home`, other plugins' directories.

**Userspace resource monitor** (per plugin):

Resource limits are enforced by a userspace sweeper (not cgroups, which are unusable in a non-privileged Docker container). The monitor polls `/proc/<pid>/{statm,status,stat}` every ~2 s and kills the plugin if any cap is exceeded. `nice` is applied at spawn so a plugin never starves the server of CPU.

- Memory cap (RSS, default 256 MiB) — via `/proc/<pid>/statm`
- PID/thread count cap (default 64) — via `/proc/<pid>/status`
- CPU priority — via `nice` (default 10)
- Disk cap (install dir, default 100 MiB) — via recursive `filepath.Walk` every ~30 s

See [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md) for the rationale behind the cgroups abandonment.

Note: `--unshare-cgroup` in the bwrap namespace list below creates an isolated cgroup _namespace_ (the plugin sees a fresh cgroup hierarchy) but does **not** impose resource limits; those come from the userspace monitor above.

Source: `internal/adapters/driven/pluginrunner/runner_landlock.go`, `runner_bwrap.go`, `runner_resmon.go`, `runner_caps_linux.go`; `cmd/evey-plugin-launcher/{main.go,seccomp_linux.go,landlock_linux.go}`; `cmd/evey/hardening_linux.go`; `sandbox/sandbox.c`.

### 8. DNS whitelist (V1, best-effort)

For plugins with the `network` permission, Evey resolves all concrete hostnames from `external_fetch` at startup and writes a minimal `/etc/hosts` file, which is bind-mounted read-only over `/etc/hosts` inside the sandbox. The plugin's DNS resolver then only resolves whitelisted hostnames.

**V1 limitations (read carefully):**

| Limitation | Impact | V2 plan |
|---|---|---|
| Wildcard entries (`*.example.com`) are skipped | Subdomains declared with wildcards are not resolved — plugin effectively has no DNS for them | Enumerate via TLS cert transparency or per-request nftables rules |
| IP literal bypass | A plugin can `connect()` directly to an IP address — DNS whitelist has no effect | Per-namespace nftables egress filter in V2 |
| Custom resolvers | A plugin can call a DNS-over-HTTPS resolver that is itself whitelisted, bypassing `/etc/hosts` | Egress proxy enforcement in V2 |
| SNI trickery | A plugin can connect to a whitelisted IP and send a different SNI header | TLS inspection or egress proxy in V2 |
| DNS TTL drift | Hostnames resolved at startup may change IPs — `/etc/hosts` is not refreshed at runtime | Periodic re-resolution or per-request enforcement |

Source: `internal/adapters/driven/pluginrunner/runner_dns.go`.

### 9. IPC peer-credential authentication

The host exposes one Unix socket per plugin at `/tmp/evey/plugins/<plugin_id>/host.sock`. Every incoming gRPC call is authenticated by reading the peer's PID via `getsockopt(SO_PEERCRED)` — a value supplied by the kernel that cannot be forged in user space. The PID is compared against the value registered immediately after `cmd.Start`.

Effects:
- A plugin cannot impersonate another plugin: `x-plugin-id` headers are ignored; the identity comes from the socket listener.
- A plugin that fork/execs a child cannot route host calls through the child (different PID → `Unauthenticated`).

Source: `internal/adapters/driving/pluginhost/`.

### 10. Frontend isolation

Plugin frontend modules run inside an `<iframe>` with:
- `sandbox="allow-scripts"` — no same-origin access, no form submission, no popups
- CSP `<meta>` tag inside the `srcdoc`: `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src <external_fetch_hosts>; img-src data: blob:; font-src data:`
- postMessage origin enforced on the host side
- Bridge rate-limits: `toast` max 3 messages/second, 200 characters, no HTML
- `navigate` path whitelist: `/admin/*`, `/auth/*`, `/setup/*`, `/profile/security/*` are rejected
- `resize` clamped: 50–800 px
- Slot whitelist enforced both server-side (install) and client-side (render)

## Permissions catalog

| Permission | What it grants |
|---|---|
| `network` | Plugin may make outbound network connections. **Must** be combined with a non-empty `external_fetch` list — refused at install without it. |
| `subprocess` | Plugin may spawn child processes. Without it, the LD_PRELOAD seccomp shim blocks `execve`/`execveat` for interpreted plugins. (The launcher's kernel seccomp does not deny `execve` — it must exec the plugin — but any child inherits the kernel filter + the Landlock domain.) |
| `native-binary` | Plugin may ship `.so`/`.dll`/`.exe`/`.wasm`/`.jar` or embedded archives. Admin sees and must accept the opaque code at install preview. |
| `filesystem` | Declared intent to access the filesystem beyond the plugin's own directory. Enforced at audit; not yet a hard sandbox rule (see Known limits). |
| `media.read` | Plugin may call `MediaRead` RPCs: read media file metadata and stream access. |
| `media.write` | Plugin may call `MediaWrite` RPCs: create/update media records. |
| `api.library` | Plugin may call library management RPCs: list/search libraries, trigger scans. |
| `api.user` | Plugin may call user-scoped RPCs (read own profile, preferences). |
| `api.metadata` | Plugin may call metadata RPCs: search and write enrichment data. |
| `notification.sender` | Plugin may publish notification events (web push, email, webhook). |
| `webhook.receiver` | Plugin may register as a webhook target (receives HTTP POST from external services). |

## External fetch rules

Declared in `external_fetch: [...]` in the manifest. Validated at install by `internal/pkg/extfetch/validate.go`. All-or-nothing: a single invalid entry causes install to be refused.

| Entry | Valid | Reason |
|---|---|---|
| `api.tmdb.org` | Yes | Exact hostname |
| `*.tmdb.org` | Yes | Wildcard leftmost, TLD+SLD are concrete |
| `api.tmdb.org/v3/*` | Yes | Path wildcard allowed |
| `https://api.tmdb.org` | Yes | Explicit https scheme |
| `*` | No | Catch-all |
| `https://*` | No | Host wildcard alone |
| `*.com` | No | Wildcard on SLD (must be concrete) |
| `*.*.tmdb.org` | No | More than one wildcard label |
| `cdn.*.tmdb.org` | No | Wildcard must be the leftmost label |
| `http://api.tmdb.org` | No | Only `https://` is permitted |
| `192.168.1.1` | No | IP literal not allowed |
| `localhost` | No | Local host not allowed |
| `127.0.0.1` | No | Loopback not allowed |
| `/api/*` | No | Path without host |

Rules: scheme must be `https` (default when absent); wildcard `*` is only allowed as the leftmost label; TLD and SLD must be concrete; no IP literals (IPv4, IPv6, RFC1918, loopback); no `localhost`.

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

**Never allowed:** `admin.*`, `login.*`, `setup.*`, `profile.security.*` — these paths are authentication/privilege-critical and plugin injection there is a privilege-escalation or phishing vector.

Source: `internal/domain/plugin/slots.go`.

## Update workflow

1. Admin uploads a new ZIP → `PreviewPluginUpdate` runs the static analyzer + cosign check and returns a diff of changed permissions/slots/`external_fetch` vs the installed version.
2. If the diff is non-empty (new permissions, new slots, changed external hosts), the admin must explicitly acknowledge before the update is applied (`requires_ack = true`).
3. The source hash is captured at preview and verified at apply (anti-TOCTOU: the ZIP that was analyzed is the ZIP that gets installed).
4. If cosign policy is `enforce`, a signature mismatch at the apply step blocks the update even if the preview passed.

## Known limits

| Limit | Impact | Mitigation today |
|---|---|---|
| `auto` may resolve to `basic` on a host without Landlock and without usable bubblewrap | No **per-path** FS confinement: a plugin can read non-sensitive world-readable files, and `/proc` process listing is visible | Server secrets stay protected (UID separation + `PR_SET_DUMPABLE=0`); kernel seccomp (incl. static binaries) + `-deny-inet` still apply; the resolved mode is logged at first plugin start |
| All plugins share UID 1001 | A malicious plugin could signal siblings (signalling PID 1 / the server is seccomp-blocked) | `PR_SET_DUMPABLE=0` blocks cross-process `/proc/<pid>/{environ,mem,maps,fd}` reads; Landlock / bubblewrap fully isolate plugin directories from each other |
| Landlock TCP filter is per-port, not per-host | A network plugin can reach any host on an allowed port | Per-host `external_fetch` allow-list enforced at the HTTP layer; non-network plugins get no IP socket (`-deny-inet`) |
| DNS whitelist V1 does not block IP literals | Plugin can connect directly to IP addresses | V2: per-namespace nftables egress filter |
| Regex analyzer bypassable by advanced obfuscation | Motivated attacker can hide patterns | Admin Preview + permission review; enforcement boundary is the sandbox + resource monitor + IPC peer-cred, not the analyzer |

## What V2 will add

- Per-namespace nftables egress filter (blocks IP literal bypass)
- Egress proxy for full request-level enforcement
- seccomp allowlist mode (strict sandbox: only a minimal syscall set allowed)
- `external_fetch` coverage for wildcard entries via per-request enforcement
