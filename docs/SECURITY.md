# Plugin sandbox and threat model

This page describes what the server does to contain a plugin: the checks at
install, the layers at run time, the sandbox modes, and what each still cannot
stop. If you are writing a plugin and want to know what you may declare and
what will be refused, read [Permissions and slots](plugins/security-model.md).

## Threat model

Plugins run as separate processes that talk to the server over gRPC on Unix
sockets. The server trusts no plugin code by default. The risks it defends
against, from the most to the least severe:

1. **Impersonation** - a plugin pretends to be another plugin to use its
   permissions.
2. **Privilege escalation** - a plugin escapes its sandbox and runs commands
   as the server.
3. **Filesystem access** - a plugin reads or writes outside its own
   directory, in particular the server's secrets.
4. **Network exfiltration** - a plugin sends data to a host of its choosing.
5. **Resource abuse** - mining, fork bombs, memory or disk exhaustion.
6. **Supply chain** - obfuscated or compiled code the analysis cannot read.

## At install

### ZIP extraction

Before any file is written:

- at most 5,000 entries, 100 MB per file, 500 MB uncompressed in total, and a
  compression ratio of 100:1 (decompression bombs);
- entries whose path leaves the archive (`..`) are refused;
- symbolic links are refused;
- setuid, setgid and sticky bits are removed.

Source: `internal/adapters/driven/pluginrunner/zip.go`.

### Manifest and permissions

The manifest declares the permissions a plugin requests. An unknown permission
refuses the install. They are then enforced:

- **`subprocess`** allows child processes. Without it, process creation is
  refused by the kernel (`-deny-fork`), and the `LD_PRELOAD` filter also
  refuses `execve` for interpreted plugins.
- **`native-binary`** allows shipping compiled code or archives, which the
  analysis cannot read.
