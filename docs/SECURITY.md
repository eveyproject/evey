# Plugin sandbox and threat model

> **Which page do you want?** This one describes what the *server* does to
> contain a plugin: the layers, the sandbox modes, what each one still cannot
> stop, and how to configure it. If you are writing a plugin and want to know
> what you may declare and what will be refused, read
> [the plugin security model](plugins/security-model.md) instead. The two pages
> cover the same subject from opposite ends and deliberately do not repeat each
> other.

This document describes the defense layers Evey applies to third-party plugins, the limits of those defenses, and the roadmap for stronger isolation.

## Threat model: what a hostile plugin would try

Evey plugins run as separate OS processes that exchange gRPC over Unix sockets with the host. The host trusts no plugin code by default. Risks we defend against, in order of severity:

1. **Host RPC impersonation** - a malicious plugin pretends to be another plugin to gain its permissions.
2. **Privilege escalation** - a plugin escapes its sandbox and runs arbitrary commands as the server user.
3. **Filesystem leak / write** - a plugin reads or writes paths outside its install directory.
4. **Network exfiltration** - a plugin opens outbound connections to send data home.
5. **Resource abuse** - crypto mining, fork bomb, fd exhaustion.
6. **Supply chain** - a plugin author ships obfuscated or compiled malware that the static analyzer cannot inspect.

## Defense layers (current)

### 1. Static analysis at install (PREVIEW + INSTALL)

Every plugin ZIP is unpacked to a temp dir and walked by [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go). Categories scanned, with severity:

| Category | Severity | Examples |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `exec()`, `child_process.exec`, PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, reverse/bind shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink ops |
| `obfuscation` | CRITICAL | `eval`, `exec`, base64 decode, hex sequences, `__builtins__`, PHP `assert(base64_decode(...))`, shell `base64 -d \| bash`, `echo <blob> \| openssl` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, signal PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, Python `__subclasses__()`, `__class__.__bases__`, `__mro__` class-hierarchy traversal |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`/`pickle.load`, `marshal.loads`, `__reduce__`/`__reduce_ex__`; WARNING: `yaml.load()` without explicit Loader |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer`, `unsafe.Slice`, `unsafe.SliceData`, `unsafe.StringData` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` (opaque to scanner) |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` (extractable code) |
| `mime_mismatch` | WARNING (treated as critical) | file ext claims image, content is something else |
| `suspicious_url` | WARNING | non-HTTP scheme in URL literal |
| `unvendored_deps` | WARNING | `requirements.txt`, `package.json` present |

CRITICAL findings or `mime_mismatch` block install (`Safe = false`).

`compiled_binary` and `embedded_archive` block install **by default** unless the manifest declares the `native-binary` permission. The admin sees both the warning and the requested permission at the Preview step before accepting.

### 2. Manifest permission gate

A plugin's `plugin.yaml` declares the permissions it requests. The runner enforces these at two levels:

