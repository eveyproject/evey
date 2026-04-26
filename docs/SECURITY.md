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
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `exec()`, `child_process.exec` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, reverse/bind shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink ops |
| `obfuscation` | CRITICAL | `eval`, `exec`, base64 decode, hex sequences, `__builtins__` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, signal PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... | sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function` |
| `native_code` | CRITICAL | `ctypes.CDLL`, `ffi.Library` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` (opaque to scanner) |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` (extractable code) |
| `mime_mismatch` | WARNING (treated as critical) | file ext claims image, content is something else |
| `suspicious_url` | WARNING | non-HTTP scheme in URL literal |
| `unvendored_deps` | WARNING | `requirements.txt`, `package.json` present |

CRITICAL findings or `mime_mismatch` block install (`Safe = false`).

`compiled_binary` and `embedded_archive` block install **by default** unless the manifest declares the `native-binary` permission. The admin sees both the warning and the requested permission at the Preview step before accepting.

### 2. Manifest permission gate

A plugin's `plugin.yaml` declares the permissions it requests. The runner enforces these at two levels:

- **`subprocess`** — required to bypass the seccomp filter (i.e. allowed to spawn child processes). Without it the plugin starts under LD_PRELOAD seccomp.
- **`native-binary`** — required to ship `.so` / `.dll` / `.exe` / archives. The admin must accept opaque code at install time.
- **API permissions** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) — checked at every host RPC by [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

### 3. UID drop

When the server runs as root, the plugin process is started under UID/GID 1001 (`evey-plugin`). All plugins share this UID — which is a known limitation, see below.

### 4. Seccomp filter (LD_PRELOAD)

[`sandbox/sandbox.c`](../sandbox/sandbox.c) is a tiny constructor that calls `prctl(PR_SET_NO_NEW_PRIVS)` then `PR_SET_SECCOMP` with a BPF program that **denies ~38 syscalls**, allowing every other syscall.

Blocked syscalls, grouped by category:

| Category | Syscalls blocked |
|---|---|
| Exec | `execve`, `execveat` |
| Process control / debug | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Filesystem mount | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernel modules | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| Hardware I/O | `ioperm`, `iopl` |
| Swap / memory mgmt | `swapon`, `swapoff` |
| System control | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`, `kexec_file_load` |
| Setuid family | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

Limits — read this carefully:

- **Denylist of ~38 syscalls.** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone`, etc. are all permitted.
- **No filesystem isolation.** A plugin can read every file readable by UID 1001 — its own directory, `/tmp`, the host socket dir, and any sibling plugin's install dir (since sibling dirs are also chowned to 1001).
- **No network isolation.** Outbound TCP/UDP is unrestricted; data exfiltration is possible.
- **Static binary bypass.** A Go/Rust binary that calls `syscall(SYS_execve, ...)` directly is not affected by LD_PRELOAD because the loader never invokes the constructor. Detection: the analyzer tags the binary as `compiled_binary` and the install is blocked unless `native-binary` is requested. This is why the BLOCK-by-default policy matters.
- **Fail-closed.** If the runner cannot find or compile `libevey-sandbox.so` and the plugin lacks the `subprocess` permission, [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) refuses to start the plugin and emits a `plugin.error` event for the admin.

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

## Known limits

| Limit | Impact | Mitigation today | Roadmap |
|---|---|---|---|
| Network egress unrestricted | exfiltration possible | static analyzer flags `requests.*` patterns; admin reviews URL list at preview | bubblewrap `--unshare-net` opt-in (v1.x) |
| Filesystem unconfined within UID 1001 | sibling plugin reads | install dirs chmod 0750 + analyzer flags `/etc/passwd` patterns | bubblewrap `--ro-bind /` + writable `--bind <plugin_dir>` only |
| All plugins share UID 1001 | a malicious plugin can send signals to siblings, read their `/proc/<pid>/environ` | `PR_SET_DUMPABLE=0` in sandbox lib (TODO) | per-plugin user namespace via bubblewrap |
| Seccomp = denylist (~38 syscalls) | plugin can `open`/`socket`/`connect` freely; exec, ptrace, mount, bpf, kexec, module load, setuid-family, keyring blocked — see [§4](#4-seccomp-filter-ld_preload) | `subprocess` permission gate; native-binary BLOCK | bubblewrap seccomp policy + allowlist mode for `strict` plugins |
| Static binary skips LD_PRELOAD | seccomp inactive | analyzer flags + native-binary opt-in BLOCKS install by default | seccomp applied at exec time via `bwrap --seccomp` (kernel-level, no library required) |
| Regex analyzer is bypassable by obfuscation | motivated attacker can hide patterns | admin Preview + permission review | server-side risk-scoring + reputation check on plugin author |

## Roadmap — `sandbox_mode` config (post-beta)

Planned in a v1.x release, opt-in via `evey.yaml`:

```yaml
sandbox_mode: basic     # default — current LD_PRELOAD seccomp + UID drop
# sandbox_mode: bubblewrap  # full namespace isolation per plugin (recommended)
# sandbox_mode: strict      # bubblewrap + seccomp allowlist (locked-down API set)
```

Mapping per mode:

| Layer | basic | bubblewrap | strict |
|---|---|---|---|
| UID drop | yes (1001) | per-plugin uid map | per-plugin uid map |
| Seccomp | denylist (LD_PRELOAD) | denylist (kernel-applied) | allowlist |
| Mount ns | none | `--ro-bind /` + writable plugin dir | same |
| Network ns | none | `--unshare-net` unless `network` perm | same |
| User ns | none | `--unshare-user` | same |
| PID ns | none | `--unshare-pid` | same |

Implementation deps:

- `apk add bubblewrap` in `Dockerfile` (community repo, ~200 KB).
- Linux kernel `unprivileged_userns_clone=1` (Alpine ≥3.18 default).
- Docker users may need `--security-opt seccomp=unconfined` or `--cap-add SYS_ADMIN` if their host seccomp profile blocks `clone(CLONE_NEWUSER)`.

The default stays `basic` — friction-zero for plugin authors, opt-in for admins who want the harder isolation.

## Reporting a vulnerability

Plugin auth, permission, or sandbox issues: open an issue with the `security` label or contact the maintainer directly. Do not exploit against shared/public Evey instances.