- **API permissions** (`api.library`, `api.user`, `api.metadata`,
  `media.read`, `media.write`, `notification.sender`...) are checked on every
  call to the host by
  [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

The full list of permissions is in [Permissions and slots](plugins/security-model.md#permissions-catalog).

### Static analysis

Every file of the package is scanned by
[`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go),
after a normalisation pass that undoes common obfuscation (aliases, string
concatenation, f-strings):

| Category | Severity | Examples |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `system()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby backticks, `open('\|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, reverse and bind shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink operations |
| `obfuscation` | CRITICAL | `eval`, base64 decoding, hex sequences, `__builtins__`, `base64 -d \| bash` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, signalling PID 1 |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: `yaml.load()` without a Loader |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe` |
| `compiled_binary` | WARNING, then BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` |
| `embedded_archive` | WARNING, then BLOCK | `.zip`, `.tar.gz` |
| `mime_mismatch` | treated as CRITICAL | an extension that claims an image, with other content |
| `suspicious_url` | WARNING | a non-HTTP scheme in a URL literal |
| `unvendored_deps` | WARNING | `requirements.txt` or `package.json` without vendored modules |

A CRITICAL finding or a `mime_mismatch` refuses the install. `compiled_binary`
and `embedded_archive` refuse it unless the manifest declares `native-binary`.

**What the analysis cannot guarantee.** It works on patterns. It resolves known
obfuscation classes (string concatenation into a callable over several steps,
builtin lookup by name, `Function.constructor` escapes), each with a regression
fixture in `tests/fixtures/plugins/malicious/`, but it cannot follow every
transformation done at run time (`strrev("metsys")`, decoding a payload read
from a file). It also matches inside comments: a `# pip install foo` comment
triggers `runtime_install`. **The enforcement boundary is the run-time sandbox,
not the analysis**: a plugin that slips a pattern past it still cannot read the
server's data, call a denied system call, exceed its resource limits, or call
a host method it has no permission for.

### Signature

A package can be signed with cosign. `EVEY_PLUGINS_COSIGN_POLICY` sets what a
failed verification does:

| Policy | Effect |
|---|---|
| `off` | The signature is not checked. |
| `warn` (default) | A failure is recorded and shown to the administrator, without blocking. With no public key configured, nothing is verified. |
| `enforce` | A failure refuses the install or the update. |

The public key is given by `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. Source:
`internal/adapters/driven/pluginrunner/cosign.go`.

### Administrator preview

`POST /api/v1/admin/plugins/preview` runs the analysis without installing.
The administrator sees the requested permissions, every finding with its
file, line and category, the URLs found in the code, and the SHA256 of the
package; the same hash is checked when the install is applied, so the package
analysed is the package installed. An update shows the difference in
permissions, slots and hosts, and a new permission requires an explicit
acknowledgement.

The administrator is the trust boundary for elevated permissions: Evey shows
the surface, the administrator accepts or refuses.

## At run time

### Filesystem

A plugin sees its own directory and its socket directory, and not the server's
data. How this is enforced depends on the [sandbox mode](#sandbox-modes):
a Landlock allowlist, or a bubblewrap mount whitelist. In `basic` mode on a
kernel without Landlock, there is no per-path confinement: the separate user
and file permissions are then what protect the server's secrets (below).

### Network

A plugin has **no direct network access**, whatever it declares: every IP
socket is refused (`-deny-inet` in `landlock` and `basic`, a network namespace
in `bubblewrap` and `strict`). A plugin with the `network` permission calls the
host's `Fetch` method over its Unix socket. The host, in
`internal/adapters/driving/pluginhost/fetch.go`:

- checks the target against the plugin's `external_fetch` list (exact hosts,
  case-insensitive);
- allows only `http` and `https`, and only `GET` and `HEAD`;
- connects through an SSRF-safe client (`internal/pkg/ssrf`) that refuses
  private, reserved and link-local addresses after DNS resolution, so DNS
  rebinding and redirects are checked again;
- adds none of the server's credentials, and caps the response size.

This closes the paths direct egress left open (IP literals such as
`169.254.169.254`, a plugin's own resolver). What remains: the broker controls
*where* a call goes, not *what* it carries. See [Known limits](#known-limits).

**Recommended for operators.** The broker moves the SSRF surface to the host.
An egress rule on the `evey-server` container, independent of the code, adds a
second wall: drop `169.254.0.0/16` and restrict the internal network to the
database and search endpoints.

### System calls

Two seccomp filters refuse the same dangerous system calls:

1. **The kernel filter**, installed by
   [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) just before it starts
   the plugin ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)).
   It is inherited by the plugin and its children and **also covers compiled,
   static binaries**. A mismatched architecture in a system call kills the
   process. Applied in `landlock` and `basic`.
2. **The `LD_PRELOAD` filter**, [`sandbox/sandbox.c`](../sandbox/sandbox.c),
   loaded into interpreted plugins that do not declare `subprocess`. It also
   refuses `execve`/`execveat`, and marks the plugin non-dumpable. Applied in
   `landlock`, `basic` and `strict`. If the library is missing, such a plugin
   is refused start (`plugin.error`).

Refused system calls:

| Category | System calls |
|---|---|
| Debugging | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Signals | `kill`; the kernel filter also refuses `tgkill`, the `LD_PRELOAD` filter refuses `tkill`/`tgkill` aimed at PID 1 |
| Mounts | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Kernel modules | `init_module`, `finit_module`, `delete_module` |
| BPF, perf | `bpf`, `perf_event_open` |
| Hardware I/O | `ioperm`, `iopl` (`LD_PRELOAD` filter only) |
| Swap | `swapon`, `swapoff` |
| System control | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`; `kexec_file_load` (`LD_PRELOAD` filter only) |
| Identity | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid`, `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

For a plugin without `subprocess`, the kernel also refuses process creation
(`fork`, `vfork`, `clone` without `CLONE_THREAD`); threads keep working.

This is a **denylist**: `open`, `read`, `write`, `mmap`, Unix `socket` and
`connect` stay allowed. Paths are confined by the filesystem layer, not by
seccomp. In `bubblewrap` the launcher is not used: a static binary gets no
seccomp filter there, and is contained by the namespaces and the mount
whitelist.

### A separate user

A plugin can run as user 1001 (`evey-plugin`), distinct from the server (1000),
so the server's secrets are unreadable to it by file permissions alone.
`EVEY_PLUGINS_UID_SEPARATION` controls it:

- `auto` (default): only when the sandbox resolves to `basic`, the one case
  where file permissions are the only wall around the secrets;
- `on`: always, as an extra layer;
- `off`: never.

The switch is made by the launcher, which holds `cap_setuid`; the server itself
runs without privileges or capabilities. The launcher refuses any target below
1000. If it cannot switch, a plugin in `basic` is refused (the user is the only
wall there), while in `landlock` it runs as the server's user with a warning.
In `bubblewrap` and `strict`, the user namespace plays this role.

When active, the signing key (`0600`) and the backups (`0700` directory,
`0600` files) are owner-only at run time, which holds even on a bind-mounted
`/data`. All plugins share user 1001: they are isolated from each other by the
filesystem layer, not by the user.

### Host socket authentication

Each plugin has its own socket, `/tmp/evey/plugins/<plugin_id>/host.sock`,
served by its own gRPC server. On every call, the server reads the caller's
PID from the kernel (`SO_PEERCRED`, which user space cannot forge) and compares
it with the PID recorded when the plugin started. The plugin's identity comes
from the socket, never from a header. A plugin therefore cannot impersonate
another, call another plugin's socket, or route calls through a child process.

### Resource limits

A monitor in user space (cgroups are unavailable in an unprivileged container)
reads `/proc/<pid>` every 2 seconds or so and stops a plugin that exceeds a
limit:

- memory (RSS), 256 MiB by default;
- processes and threads, 64 by default;
- disk used by its directory, 100 MiB by default, checked every 30 seconds;
- CPU priority, `nice` 10 by default, applied at start.

The limits are set per plugin in **Admin → Plugins**. See
[ADR-0013](adr/0013-userspace-plugin-resource-monitor.md).

### Interface isolation

A plugin's interface runs in an `<iframe sandbox="allow-scripts">`, without
same-origin access, forms or popups, under a Content-Security-Policy that
allows connections only to its `external_fetch` hosts. The bridge limits what
it can ask of the page: `toast` at most 3 per second, 200 characters, no HTML;
`navigate` refuses `/admin/*`, `/auth/*`, `/setup/*` and `/profile/security/*`;
`resize` between 50 and 800 px. Slots are checked at install and again at
render.

### Server hardening

At startup the server marks itself non-dumpable (`PR_SET_DUMPABLE=0`,
[`hardening_linux.go`](../cmd/evey/hardening_linux.go)): a process running as
the same user cannot read its memory or environment through `/proc`, which
protects the signing key in memory and the database and search passwords,
even on a kernel without Landlock.

### Crash notification

The runner publishes a `plugin.error` event when the sandbox library is
missing for a plugin that needs it, or when a plugin exits with an error. The
notification service forwards it to the administrator through the configured
channels. A PID mismatch on a host socket is logged as a warning.

## Sandbox modes

The mode selects how the filesystem and the network are confined.
`EVEY_PLUGINS_SANDBOX_MODE` sets the initial value (default `auto`); it can be
changed in **Admin → Plugins**. The runner applies it in
[`applySandbox`](../internal/adapters/driven/pluginrunner/runner_sandbox.go). In every
mode: no direct IP egress, the resource monitor, and the host socket
authentication.

### `auto` (default)

Resolves, once at the first plugin start, to the strongest mode the host
supports: **landlock**, then **bubblewrap**, then **basic**. Landlock comes
first because it needs no privilege and runs under Docker's default security
profile. bubblewrap is chosen only when Landlock is absent and a test sandbox
actually builds
([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go)),
which in practice means bare metal or a privileged container. The resolved
mode is logged, and shown on the **Host sandbox** line of **Admin → Plugins →
Security Dashboard**.

### `landlock`

- **Filesystem:** a Landlock allowlist applied by the launcher. System
  directories (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`,
  `/dev`) read-only; the plugin's directory and socket directory read-write;
  everything else, including `/data` and other plugins, denied.
- **Network:** `-restrict-net` and `-deny-inet`; egress only through `Fetch`.
- **System calls:** the kernel filter and the `LD_PRELOAD` filter.
- **Requires:** a kernel 5.13 or newer with Landlock in the active LSM list,
  and the launcher (both are the case on the official image with a recent
  host). An explicit `landlock` refuses to start a plugin it cannot confine.

### `bubblewrap`

- **Filesystem and processes:** six unshared namespaces (`user`, `pid`, `uts`,
  `ipc`, `cgroup`, `net`) and a mount whitelist; nothing else exists inside.

  | Path | Access |
  |---|---|
  | `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64` | read-only |
  | `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | read-only (never `/etc` as a whole: no `passwd`, `shadow`, `ssh`) |
  | the plugin's directory, its socket directory | read-write |
  | `/proc` | its own mount |
  | `/dev` | a minimal synthetic set (null, zero, random, urandom, tty, pts) |
  | `/tmp` | an empty tmpfs |

  `/data`, `/var`, `/home`, `/root`, `/run` and other plugins' directories do
  not exist inside. A canary plugin (`tests/fixtures/plugins/canary-sandbox-runtime/`)
  checks this layout against a running stack. Source: `buildBwrapCmd` in
  `internal/adapters/driven/pluginrunner/runner_bwrap.go`.
- **Network:** the network namespace, for every plugin.
- **System calls:** no seccomp filter; the namespaces provide the isolation.
- **Requires:** the `bwrap` binary and the `mount`/`unshare` system calls. In
  Docker that needs a relaxed security profile, so this is effectively a
  bare-metal mode.

### `strict`

`bubblewrap` plus the `LD_PRELOAD` filter (interpreted plugins only).

### `basic`

The last resort of `auto`: the separate user, the kernel filter, the
`LD_PRELOAD` filter, `-deny-inet`, and Landlock if the kernel has it. On a
kernel without Landlock there is no per-path confinement: a plugin can read
non-sensitive world-readable files and see the process list. The server's
secrets stay protected by the separate user and `PR_SET_DUMPABLE`, the network
is cut, and dangerous system calls are refused, compiled binaries included.

### Summary

| Layer | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|
| Filesystem | Landlock allowlist | none without Landlock (file permissions only) | mount whitelist | mount whitelist |
| Network | `-restrict-net` + `-deny-inet` | `-deny-inet` | network namespace | network namespace |
| Kernel seccomp (static binaries too) | yes | yes | no | no |
| `LD_PRELOAD` seccomp (interpreted plugins without `subprocess`) | yes | yes | no | yes |
| Separate user | with `on` | yes (`auto`) | user namespace | user namespace |
| Resource limits | yes | yes | yes | yes |

| Deployment | What you get |
|---|---|
| Docker on a recent kernel (`auto` gives `landlock`) | Per-path confinement, no direct egress, dangerous system calls refused including static binaries. The normal case. |
| Docker on an old kernel without Landlock, e.g. an old NAS (`auto` gives `basic`) | Secrets protected and network cut, but world-readable files visible. See [Plugin sandbox setup](plugins/sandbox-deployment.md). |
| Bare metal (`auto` gives `bubblewrap`) | The strongest filesystem and process isolation, without a seccomp filter for static binaries. |

## Configuration

| Variable | Default | Effect |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` or `basic`. Also editable in **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` or `off`. Incompatible with `no_new_privs` on the container. |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` or `enforce`. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | none | The key signatures are checked against. |

The official image ships `evey-plugin-launcher` (Landlock and the kernel
filter) and `bubblewrap`. bubblewrap also needs unprivileged user namespaces on
the host. Which setup fits which machine, in plain terms:
[Plugin sandbox setup](plugins/sandbox-deployment.md).

## Known limits

| Limit | Impact | What limits it today |
|---|---|---|
| No per-path confinement in `basic` on a kernel without Landlock | A plugin can read non-sensitive world-readable files and other plugins' directories | Secrets protected by the separate user and `PR_SET_DUMPABLE`; `auto` prefers Landlock or bubblewrap |
| All plugins share user 1001 | A plugin can signal its siblings (signalling the server is refused) | `PR_SET_DUMPABLE` blocks reading other processes through `/proc`; Landlock and bubblewrap isolate plugin directories |
| Seccomp is a denylist | File and Unix socket calls stay allowed | Paths are confined by Landlock or bubblewrap; IP sockets are refused |
| The broker moves the SSRF surface to the host | A flaw in the broker's validation could reach an internal address | Allowlist and SSRF-safe client in Go; an egress rule on the container as an independent layer |
| The broker controls where a call goes, not what it carries | Data can leave towards a host on the plugin's list | Allow only hosts you trust with that data |
| The analysis works on patterns | Deliberate obfuscation can hide code | Administrator preview; the sandbox, not the analysis, is the boundary |
| `LD_PRELOAD` only covers dynamic, interpreted plugins | A static binary in `basic` without the launcher escapes it | The official image always ships the launcher, whose kernel filter covers static binaries |

## Network exposure (outside the plugin threat model)

Two invariants govern who can reach the server at all:

- **Only the HTTP port is meant to be published.** The gRPC port
  (`EVEY_SERVER_PORT`, default 9090) is an internal transport whose only client
  is the HTTP gateway in the same process; the HTTP API is that port plus one.
  gRPC binds to loopback by default (`EVEY_GRPC_BIND=127.0.0.1`) because the
  remote-access gate is HTTP middleware and does not cover it. Widening the bind
  logs a warning; firewalling it is then the operator's job.
- **The remote-access gate decides from the one client address validated at
  the trusted proxy boundary**, never from a request header. A client cannot
  claim `127.0.0.1` through `X-Forwarded-For`, and a request that never crossed
  that boundary counts as remote. This is a deployment control, not
  authentication.

See [Remote access and HTTPS](administration/remote-access.md) and the
[Configuration reference](environment/server.md#remote-access).

## What the logs may contain

The server log feeds **Logs & Debug**, bug reports and backups, so credentials
are scrubbed when a line is written (`internal/pkg/logger/redact.go`): an
attribute whose key names a secret (`authorization`, `token`, `api_key`,
`password`, `pin`, `secret`...) is replaced whole, and any string carrying a
bearer token, a JWT or a `secret=value` pair is scrubbed. The log routes apply
the same rule to older lines on the way out. For code: never log a non-secret
value under a secret key name (it will be masked), and add a new kind of
credential to that list before it is ever logged.

## Reporting a vulnerability

Report privately, never in a public issue: `security@eveyproject.org`, the
address of the repository's [security policy](../SECURITY.md). It covers plugin
authentication, permissions and sandbox escapes like anything else. Do not
test against Evey instances that are not yours.

<!-- sources
install checks: internal/adapters/driven/pluginrunner/zip.go ; internal/adapters/driven/pluginrunner/analyzer.go ; internal/adapters/driven/pluginrunner/cosign.go ; internal/app/plugin/install.go
runtime: internal/adapters/driven/pluginrunner/runner_sandbox.go - applySandbox ; internal/adapters/driven/pluginrunner/runner_landlock.go (LD_PRELOAD layered in landlock) ; internal/adapters/driven/pluginrunner/runner_bwrap.go - buildBwrapCmd ; internal/adapters/driving/pluginhost/fetch.go - Fetch
config: internal/config/config.go - EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_COSIGN_POLICY, EVEY_PLUGINS_COSIGN_PUBLIC_KEY, EVEY_GRPC_BIND
-->