- **`subprocess`** - allowed to spawn child processes. Without it, the LD_PRELOAD seccomp shim denies `execve`/`execveat` for interpreted plugins. (The launcher's kernel seccomp does *not* deny `execve` - it must exec the plugin - but children inherit the filter + Landlock domain regardless.)
- **`native-binary`** - required to ship `.so` / `.dll` / `.exe` / archives. The admin must accept opaque code at install time.
- **API permissions** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) - checked at every host RPC by [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

### 3. UID separation (DAC)

A plugin process can be dropped to UID 1001 (`evey-plugin`), distinct from the server's UID (1000, `evey`), so the server's secrets are unreadable to it by plain DAC. It is controlled by `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, default `auto`):

- `auto` - drop **only when the sandbox resolves to `basic`** (no Landlock/bubblewrap FS confinement) - the one case where DAC is the only thing protecting secrets. Skipped under landlock/bubblewrap, where it's redundant.
- `on` - always (an extra independent layer on top of Landlock/bubblewrap).
- `off` - never.

**The drop is performed by the launcher, not the server.** The server stays non-root with no capabilities; `evey-plugin-launcher` carries `cap_setuid` (a file capability, `setcap cap_setuid+ep` in the `Dockerfile`) and `setgid`/`setuid`s itself to the shared group + UID 1001 right before exec. It refuses any target UID below 1000. If it can't drop (no capability, `no_new_privs` set, or the launcher binary itself missing), the outcome depends on the mode: in `basic`, where the UID is the only secret wall, the plugin is **refused** (fail-closed); under `landlock`, where the filesystem is already walled off, it warns and runs the plugin at the server's UID. A root server drops plugins directly via `SysProcAttr` and needs no capability. Plain-language guide: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

What this buys, **even on a kernel without Landlock** (when active):

- The server's secrets are DAC-protected from a uid-1001 plugin: `jwt.key` (`0600`) and backups (`0700` dir + `0600` files) are made owner-only **at runtime**, so the protection holds even on a bind-mounted `/data`; a uid-1001 plugin cannot read them. (The image also `chmod 700`s `/data/{keys,logs,backups}`, but a bind-mounted host `/data` keeps its own perms - the runtime modes above are what's guaranteed.)
- Combined with `PR_SET_DUMPABLE=0` on the server (see [§8](#8-server-process-hardening-pr_set_dumpable0)), the in-memory JWT key and environment secrets (`DB_PASSWORD`, `MEILI_MASTER_KEY`) are unreadable via `/proc/<server>/{mem,environ,maps}`.

All plugins share UID 1001, so they are not isolated from *each other* by DAC alone - that is what the filesystem confinement layer (Landlock / bubblewrap) adds. Under `basic`+uid-separation a malicious plugin could read or disturb a sibling's files; isolating plugins from each other without Landlock would need a UID *per plugin*. See [§4](#4-syscall-confinement-kernel-seccomp--ld_preload), the per-mode table below, and [Known limits](#known-limits-of-the-sandbox).

### 4. Syscall confinement (kernel seccomp + LD_PRELOAD)

Two seccomp filters apply nearly the same denylist (they differ only on the signal syscalls - see the table); both call `prctl(PR_SET_NO_NEW_PRIVS)` first, then install a BPF program that EPERMs the denied syscalls and allows everything else.

1. **Kernel-applied (the primary layer).** The [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) shim is prepended to the plugin argv and installs the BPF filter ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)) **right before `execve`**, per build arch. The filter is inherited across `execve` into the plugin and into any child it later spawns. Because it is installed by the launcher and not via `LD_PRELOAD`, it **also constrains static/compiled plugin binaries** - which never load `libevey-sandbox.so` and used to bypass the shim entirely. An arch mismatch in the `seccomp_data` ABI field is `SECCOMP_RET_KILL_PROCESS` (foils i386/x32 syscall-number confusion).
2. **LD_PRELOAD (belt-and-suspenders for dynamic plugins).** [`sandbox/sandbox.c`](../sandbox/sandbox.c) is a tiny constructor preloaded into interpreted plugins; same denylist, plus it sets `PR_SET_DUMPABLE=0` on the plugin itself.

**Which of the two runs depends on the sandbox mode** (see the per-mode table below): the launcher's kernel filter is applied in `landlock` and `basic`; the LD_PRELOAD shim is applied in `strict` and `basic`. `bubblewrap`/`strict` do **not** run the launcher, so a **static** plugin binary under plain `bubblewrap` gets no syscall denylist at all - its containment there is the bwrap namespaces + filesystem isolation, not seccomp.

Blocked syscalls, grouped by category:

| Category | Syscalls blocked |
|---|---|
| Process control / debug | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Process signals | `kill` (both filters); the launcher also denies `tgkill` outright, while the LD_PRELOAD shim instead denies `tkill`/`tgkill` only when aimed at PID 1 (the server) |
| Filesystem mount | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernel modules | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| Hardware I/O | `ioperm`, `iopl` (LD_PRELOAD shim only) |
| Swap / memory mgmt | `swapon`, `swapoff` |
| System control | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load` (both filters); `kexec_file_load` (LD_PRELOAD shim only) |
| Setuid family | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

`execve`/`execveat` is **denied in the LD_PRELOAD shim** (so an interpreted plugin without `subprocess` cannot spawn children) but **NOT denied in the launcher's kernel filter** - the launcher itself must `execve` the plugin. Spawned children inherit the launcher's filter + the Landlock domain, so they stay confined regardless.

**No-IP-egress for ALL plugins (since 2026-07-11).** Every plugin - network-permitted or not - is passed `-deny-inet` (landlock/basic) or `--unshare-net` (bubblewrap): the plugin process can open no `socket(AF_INET)` / `socket(AF_INET6)` at all. A plugin that needs the network does NOT connect directly; it calls the host `Fetch` broker over its `AF_UNIX` HostService socket (left allowed), which enforces the `external_fetch` allow-list and an SSRF-safe dialer host-side. This closes the IP-literal egress hole (e.g. `169.254.169.254`) that a per-host DNS allowlist and per-port kernel filter could not. See [ADR-0014](adr/0014-landlock-plugin-sandbox.md).

Limits - read this carefully:

- **Denylist, not allowlist.** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone`, etc. are all permitted. (The `strict` mode adds the LD_PRELOAD layer on top of bubblewrap; a true syscall allowlist is still roadmap.)
- **Filesystem isolation comes from a different layer.** seccomp does not confine paths; that is Landlock (or bubblewrap). On a kernel without Landlock and without bubblewrap, the plugin can read any world-readable file (but not the server's secrets - those are DAC-protected, see §3).
- **Network confinement.** Direct IP egress is cut for every plugin, network-permitted or not: `-restrict-net` (no `-net-connect` grants) plus `-deny-inet` block all `AF_INET`/`AF_INET6` sockets at the kernel. There is no per-port or per-host kernel filter. A network-permitted plugin reaches the network only through the host `Fetch` broker over its `AF_UNIX` socket, which enforces the `external_fetch` allow-list host-side.
- **Fail-closed for interpreted plugins.** If the runner cannot locate `libevey-sandbox.so` and the plugin lacks `subprocess`, [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) refuses to start the plugin and emits a `plugin.error` event. The launcher's kernel seccomp is best-effort (a kernel without seccomp degrades to the other layers).

### 5. Per-plugin host gRPC socket + SO_PEERCRED auth

The host exposes one Unix socket per plugin at `/tmp/evey/plugins/<plugin_id>/host.sock`. Each socket is served by a dedicated gRPC server with an interceptor that:

1. Reads the peer's PID via `getsockopt(SO_PEERCRED)` - **the kernel supplies this value, it cannot be spoofed in user space**.
2. Compares against the PID the runner registered with `SetExpectedPID` immediately after `cmd.Start`.
3. Stamps the plugin ID into the gRPC context via a closure (the ID comes from the listener identity, never from the wire).

Effect:

- A plugin cannot impersonate another plugin by sending an `x-plugin-id` header (the header is ignored).
- A plugin cannot reach another plugin's host socket and call its RPCs - even if it `connect()`s, the PID check rejects with `Unauthenticated: PID mismatch`.
- A plugin that fork/execs a child cannot route host calls through the child (different PID → rejected). This is by design and aligns with the `subprocess` permission gate.

### 6. Admin Preview gate

`POST /api/v1/admin/plugins/preview` runs the static analyzer and returns the manifest + findings without installing. The admin sees:

- Requested permissions (including `native-binary`, `subprocess`, `network`, `api.*`).
- All findings with severity, file, line, and category.
- Extracted URLs grouped by type (literal, template, concatenation).
- Source SHA256 (matches the upcoming install).

The admin is the trust boundary for elevated permissions - Evey shows the surface, the admin accepts or rejects.

### 7. Plugin crash notification

Runner publishes a `plugin.error` domain event when:

- The sandbox library is missing and the plugin lacks `subprocess`.
- The plugin process exits non-zero.
- The auth interceptor logs a PID-mismatch attempt (warning-level slog only - not yet wired to the bus).

The notification service subscribes and pages the admin via the configured channel (web push / email / webhook).

### 8. Server process hardening (`PR_SET_DUMPABLE=0`)

At startup the server calls `prctl(PR_SET_DUMPABLE, 0)` ([`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go)). This flips the owner of `/proc/<server>/{mem,environ,maps,fd,…}` to root, so a process sharing the server's UID cannot read them. It matters most in the no-Landlock fallback (e.g. an old Synology kernel), where a plugin could otherwise scrape the server's in-memory JWT key and its environment secrets (`DB_PASSWORD`, `MEILI_MASTER_KEY`) via `/proc`. On Landlock kernels the plugin is already denied `/proc` of the server; this is the cross-kernel belt-and-suspenders. It does not affect the server reading its own files; a failure is logged, not fatal.

