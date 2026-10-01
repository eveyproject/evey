# Bare-Metal Deployment (no Docker)

Running Evey directly on a host (systemd, an init script, or by hand) instead of
the [Docker image](installation.md). The Docker image is the recommended path -
it ships the plugin sandbox, hardening, and runtimes pre-wired. Going bare metal
means **you** reproduce that posture: the right binaries side by side, a
dedicated non-root user, the sandbox backend your kernel supports, and
owner-only secret directories. This page is the step-by-step for that.

If you only want the easy install, stop here and use
[installation.md](installation.md).

## 1. Prerequisites

### Binaries (build from source)

Build both Go binaries and **place them in the same directory**. The plugin
runner locates the launcher by checking `$EVEY_PLUGIN_LAUNCHER` first, then
next to its own executable (`os.Executable()`), then `$PATH` - so keeping them
adjacent is the simplest correct layout.

```bash
# from the repo root
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# install side by side, e.g.:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# optional: shell completion and the manual page, both printed by the binary
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

Lookup order in `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (must be an executable file) - explicit override.
2. `evey-plugin-launcher` in the **same directory** as the running `evey` binary.
3. `evey-plugin-launcher` somewhere on `$PATH`.

### The LD_PRELOAD seccomp shim

Build the `libevey-sandbox.so` shim (a small C file; needs a C compiler):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

The runner looks for it at the **hardcoded path `/app/libevey-sandbox.so`**
(`sandboxLibPath` in `runner.go`). If it is absent, plugins **refuse to start**
(fail-closed) - unless you run in dev mode. This shim adds a seccomp denylist for
dynamically-linked plugins and is layered on top of Landlock/bubblewrap.

> The Docker image keeps the open-source binaries, SDK, and this shim under
> `/app` (no secrets there). On bare metal you can either mirror that path
> (`/app/libevey-sandbox.so`) or symlink it.

### Runtimes

Install whatever your plugins' `entrypoint:` invokes. The runner recognises
these interpreters (`knownInterpreters` in `runner_start.go`): `python3`, `python`,
`node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. At minimum:

- **`python3`** - the majority of community plugins.
- **`node`**, **`php`**, **`ruby`**, **`lua`** - only if your plugins use them.

Plus the core runtime dependencies:

- **ffmpeg** (transcoding; the image also bundles VAAPI drivers + `chromaprint`
  for intro detection).
- **PostgreSQL** (server + a database).
- **Meilisearch** (search index; or run it embedded via the official binary).
- **`bwrap`** (bubblewrap) - only if you intend to use the bubblewrap sandbox
  backend (see below).
- **`libcap`** (`setcap`) - to grant the file capability described in §4.

## 2. Run as a dedicated non-root user

**Never run the server as root.** Create a group and two unprivileged users,
as the Docker image does; plugins are dropped to UID 1001 (`pluginUID` in
`pluginhost/host_server.go`):

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

The group's id matters: the launcher drops a plugin to UID 1001 and to the
server's own group, and refuses any id below 1000. A group created by
`useradd --system` alone gets a system id below 1000, and plugins in `basic`
mode are then refused.

- `evey` (UID 1000) runs the server.
- `evey-plugin` (UID 1001) is the UID plugins are dropped to (by the launcher
  when uid separation is active), so a compromised plugin lands as a different,
  unprivileged user that cannot read the server's secrets by plain filesystem
  permissions. See §4.

The server marks itself **non-dumpable** at startup (`PR_SET_DUMPABLE=0`,
`cmd/evey/hardening_linux.go`) so a co-resident plugin can't scrape
`/proc/<server-pid>/{mem,environ}`.

## 3. Pick the sandbox backend for your kernel

Set `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (the default) resolves
**landlock → bubblewrap → basic** and picks the strongest your host supports
(`resolveAutoMode()` in `runner_landlock.go`). Choose explicitly if you want
fail-closed behaviour.

### Landlock (recommended)

