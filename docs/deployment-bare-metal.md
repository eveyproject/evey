# Without a container

How to run Evey directly on a host (systemd, an init script, or by hand)
instead of the [Docker image](installation.md). The image is the
recommended path: it ships the plugin sandbox, the hardening and the plugin
runtimes already wired. Going bare metal means reproducing that posture
yourself: the right binaries side by side, a dedicated non-root user, the
sandbox backend your kernel supports, and owner-only secret directories.
This page walks through it in order.

## Build and install the binaries

Build the two Go binaries and install them in the same directory: the
plugin runner looks for the launcher in `$EVEY_PLUGIN_LAUNCHER` first, then
next to its own executable, then on `$PATH`.

```bash
# from the repository root
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# optional: shell completion and the manual page, both printed by the binary
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` prints the version, commit and build date without any
configuration.

### The seccomp shim

Build `libevey-sandbox.so` (one C file, needs a C compiler) and install it
at the path the runner expects, `/app/libevey-sandbox.so`:

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

The path is fixed in the runner. Without the file, plugins refuse to start
(fail closed) unless the server runs in dev mode. The shim adds a seccomp
denylist for dynamically linked plugins, on top of Landlock or bubblewrap.
The image keeps it under `/app` with the SDK; a symlink works too.

### Runtimes and services

Install whatever your plugins' `entrypoint:` invokes. The runner recognises
`python3`, `python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash` and `sh`;
`python3` covers most community plugins. Then the services the server talks
to:

| Dependency | Why |
|---|---|
| **ffmpeg** and **ffprobe** | transcoding, probing; the image also bundles VAAPI drivers and `fpcalc` (chromaprint) for intro detection |
| **PostgreSQL** (server plus a database) and **`pg_dump`** | the database; `pg_dump` writes the automatic pre-migration dump |
| **Meilisearch** | the search index; without it search falls back to SQL |
| **`bwrap`** (bubblewrap) | only for the bubblewrap sandbox backend |
| **`setcap`** (libcap) | to grant the launcher its file capability |

The embedded database and search engine are a feature of the image's
entrypoint, not of the binary: on bare metal you run both yourself and point
`EVEY_DB_HOST` and `EVEY_MEILI_URL` at them.

## Run as a dedicated non-root user

Never run the server as root. Create a group and two unprivileged users, as
the image does:

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) runs the server.
- `evey-plugin` (UID 1001) is the user plugins are dropped to when uid
  separation is active, so a compromised plugin lands as a different
  unprivileged user that cannot read the server's secrets by plain file
  permissions.

!!! warning
    The group id matters: the launcher drops a plugin to UID 1001 and to the
    server's own group, and refuses any uid or gid below 1000. A group
    created by `useradd --system` alone gets a system id below 1000, and
    plugins in `basic` mode are then refused.

The server marks itself non-dumpable at startup (`PR_SET_DUMPABLE=0`), so a
co-resident plugin cannot read `/proc/<server-pid>/{mem,environ}`.

## Pick the sandbox backend for your kernel

Set `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (the default) resolves in the order
landlock, bubblewrap, basic and picks the strongest the host supports. Set
a mode explicitly when you want fail-closed behaviour.

### Landlock (recommended)

A per-path filesystem allowlist (a plugin sees its own directory and the
socket directory, never your data or secrets) plus a Landlock v4 TCP egress
filter. It needs no privilege, no mount and no namespace: the launcher
applies it to itself, then executes the plugin.

Landlock must be compiled in and in the active LSM list. The runner probes
`landlock_create_ruleset`; the probe fails when the syscall is absent
(kernel older than 5.13) or when Landlock is not in the active list. Check:

```bash
uname -r                              # want 5.13 or newer
cat /sys/kernel/security/lsm          # must include "landlock"
```

If `landlock` is missing from that list, add it on the kernel command line
and reboot (keep the existing entries, the list varies per distribution):

```
# e.g. in GRUB_CMDLINE_LINUX:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Then either keep `auto` or pin it:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # fail closed: a plugin is refused when Landlock cannot be enforced
```

`landlock` passes `-require-landlock` to the launcher, which exits non-zero
rather than run a plugin unconfined.

### bubblewrap

When Landlock is not available, install `bwrap`. It puts each plugin in
user, pid, uts, ipc and network namespaces with a mount allowlist. It needs
unprivileged user namespaces and the `mount` syscall, true on most bare-metal
Linux but blocked inside an unprivileged container, which is why Landlock is
the container default. `auto` only selects bubblewrap when Landlock is
absent and a throwaway `bwrap` probe succeeds.

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # namespaces
EVEY_PLUGINS_SANDBOX_MODE=strict      # namespaces plus the seccomp shim
```

Check that the host permits the namespace and mount setup:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (last resort)

`EVEY_PLUGINS_SANDBOX_MODE=basic` is a uid drop plus the seccomp shim; the
launcher, when present, still installs its kernel seccomp denylist and
best-effort Landlock. There is no per-path filesystem confinement: a plugin
runs at UID 1001 with seccomp only. Use it on old or locked-down kernels
where neither Landlock nor bubblewrap works, and rely on the uid separation
below to protect the secrets.

A plugin granted the `subprocess` permission requires the launcher even in
`basic` mode: the shim does not survive an `exec` of a child process, only
the launcher's kernel seccomp does. Without the launcher such a plugin
refuses to start rather than run unconfined.