## Sandbox modes

`sandbox_mode` is configured via `evey.yaml` or `EVEY_PLUGINS_SANDBOX_MODE` (default `auto`). It selects the confinement **backend**, and the syscall/egress mechanism is backend-specific: `landlock` and `basic` run the plugin through the launcher, which installs the kernel seccomp denylist + `-deny-inet`; `bubblewrap`/`strict` instead isolate the network with a namespace (`--unshare-net`) and have no launcher seccomp (`strict` adds only the dynamic-only LD_PRELOAD shim). What DOES apply across every mode: UID separation, the userspace resource monitor, and the fact that **direct IP egress is cut** (by `-deny-inet` or `--unshare-net`) so a network plugin reaches out only through the host `Fetch` broker. The runner picks the backend in [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go); `auto` resolves once at first plugin start ([`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go)).

### `auto` (default)

Resolves the best backend the host actually supports, in order: **landlock → bubblewrap → basic**.

- Prefers **landlock**: it needs no privilege, runs under Docker's default seccomp + AppArmor with zero host changes, and cannot be relaxed once applied - the right default for the primary containerised deployment.
- Falls back to **bubblewrap** only where Landlock is absent *and* a throwaway bwrap sandbox actually builds ([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) - bare-metal / privileged container with usable userns + mount).
- Else **basic**. The resolved mode is logged at first plugin start.

### `landlock`

- **Confines:** filesystem via a Landlock LSM allowlist applied by the launcher - system dirs (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`, `/dev`) read-only/exec; the plugin's own install dir + its socket dir read-write; everything else (notably `/data`, sibling plugin dirs) denied. Network: **no direct IP egress for any plugin** - `-restrict-net` (no `-net-connect` grant) plus `-deny-inet` block all `AF_INET`/`AF_INET6` sockets; network-permitted plugins reach the network only through the host `Fetch` broker over their `AF_UNIX` socket. On top: the kernel + LD_PRELOAD seccomp denylist.
- **Requires:** kernel ≥ 5.13 with Landlock in the active LSM list, and the `evey-plugin-launcher` binary. No mount, no unshare, no privilege - runs cleanly under Docker's default security profile.
- **Selected when:** `sandbox_mode=landlock` (fails the plugin closed if Landlock can't be enforced) or `auto` resolves to it (the official image's normal case).
- **Limits:** direct IP egress is cut for every plugin (`-deny-inet` holds even on Landlock ABI <4); per-host allow-listing now lives host-side in the `Fetch` broker, not in the kernel. Filesystem confinement holds regardless.

### `bubblewrap`

- **Confines:** filesystem + network + process view via 6 unshared namespaces (`--unshare-user/-pid/-uts/-ipc/-cgroup/-net` - `--unshare-net` is applied unconditionally to **every** plugin; direct egress is brokered) and a mount whitelist - read-only system dirs, a writable plugin dir + socket dir, a synthetic `/proc` and minimal `/dev`, a tmpfs `/tmp`; `/data`, `/var`, `/home`, `/root`, sibling dirs are simply not mounted. `applySandbox` routes this mode to `applyBwrapSandbox`, which never invokes the launcher: the launcher's kernel seccomp does **not** apply here, and neither does the LD_PRELOAD shim (that's `strict` only). Syscall confinement comes from the unshared namespaces alone.
- **Requires:** the `bwrap` binary AND the `mount`/`unshare` syscalls; inside Docker that needs relaxed security, so it is effectively bare-metal (or a privileged container) only.
- **Selected when:** `sandbox_mode=bubblewrap`, or `auto` on a Landlock-less host where the bwrap probe passes.
- **Limits:** `--unshare-cgroup` is a cgroup *namespace*, not a resource limit (those come from the userspace monitor). No kernel seccomp denylist applies in this mode (see above); `-deny-inet` is not used either since `--unshare-net` already blocks all IP sockets.