A per-path filesystem allowlist (plugins see only their own dir + socket dir,
never your data/secrets) plus a Landlock v4 TCP-egress filter. Needs **no
privilege, no mount, no namespaces** - the launcher applies it to itself, then
`execve`s the plugin.

**Kernel requirement** - Landlock must be both compiled in *and* in the active
LSM list. The runner's `landlockAvailable()` (`landlock_probe_linux.go`) probes
`landlock_create_ruleset(VERSION)`; it returns an errno when the syscall is
absent (kernel `<5.13`) or when Landlock is not in the active LSM list. Verify:

```bash
uname -r                              # want >= 5.13
cat /sys/kernel/security/lsm          # must include "landlock"
```

If `landlock` is missing from that list, enable it on the kernel command line
and reboot (the exact list varies per distro - keep the existing entries and
append `landlock`):

```
# e.g. in GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Then:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # explicit + fail-closed (refuses a plugin if Landlock can't be enforced)
# or leave it on the default:
EVEY_PLUGINS_SANDBOX_MODE=auto        # resolves to landlock when available
```

`landlock` mode passes `-require-landlock` to the launcher, which exits non-zero
rather than running a plugin unconfined if Landlock isn't enforced.

### bubblewrap

If you can't get Landlock, install the `bwrap` binary. bubblewrap puts each
plugin in user/pid/uts/ipc/(net) namespaces with a mount whitelist. It needs
**unprivileged user namespaces enabled** and the **`mount` syscall available** -
true on most bare-metal Linux, but blocked inside a non-privileged Docker
container (which is exactly why Landlock is the container default). `auto` only
selects bubblewrap when Landlock is **absent** and a throwaway bwrap probe
actually succeeds (`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap namespaces
# or:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + the LD_PRELOAD seccomp shim
```

Sanity-check that the host actually permits the namespace+mount setup:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (last resort)

`EVEY_PLUGINS_SANDBOX_MODE=basic` - UID drop + the LD_PRELOAD seccomp shim, and
the launcher still installs the kernel seccomp denylist and best-effort Landlock
if present. There is **no per-path filesystem confinement**: a plugin runs at
UID 1001 with seccomp only. Use this only on old/locked-down kernels where
neither Landlock nor bubblewrap is available, and lean on the UID separation in
§4 to protect secrets.

> **A plugin granted the `subprocess` permission requires the launcher**, even in
> basic mode. The LD_PRELOAD shim does not survive an `exec` of a child process,
> so it cannot confine a plugin allowed to spawn subprocesses - only the
> launcher's kernel seccomp (installed right before `exec` and inherited across
> it) does. If `evey-plugin-launcher` is missing (see §1), such a plugin
> **refuses to start** in production (fail-closed) rather than running
> unconfined. Plugins without that permission fall back to the LD_PRELOAD shim as
> documented above.

## 4. UID separation (protect secrets without root)

UID separation runs each plugin under a separate user (UID 1001, `evey-plugin`),
distinct from the server (UID 1000), so a plugin can't read the server's
`jwt.key`, backups or logs by plain filesystem permissions (DAC). It matters most
in **`basic`** mode (no Landlock path confinement); under Landlock or bubblewrap
the filesystem is already walled off, so it's redundant there.