## Keep secrets out of a plugin's reach

Uid separation runs each plugin under `evey-plugin` (UID 1001), distinct
from the server (UID 1000), so a plugin cannot read `jwt.key`, the backups
or the logs by plain file permissions. It matters in `basic` mode; under
Landlock or bubblewrap the filesystem is already walled off, so it is
redundant there.

`EVEY_PLUGINS_UID_SEPARATION`:

| Value | Behaviour |
|---|---|
| `auto` (default) | on only when the sandbox resolves to `basic` |
| `on` | always, as an extra independent layer |
| `off` | never |

The drop is done by the launcher, not the server. The server stays non-root
with no capability; the launcher carries one file capability and switches to
UID 1001 just before executing the plugin:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

The launcher refuses any target uid or gid below 1000. When it cannot drop
(capability missing, `no_new_privs` set), a plugin in `basic` mode is
refused, since the uid is its only wall there; under Landlock it logs a
warning and runs at the server's uid.

!!! warning
    Uid separation is incompatible with `no_new_privs`. If the server runs
    under systemd `NoNewPrivileges=yes`, the file capability is ignored on
    exec: plugins are refused in `basic` mode, and under Landlock uid
    separation degrades to same-uid. Pick one.

If the server runs as root instead (not recommended), it drops plugins to
1001 directly and the capability is not needed. The plain-language picture
per scenario is in [Plugin sandbox setup](plugins/sandbox-deployment.md).

## Secret file permissions

Mirror the image: secret-bearing directories are owner-only, so a plugin
dropped to UID 1001 cannot read them even without Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# the server owns plugins/ (it extracts installs there); group evey,
# which evey-plugin belongs to, only reads and executes the installed files:
sudo chmod 750 /var/lib/evey/plugins
# owner-only on the secret directories:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# the JWT signing key itself (generated 0600 on first boot):
sudo chmod 600 /var/lib/evey/jwt.key
```

Adjust the paths to wherever you point the `EVEY_*_DIR` variables and
`EVEY_JWT_PRIVATE_KEY_PATH`.

## Configure the environment

All configuration is `EVEY_*` environment variables; [Configuration](configuration.md)
has the full table. Points specific to a bare-metal run:

- Point `EVEY_DB_HOST` and `EVEY_MEILI_URL` at your PostgreSQL and
  Meilisearch. `EVEY_DB_PASSWORD` must not be the default `evey`.
- The Go defaults for paths were chosen for the container: set
  `EVEY_LOG_DIR` (defaults to the relative `logs`), `EVEY_BACKUP_DIR`
  (`/data/backups`), `EVEY_TRANSCODING_CACHE_DIR` (`/data/transcode`),
  `EVEY_TRICKPLAY_CACHE_DIR` (`/cache/trickplay`) and
  `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`) to your own directories.
- The server serves plain HTTP: put a reverse proxy in front for TLS, with an
  `https://` **External URL** and the proxy in **Trusted proxies** (**Admin
  -> Remote Access**; `EVEY_SERVER_BASE_URL` and `EVEY_TRUSTED_PROXIES` are
  the fallbacks). See [Reverse proxy](reverse-proxy.md).
- **The server opens two ports, and only one may be exposed.**
  `EVEY_SERVER_PORT` (default `9090`) is the native gRPC listener; the HTTP
  API your clients and reverse proxy talk to is that port plus one (`9091`).
  The remote-access gate, which decides whether clients outside your LAN may
  connect, is wired into the HTTP chain and does not cover the gRPC
  listener, which therefore binds to loopback only by default
  (`EVEY_GRPC_BIND=127.0.0.1`). If you have a reason to publish it, set
  `EVEY_GRPC_BIND=0.0.0.0`; the server warns at boot that the gate does not
  apply to it, and firewalling it is then on you.
- Migrations run at startup; there is no separate migrate step.
- The plugin resource caps (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) and the sandbox mode are editable at runtime
  from **Admin -> Plugins -> Resource limits**; the environment values are
  the initial defaults.
- The admin panel's server restart relies on the image's supervisor and
  reports itself unavailable in a bare run: restart the service yourself.

## Verify the sandbox

The server logs the backend it resolved. After the first plugin starts, look
for:

- once, for `auto`: `plugin sandbox auto-detection complete
  selected_mode=landlock` (or `bubblewrap`, `basic`);
- at every plugin start: `plugin sandbox applied sandbox_mode=landlock
  plugin_id=...`;
- the warning you do not want when you expected Landlock: `plugin sandbox
  auto-detection fell back to basic mode: no per-path filesystem isolation
  available ...`, meaning neither Landlock nor a working `bwrap` was found.
  Re-check the backend section above.

Confirm the kernel side independently:

```bash
cat /sys/kernel/security/lsm              # lists "landlock"
grep Seccomp: /proc/<plugin-pid>/status   # 2 = a seccomp filter is active on the plugin
```

A `Seccomp: 2` line on a live plugin process confirms the kernel filter is
installed. With `EVEY_PLUGINS_SANDBOX_MODE=landlock` and Landlock not
enforced, the plugin refuses to start: that is the fail-closed contract
working.

For backups, updates and the recovery commands, the container pages apply
as they are: [Backups and updates](backups.md), [Recovery](runbooks/recovery.md)
and the [command line](runbooks/cli.md) describe the same `evey admin`
binary.

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/en/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