### `strict`

- Same as `bubblewrap` **plus** the LD_PRELOAD seccomp lib layered on top. Same requirements and limits as bubblewrap. (A true syscall *allowlist* is still roadmap.)

### `basic`

- **Confines:** UID drop to 1001 + the kernel seccomp denylist + `-deny-inet` + best-effort Landlock - all delivered by wrapping the plugin with the launcher, when the launcher binary is present (it is in the official image). If the launcher is absent (some dev images) it degrades to LD_PRELOAD-only + UID drop.
- **Selected when:** `sandbox_mode=basic`, or `auto` falls back to it (no Landlock and bubblewrap unusable - e.g. a locked-down old kernel).
- **Limits:** this is the weakest mode for *filesystem* confinement: when the kernel has no Landlock there is **no per-path FS confinement**, so a plugin can read non-sensitive world-readable files, and `/proc` process *listing* is visible. It is **not** "no isolation": the server's secrets remain DAC-protected (uid separation + `PR_SET_DUMPABLE=0`), the plugin gets no IP egress (`-deny-inet`), dangerous syscalls are blocked kernel-side (covering static binaries), and resources are capped by the monitor.

### Posture summary

| Deployment | FS confinement | Network | Dangerous syscalls | Secrets | Verdict |
|---|---|---|---|---|---|
| Docker + Landlock (`auto`→landlock) | per-path allowlist | no direct egress (`-deny-inet`); brokered | blocked (incl. static) | protected | **strong** |
| Docker without Landlock (e.g. old NAS, `auto`→basic) | none (world-readable visible) | no IP egress (`-deny-inet`) | blocked (incl. static) | protected (DAC + DUMPABLE + seccomp) | decent, not Landlock-equivalent |
| Bare-metal (`auto`→bubblewrap) | namespace + mount whitelist | net namespace | namespaces (PID/mount/net) isolate; LD_PRELOAD seccomp in `strict` - the launcher's kernel filter is *not* applied in bwrap mode | protected | **strongest** |

