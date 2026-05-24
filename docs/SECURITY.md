# Plugin Security Model

This document describes the defense layers Evey applies to third-party plugins, the limits of those defenses, and the roadmap for stronger isolation.

## Threat model

Evey plugins run as separate OS processes that exchange gRPC over Unix sockets with the host. The host trusts no plugin code by default. Risks we defend against, in order of severity:

1. **Host RPC impersonation** — a malicious plugin pretends to be another plugin to gain its permissions.
2. **Privilege escalation** — a plugin escapes its sandbox and runs arbitrary commands as the server user.
3. **Filesystem leak / write** — a plugin reads or writes paths outside its install directory.
4. **Network exfiltration** — a plugin opens outbound connections to send data home.
5. **Resource abuse** — crypto mining, fork bomb, fd exhaustion.
6. **Supply chain** — a plugin author ships obfuscated or compiled malware that the static analyzer cannot inspect.

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

- **`subprocess`** — allowed to spawn child processes. Without it, the LD_PRELOAD seccomp shim denies `execve`/`execveat` for interpreted plugins. (The launcher's kernel seccomp does *not* deny `execve` — it must exec the plugin — but children inherit the filter + Landlock domain regardless.)
- **`native-binary`** — required to ship `.so` / `.dll` / `.exe` / archives. The admin must accept opaque code at install time.
- **API permissions** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) — checked at every host RPC by [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

### 3. UID separation (DAC)

A plugin process can be dropped to UID 1001 (`evey-plugin`), distinct from the server's UID (1000, `evey`), so the server's secrets are unreadable to it by plain DAC. It is controlled by `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, default `auto`):

- `auto` — drop **only when the sandbox resolves to `basic`** (no Landlock/bubblewrap FS confinement) — the one case where DAC is the only thing protecting secrets. Skipped under landlock/bubblewrap, where it's redundant.
- `on` — always (an extra independent layer on top of Landlock/bubblewrap).
- `off` — never.

**The drop is performed by the launcher, not the server.** The server stays non-root with no capabilities; `evey-plugin-launcher` carries `cap_setuid` (a file capability, `setcap cap_setuid+ep` in the `Dockerfile`) and `setgid`/`setuid`s itself to the shared group + UID 1001 right before exec. It refuses any target UID below 1000, and is **best-effort**: if it can't drop (no capability, or `no_new_privs` set) it warns and runs the plugin at the server's UID rather than failing the spawn — so enabling it can never break plugin startup. A root server drops plugins directly via `SysProcAttr` and needs no capability. Plain-language guide: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

What this buys, **even on a kernel without Landlock** (when active):

- The server's secrets are DAC-protected from a uid-1001 plugin: `jwt.key` is `0600`, and `/data/{keys,logs,backups}` are owner-only; backups are written `0700` dir + `0600` files. A plugin running as 1001 cannot read any of them.
- Combined with `PR_SET_DUMPABLE=0` on the server (see [§8](#8-server-process-hardening-pr_set_dumpable0)), the in-memory JWT key and environment secrets (`DB_PASSWORD`, `MEILI_MASTER_KEY`) are unreadable via `/proc/<server>/{mem,environ,maps}`.

All plugins share UID 1001, so they are not isolated from *each other* by DAC alone — that is what the filesystem confinement layer (Landlock / bubblewrap) adds. Under `basic`+uid-separation a malicious plugin could read or disturb a sibling's files; isolating plugins from each other without Landlock would need a UID *per plugin*. See [§4](#4-syscall-confinement-kernel-seccomp--ld_preload), the per-mode table below, and [Known limits](#known-limits).

### 4. Syscall confinement (kernel seccomp + LD_PRELOAD)

Two seccomp filters apply the same denylist; both call `prctl(PR_SET_NO_NEW_PRIVS)` first, then install a BPF program that EPERMs the denied syscalls and allows everything else.

1. **Kernel-applied (the primary layer).** The [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) shim is prepended to the plugin argv and installs the BPF filter ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)) **right before `execve`**, per build arch. The filter is inherited across `execve` into the plugin and into any child it later spawns. Because it is installed by the launcher and not via `LD_PRELOAD`, it **also constrains static/compiled plugin binaries** — which never load `libevey-sandbox.so` and used to bypass the shim entirely. An arch mismatch in the `seccomp_data` ABI field is `SECCOMP_RET_KILL_PROCESS` (foils i386/x32 syscall-number confusion).
2. **LD_PRELOAD (belt-and-suspenders for dynamic plugins).** [`sandbox/sandbox.c`](../sandbox/sandbox.c) is a tiny constructor preloaded into interpreted plugins; same denylist, plus it sets `PR_SET_DUMPABLE=0` on the plugin itself.

Blocked syscalls, grouped by category:

| Category | Syscalls blocked |
|---|---|
| Process control / debug | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Process signals | `kill`; `tkill`/`tgkill` only when the target is PID 1 (the server) |
| Filesystem mount | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernel modules | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| Hardware I/O | `ioperm`, `iopl` (LD_PRELOAD shim only) |
| Swap / memory mgmt | `swapon`, `swapoff` |
| System control | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`, `kexec_file_load` (LD_PRELOAD shim only) |
| Setuid family | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

`execve`/`execveat` is **denied in the LD_PRELOAD shim** (so an interpreted plugin without `subprocess` cannot spawn children) but **NOT denied in the launcher's kernel filter** — the launcher itself must `execve` the plugin. Spawned children inherit the launcher's filter + the Landlock domain, so they stay confined regardless.

**No-IP-egress for non-network plugins.** When a plugin lacks the `network` permission, the launcher is also passed `-deny-inet`: the kernel filter additionally EPERMs `socket(AF_INET)` / `socket(AF_INET6)`. This is the kernel equivalent of bubblewrap's `--unshare-net` — IP-literal egress can't sidestep the DNS allowlist — and it holds even on Landlock ABI <4 where the Landlock TCP filter is a no-op. `AF_UNIX` is left allowed so the HostService gRPC socket still works.

Limits — read this carefully:

- **Denylist, not allowlist.** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone`, etc. are all permitted. (The `strict` mode adds the LD_PRELOAD layer on top of bubblewrap; a true syscall allowlist is still roadmap.)
- **Filesystem isolation comes from a different layer.** seccomp does not confine paths; that is Landlock (or bubblewrap). On a kernel without Landlock and without bubblewrap, the plugin can read any world-readable file (but not the server's secrets — those are DAC-protected, see §3).
- **Network confinement.** A non-network plugin is cut off from IP sockets at the kernel (`-deny-inet`); a network plugin is additionally narrowed to its declared egress ports by the Landlock TCP filter where available (ABI v4+).
- **Fail-closed for interpreted plugins.** If the runner cannot locate `libevey-sandbox.so` and the plugin lacks `subprocess`, [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) refuses to start the plugin and emits a `plugin.error` event. The launcher's kernel seccomp is best-effort (a kernel without seccomp degrades to the other layers).

### 5. Per-plugin host gRPC socket + SO_PEERCRED auth

The host exposes one Unix socket per plugin at `/tmp/evey/plugins/<plugin_id>/host.sock`. Each socket is served by a dedicated gRPC server with an interceptor that:

1. Reads the peer's PID via `getsockopt(SO_PEERCRED)` — **the kernel supplies this value, it cannot be spoofed in user space**.
2. Compares against the PID the runner registered with `SetExpectedPID` immediately after `cmd.Start`.
3. Stamps the plugin ID into the gRPC context via a closure (the ID comes from the listener identity, never from the wire).

Effect:

- A plugin cannot impersonate another plugin by sending an `x-plugin-id` header (the header is ignored).
- A plugin cannot reach another plugin's host socket and call its RPCs — even if it `connect()`s, the PID check rejects with `Unauthenticated: PID mismatch`.
- A plugin that fork/execs a child cannot route host calls through the child (different PID → rejected). This is by design and aligns with the `subprocess` permission gate.

### 6. Admin Preview gate

`POST /api/v1/admin/plugins/preview` runs the static analyzer and returns the manifest + findings without installing. The admin sees:

- Requested permissions (including `native-binary`, `subprocess`, `network`, `api.*`).
- All findings with severity, file, line, and category.
- Extracted URLs grouped by type (literal, template, concatenation).
- Source SHA256 (matches the upcoming install).

The admin is the trust boundary for elevated permissions — Evey shows the surface, the admin accepts or rejects.

### 7. Plugin crash notification

Runner publishes a `plugin.error` domain event when:

- The sandbox library is missing and the plugin lacks `subprocess`.
- The plugin process exits non-zero.
- The auth interceptor logs a PID-mismatch attempt (warning-level slog only — not yet wired to the bus).

The notification service subscribes and pages the admin via the configured channel (web push / email / webhook).

### 8. Server process hardening (`PR_SET_DUMPABLE=0`)

At startup the server calls `prctl(PR_SET_DUMPABLE, 0)` ([`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go)). This flips the owner of `/proc/<server>/{mem,environ,maps,fd,…}` to root, so a process sharing the server's UID cannot read them. It matters most in the no-Landlock fallback (e.g. an old Synology kernel), where a plugin could otherwise scrape the server's in-memory JWT key and its environment secrets (`DB_PASSWORD`, `MEILI_MASTER_KEY`) via `/proc`. On Landlock kernels the plugin is already denied `/proc` of the server; this is the cross-kernel belt-and-suspenders. It does not affect the server reading its own files; a failure is logged, not fatal.