Controlled by `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, default
`auto`):

- `auto` - enable only when the sandbox resolves to `basic`. On a modern
  bare-metal kernel (Landlock) it stays off, by design.
- `on` - always, even under Landlock/bubblewrap (an extra independent layer).
- `off` - never.

**The drop is done by the launcher, not the server.** The server stays non-root
with **no** capabilities; the small `evey-plugin-launcher` binary carries a single
file capability and `setuid`s itself to 1001 just before exec'ing the plugin:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

This keeps the server fully unprivileged - a server RCE can't `setuid(0)` because
the server holds no caps; only the tiny, no-input launcher does (and it refuses
any target UID below 1000). If the launcher can't drop (capability missing, or
`no_new_privs` set), a plugin in `basic` mode is **refused** - the UID is its only
secret wall there; under Landlock it logs a warning and runs at the server's UID.

> **Incompatible with `no_new_privs`.** If you launch the server with
> `no-new-privileges` (or systemd `NoNewPrivileges=yes`), the file capability is
> ignored on exec: plugins are refused in `basic` mode, and under Landlock uid
> separation degrades to same-UID (with a warning). Pick one.

If the server runs **as root** instead (not recommended), it drops plugins to
1001 directly and the launcher capability isn't needed.

See the plain-language guide for the per-scenario picture:
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Secret file permissions

Mirror what the Docker image does (`Dockerfile`): keep the secret-bearing
directories **owner-only** so a plugin dropped to UID 1001 can't read them via
plain DAC, even without Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# The SERVER owns plugins/ (it extracts installs there); group evey - which
# evey-plugin belongs to - only reads/execs the installed files:
sudo chmod 750 /var/lib/evey/plugins
# owner-only (no group/other) on the secret dirs:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# the JWT signing key itself:
sudo chmod 600 /var/lib/evey/jwt.key      # (auto-generated 0600 on first boot)
```

(Adjust paths to wherever you point `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH`.)

## 6. Environment & config

All runtime config is via `EVEY_*` environment variables. Set the database,
search, storage paths, JWT key path, and reverse-proxy vars the same way the
Docker deployment does - see [configuration.md](configuration.md) for the full
table and [installation.md](installation.md) for the database/search/reverse-proxy
walkthrough. Don't duplicate the table here. Notes specific to bare metal:

- Point `EVEY_DB_HOST` / `EVEY_MEILI_URL` at your local PostgreSQL / Meilisearch.
- The server still serves **plain HTTP** - put a reverse proxy in front for TLS
  (an `https://` External URL and the proxy in Trusted proxies, in
  **Admin → Remote Access**, or `EVEY_SERVER_BASE_URL` and
  `EVEY_TRUSTED_PROXIES`). See [reverse-proxy.md](reverse-proxy.md).
- **The server opens TWO ports, and only one of them may be exposed.**
  `EVEY_SERVER_PORT` (default `9090`) is the native gRPC listener; the HTTP API
  your clients and reverse proxy talk to is **that port plus one** (`9091`).
  Point your reverse proxy and your firewall at the HTTP port only. The gRPC
  port is an internal transport: the remote-access network gate, which decides
  whether clients outside your LAN may connect at all, is wired into the HTTP
  chain and **does not cover the gRPC listener**. Because of that, the gRPC
  listener binds to **loopback only** by default (`EVEY_GRPC_BIND=127.0.0.1`),
  so a bare-metal run cannot expose it by accident. If you have a reason to
  publish it - a native gRPC client on another host - set
  `EVEY_GRPC_BIND=0.0.0.0`; the server warns at boot that the gate does not
  apply to it, and firewalling it is then on you.
- Migrations run automatically at startup; no separate migrate step.
- The plugin resource caps (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) and the sandbox mode are admin-editable at runtime
  from Admin → Plugins; the env values are just the initial defaults.

## 7. Verify your sandbox

The server logs the resolved sandbox backend. After the first plugin starts,
check the server log:

- **auto resolution** (logged once):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (or `bubblewrap` / `basic`).
- **per-plugin** (every plugin start):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **fell back to basic** (a warning you do *not* want to see if you expected
  Landlock): `plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available …` - means neither Landlock nor a working bwrap
  was found. Re-check §3.

Confirm the kernel pieces independently:

```bash
cat /sys/kernel/security/lsm          # Landlock active? (should list "landlock")
grep Seccomp: /proc/<plugin-pid>/status   # 2 = filter mode active on a running plugin
```

A `Seccomp: 2` line on a live plugin process confirms the kernel seccomp filter
is installed. If you pinned `EVEY_PLUGINS_SANDBOX_MODE=landlock` and Landlock
isn't enforced, the plugin **refuses to start** - that's the fail-closed
contract working as intended.

---

For everything else (backups, updates, recovery CLI, troubleshooting) the
Docker-oriented sections of [installation.md](installation.md) apply equally -
the `evey admin` subcommands and backup mechanics are the same binary.