## Known limits of the sandbox

| Limit | Impact | Mitigation today | Roadmap |
|---|---|---|---|
| No per-path FS confinement in `basic` mode (no-Landlock host) | a plugin can read non-sensitive world-readable files; sibling plugin dirs are also visible | server secrets stay DAC-protected (uid separation + `PR_SET_DUMPABLE=0`); analyzer flags `/etc/passwd` patterns; `auto` prefers Landlock/bubblewrap where available | - (Landlock or bubblewrap is the answer; both already ship) |
| All plugins share UID 1001 | a malicious plugin can send signals to siblings (signalling PID 1 / the server is seccomp-blocked) | `PR_SET_DUMPABLE=0` closes `/proc/<pid>/{environ,mem,maps,fd}` reads between same-UID processes; Landlock/bubblewrap fully isolate plugin dirs | per-plugin user namespace via bubblewrap |
| Seccomp is a denylist, not an allowlist | plugin can `open`/`read`/`write`/(AF_UNIX) `socket`/`connect` freely; ptrace, mount, namespaces, bpf, kexec, module load, setuid-family, keyring, and IP sockets (all plugins) are blocked kernel-side incl. for static binaries - see [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | kernel seccomp via the launcher (pre-exec, covers static binaries) + `-deny-inet`; `subprocess` permission gate | true syscall allowlist mode for `strict` plugins |
| Broker relocates SSRF surface to the host | a bug in the broker's URL/IP validation could let a plugin reach an internal address via the host | broker enforces `external_fetch` allow-list + SSRF-safe dialer (private/reserved IPs rejected at connect time) in Go; operators SHOULD add a container egress firewall (drop `169.254.0.0/16`, whitelist DB+Meili) as an independent layer | per-namespace nftables / dedicated broker egress (V2) |
| Regex analyzer is bypassable by obfuscation | motivated attacker can hide patterns | admin Preview + permission review; enforcement boundary is the sandbox, not the analyzer | server-side risk-scoring + reputation check on plugin author |

## `sandbox_mode` config

Configurable via `evey.yaml` (or `EVEY_PLUGINS_SANDBOX_MODE` env var); see the [Sandbox modes](#sandbox-modes) section above for the detailed per-mode breakdown:

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, default `auto`) is a separate knob that controls whether plugins run under a distinct UID (see [§3](#3-uid-separation-dac)): `auto` enables it only when the mode resolves to `basic`, `on` forces it everywhere (extra independent layer), `off` disables it. Launcher-driven, fail-closed in `basic` (best-effort only under `landlock`); incompatible with `no_new_privs`. Per-scenario walkthrough: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Mapping per mode (all values reflect host capability - e.g. Landlock requires kernel ≥ 5.13 with Landlock in the active LSM list):

| Layer | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| UID separation (1001) - `EVEY_PLUGINS_UID_SEPARATION`, default `auto` | on in basic only | off (set `on` to force) | on | via userns¹ | via userns¹ |
| Kernel seccomp (launcher, pre-exec, covers static binaries) | yes if resolves to landlock/basic; no if resolves to bubblewrap | yes | yes | no (bwrap has no launcher seccomp; `applySandbox` routes bubblewrap/strict to `applyBwrapSandbox`, which never invokes the launcher) | no (strict adds only the LD_PRELOAD shim on top of bwrap - dynamic plugins only, no kernel seccomp) |
| LD_PRELOAD seccomp (interpreted plugins) | yes | yes | yes | no | yes |
| `-deny-inet` (no IP socket for ANY plugin) | yes | yes | yes | n/a (net ns) | n/a (net ns) |
| Filesystem confinement | resolves to best available | Landlock allowlist | none (DAC-only) | mount whitelist | mount whitelist |
| Network (all plugins: no direct IP egress; egress via host `Fetch` broker) | `-deny-inet` | `-restrict-net` + `-deny-inet` | `-deny-inet` | `--unshare-net` | `--unshare-net` |
| Namespaces (user/pid/uts/ipc/cgroup) | n/a | n/a | n/a | unshared | unshared |
| Resource caps | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` |

¹ In `bubblewrap`/`strict` the launcher isn't used, so the `cap_setuid` drop doesn't apply; the unshared **user namespace** provides UID isolation instead.

Note: a true syscall *allowlist* for `strict` is still roadmap; today `strict` = bubblewrap + the LD_PRELOAD denylist.

Implementation deps:

- `evey-plugin-launcher` (pure-Go, CGO-free) shipped next to the server binary - applies Landlock + the kernel seccomp filter. Used by `landlock`, `auto`, and `basic`.
- `apk add bubblewrap` in `Dockerfile` (community repo, ~200 KB) - present in the runtime image; used by `bubblewrap`/`strict`.
- For bubblewrap: Linux kernel `unprivileged_userns_clone=1` (Alpine ≥3.18 default). Inside Docker, the namespace/mount setup typically needs `--security-opt seccomp=unconfined` or relaxed AppArmor - which is why `auto` prefers Landlock for the containerised deployment.

## Plugin network egress (brokered, since 2026-07-11)

Plugins have **no direct network access**: the sandbox denies all IP sockets
(`--unshare-net` / `-deny-inet`) for every plugin, regardless of the `network`
permission. A plugin that needs the network calls the host `HostService.Fetch`
RPC over its `AF_UNIX` host socket, gated on the `network` permission. The host
(`internal/adapters/driving/pluginhost/fetch.go`) then, in Go:

- checks the target host is on the plugin's declared `external_fetch` allow-list
  (a real per-host contract now, not a best-effort DNS hint) - exact host or a
  `*.domain` wildcard;
- restricts the scheme to `http`/`https` and the method to `GET`/`HEAD`;
- dials through the SSRF-safe client (`internal/pkg/ssrf`), which rejects
  private/reserved/link-local IPs **after** DNS resolution at connect time, so
  DNS-rebinding and redirects are re-validated;
- attaches none of the server's own credentials, and caps the response body.

**Operator hardening (recommended).** The broker moves the SSRF surface to the
host, so add an independent egress layer on the `evey-server` container that does
not trust the code: drop `169.254.0.0/16` (cloud metadata / link-local) and
restrict the internal subnet to the Postgres + Meilisearch endpoints. A bug in
the broker's validation then still hits a network wall; the two layers fail
orthogonally.

**Limitations of V1**:
- IP literals bypass: a plugin calling `connect("1.2.3.4", 443)` does not go
  through DNS and reaches the IP unimpeded.
- Custom resolvers: a plugin shipping a UDP resolver (`53/udp`) can query an
  arbitrary nameserver and reach any host.
- SNI/Host header trickery: once any whitelisted host is resolved, the plugin
  can reuse that TCP connection target.

**Mitigation guidance**:
- Treat `network` permission as "audit manually": verify the plugin source
  does not call IPs directly, ship its own resolver, or call non-whitelisted
  paths.
- Operators should monitor egress traffic from the host running the plugin
  runtime (Prometheus + flow exporter, or simply `ss -tnp | grep <plugin-pid>`).

**Roadmap V2** (post-1.0):
- netfilter rules per plugin namespace constraining outbound to whitelisted
  CIDRs only.
- Optional egress HTTP proxy with TLS SNI inspection.

## Network exposure (outside the plugin threat model)

Everything above concerns plugins. The other half of this server's attack surface
is who can reach it at all, and that is governed by two invariants worth stating
here because a reader looking for "how do I expose Evey safely" starts in this
file:

- **One listener is meant to be published, and it is not the gRPC one.** The
  native gRPC port (`EVEY_SERVER_PORT`, default 9090) is an internal transport
  whose only client is the HTTP gateway inside the same process; the HTTP API is
  that port **plus one**. The gRPC listener binds to loopback by default
  (`EVEY_GRPC_BIND=127.0.0.1`) precisely because the remote-access gate below is
  HTTP middleware and does not cover it. Widening that bind is possible and logs
  a warning at boot; firewalling it is then the operator's job.
- **The remote-access gate decides LAN vs WAN from the ONE client IP validated at
  the trusted proxy boundary, never from a request header.** A client cannot
  claim to be `127.0.0.1` via `X-Forwarded-For` to be treated as local, and a
  request that never crossed that boundary is classified WAN (refused) rather
  than trusted. Note this is a *deployment* control ("this instance is for my own
  network"), not an authentication layer - authentication is what protects data.

Details and the full variable list: [configuration.md](configuration.md#remote-access-who-may-connect-from-outside-your-network)
and [deployment-bare-metal.md](deployment-bare-metal.md).

## What the logs may contain

The server log file is what the admin log routes (tail, search, live follow)
serve, what ends up in bug reports and what a backup carries, so credentials
are scrubbed **at the write** (`internal/pkg/logger/redact.go`): an attribute
whose key names a secret (`authorization`, `token`, `access_token`,
`refresh_token`, `api_key`, `password`, `pin`, `secret`, ...) is replaced whole,
and any string - the message included - carrying a bearer, a JWT or a
`secret=value` pair is scrubbed. The admin log routes apply the same rule on the
way out for lines written before that scrub existed. Two consequences for code:
a non-secret value must not be logged under a secret key name (it will be
masked - `token_info`, `device_pin` are the names used today), and a new kind of
credential must be added to that list before it is ever logged.

## Reporting a vulnerability

Report privately, never in a public issue: `security@eveyproject.org`. That is
the address the repository's [security policy](../SECURITY.md) gives, and it
covers plugin authentication, permissions and sandbox escapes like anything
else. Do not test against shared or public Evey instances that are not yours.