## Sandbox modes

`sandbox_mode` is configured via `evey.yaml` or `EVEY_PLUGINS_SANDBOX_MODE` (default `auto`). It selects the **filesystem + network confinement** backend; UID separation, the kernel/LD_PRELOAD seccomp denylist, `-deny-inet`, and the userspace resource monitor apply across all modes (subject to host capability). The runner picks the backend in [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go); `auto` resolves once at first plugin start ([`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go)).

### `auto` (default)

Resolves the best backend the host actually supports, in order: **landlock → bubblewrap → basic**.

- Prefers **landlock**: it needs no privilege, runs under Docker's default seccomp + AppArmor with zero host changes, and cannot be relaxed once applied — the right default for the primary containerised deployment.
- Falls back to **bubblewrap** only where Landlock is absent *and* a throwaway bwrap sandbox actually builds ([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) — bare-metal / privileged container with usable userns + mount).
- Else **basic**. The resolved mode is logged at first plugin start.

### `landlock`

- **Confines:** filesystem via a Landlock LSM allowlist applied by the launcher — system dirs (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`, `/dev`) read-only/exec; the plugin's own install dir + its socket dir read-write; everything else (notably `/data`, sibling plugin dirs) denied. Network via the Landlock v4 TCP filter (ABI ≥ v4 / kernel ≥ 6.7): a non-network plugin reaches no TCP at all; a network plugin may only `connect()` to its declared egress ports (80/443 plus any explicit `external_fetch` port) and may never `bind`. On top: the kernel + LD_PRELOAD seccomp denylist and `-deny-inet`.
- **Requires:** kernel ≥ 5.13 with Landlock in the active LSM list, and the `evey-plugin-launcher` binary. No mount, no unshare, no privilege — runs cleanly under Docker's default security profile.
- **Selected when:** `sandbox_mode=landlock` (fails the plugin closed if Landlock can't be enforced) or `auto` resolves to it (the official image's normal case).
- **Limits:** the Landlock TCP filter is per-port, not per-host — the per-host DNS allowlist (`external_fetch`) is still enforced separately, and on ABI <4 the TCP filter degrades to a no-op (but `-deny-inet` still cuts off a non-network plugin at the kernel). Filesystem confinement holds regardless.

### `bubblewrap`

- **Confines:** filesystem + network + process view via 5 unshared namespaces (`--unshare-user/-pid/-uts/-ipc/-cgroup`, plus `--unshare-net` unless the plugin has the `network` permission) and a mount whitelist — read-only system dirs, a writable plugin dir + socket dir, a synthetic `/proc` and minimal `/dev`, a tmpfs `/tmp`; `/data`, `/var`, `/home`, `/root`, sibling dirs are simply not mounted. The kernel + LD_PRELOAD seccomp denylist and `-deny-inet` still apply.
- **Requires:** the `bwrap` binary AND the `mount`/`unshare` syscalls; inside Docker that needs relaxed security, so it is effectively bare-metal (or a privileged container) only.
- **Selected when:** `sandbox_mode=bubblewrap`, or `auto` on a Landlock-less host where the bwrap probe passes.
- **Limits:** `--unshare-cgroup` is a cgroup *namespace*, not a resource limit (those come from the userspace monitor). The DNS allowlist (`external_fetch`) is best-effort — see [§ DNS whitelist](#dns-whitelist-v1--best-effort).

### `strict`

- Same as `bubblewrap` **plus** the LD_PRELOAD seccomp lib layered on top. Same requirements and limits as bubblewrap. (A true syscall *allowlist* is still roadmap.)

### `basic`

- **Confines:** UID drop to 1001 + the kernel seccomp denylist + `-deny-inet` + best-effort Landlock — all delivered by wrapping the plugin with the launcher, when the launcher binary is present (it is in the official image). If the launcher is absent (some dev images) it degrades to LD_PRELOAD-only + UID drop.
- **Selected when:** `sandbox_mode=basic`, or `auto` falls back to it (no Landlock and bubblewrap unusable — e.g. a locked-down old kernel).
- **Limits:** this is the weakest mode for *filesystem* confinement: when the kernel has no Landlock there is **no per-path FS confinement**, so a plugin can read non-sensitive world-readable files, and `/proc` process *listing* is visible. It is **not** "no isolation": the server's secrets remain DAC-protected (uid separation + `PR_SET_DUMPABLE=0`), the plugin gets no IP egress (`-deny-inet`), dangerous syscalls are blocked kernel-side (covering static binaries), and resources are capped by the monitor.

### Posture summary

| Deployment | FS confinement | Network | Dangerous syscalls | Secrets | Verdict |
|---|---|---|---|---|---|
| Docker + Landlock (`auto`→landlock) | per-path allowlist | per-port + `-deny-inet` | blocked (incl. static) | protected | **strong** |
| Docker without Landlock (e.g. old NAS, `auto`→basic) | none (world-readable visible) | no IP egress (`-deny-inet`) | blocked (incl. static) | protected (DAC + DUMPABLE + seccomp) | decent, not Landlock-equivalent |
| Bare-metal (`auto`→bubblewrap) | namespace + mount whitelist | net namespace | namespaces (PID/mount/net) isolate; LD_PRELOAD seccomp in `strict` — the launcher's kernel filter is *not* applied in bwrap mode | protected | **strongest** |

## Known limits

| Limit | Impact | Mitigation today | Roadmap |
|---|---|---|---|
| No per-path FS confinement in `basic` mode (no-Landlock host) | a plugin can read non-sensitive world-readable files; sibling plugin dirs are also visible | server secrets stay DAC-protected (uid separation + `PR_SET_DUMPABLE=0`); analyzer flags `/etc/passwd` patterns; `auto` prefers Landlock/bubblewrap where available | — (Landlock or bubblewrap is the answer; both already ship) |
| All plugins share UID 1001 | a malicious plugin can send signals to siblings (signalling PID 1 / the server is seccomp-blocked) | `PR_SET_DUMPABLE=0` closes `/proc/<pid>/{environ,mem,maps,fd}` reads between same-UID processes; Landlock/bubblewrap fully isolate plugin dirs | per-plugin user namespace via bubblewrap |
| Seccomp is a denylist, not an allowlist | plugin can `open`/`read`/`write`/(non-IP) `socket`/`connect` freely; ptrace, mount, namespaces, bpf, kexec, module load, setuid-family, keyring, and (for non-network plugins) IP sockets are blocked kernel-side incl. for static binaries — see [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | kernel seccomp via the launcher (pre-exec, covers static binaries) + `-deny-inet`; `subprocess` permission gate | true syscall allowlist mode for `strict` plugins |
| Landlock TCP filter is per-port, not per-host | a network plugin can reach any host on an allowed port | per-host DNS allowlist (`external_fetch`) enforced separately; non-network plugins get no IP socket at all | per-namespace nftables / egress proxy (V2) |
| Regex analyzer is bypassable by obfuscation | motivated attacker can hide patterns | admin Preview + permission review; enforcement boundary is the sandbox, not the analyzer | server-side risk-scoring + reputation check on plugin author |

## `sandbox_mode` config

Configurable via `evey.yaml` (or `EVEY_PLUGINS_SANDBOX_MODE` env var); see the [Sandbox modes](#sandbox-modes) section above for the detailed per-mode breakdown:

```yaml
sandbox_mode: auto   # default — resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + TCP egress filter + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, default `auto`) is a separate knob that controls whether plugins run under a distinct UID (see [§3](#3-uid-separation-dac)): `auto` enables it only when the mode resolves to `basic`, `on` forces it everywhere (extra independent layer), `off` disables it. Best-effort and launcher-driven; incompatible with `no_new_privs`. Per-scenario walkthrough: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Mapping per mode (all values reflect host capability — e.g. Landlock requires kernel ≥ 5.13 with Landlock in the active LSM list):

| Layer | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| UID separation (1001) — `EVEY_PLUGINS_UID_SEPARATION`, default `auto` | on in basic only | off (set `on` to force) | on | via userns¹ | via userns¹ |
| Kernel seccomp (launcher, pre-exec, covers static binaries) | yes | yes | yes | yes | yes |
| LD_PRELOAD seccomp (interpreted plugins) | yes | yes | yes | no | yes |
| `-deny-inet` (no IP socket for non-network plugins) | yes | yes | yes | n/a (net ns) | n/a (net ns) |
| Filesystem confinement | resolves to best available | Landlock allowlist | none (DAC-only) | mount whitelist | mount whitelist |
| Network | Landlock v4 TCP filter + `-deny-inet` | Landlock v4 TCP filter + `-deny-inet` | `-deny-inet` only | `--unshare-net` unless `network` perm | same |
| Namespaces (user/pid/uts/ipc/cgroup) | n/a | n/a | n/a | unshared | unshared |
| Resource caps | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` | userspace monitor + `nice` |

¹ In `bubblewrap`/`strict` the launcher isn't used, so the `cap_setuid` drop doesn't apply; the unshared **user namespace** provides UID isolation instead.

Note: a true syscall *allowlist* for `strict` is still roadmap; today `strict` = bubblewrap + the LD_PRELOAD denylist.

Implementation deps:

- `evey-plugin-launcher` (pure-Go, CGO-free) shipped next to the server binary — applies Landlock + the kernel seccomp filter. Used by `landlock`, `auto`, and `basic`.
- `apk add bubblewrap` in `Dockerfile` (community repo, ~200 KB) — present in the runtime image; used by `bubblewrap`/`strict`.
- For bubblewrap: Linux kernel `unprivileged_userns_clone=1` (Alpine ≥3.18 default). Inside Docker, the namespace/mount setup typically needs `--security-opt seccomp=unconfined` or relaxed AppArmor — which is why `auto` prefers Landlock for the containerised deployment.

## DNS whitelist (V1 — best effort)

Plugins with the `network` permission must declare an `external_fetch` allow-list
in their manifest. In **bubblewrap** mode the namespace is given a synthetic
`/etc/hosts` containing only the resolved IPs of allowed hosts, so anything else
returns NXDOMAIN. In **landlock** mode the kernel TCP filter narrows egress to the
declared *ports* (the Landlock filter is per-port, not per-host), and a
non-network plugin gets no IP socket at all (`-deny-inet`); per-host enforcement
is the same best-effort `external_fetch` allow-list applied at the HTTP layer.

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

## Reporting a vulnerability

Plugin auth, permission, or sandbox issues: open an issue with the `security` label or contact the maintainer directly. Do not exploit against shared/public Evey instances.
