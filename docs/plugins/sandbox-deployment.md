# Plugin sandbox setup

Which sandbox setup fits your server, in plain words. For almost everyone
the answer is: leave the defaults. The server detects the machine and picks
the strongest isolation it can. Read on only if you run an old NAS, run Evey
without Docker, or want stronger isolation. The full technical model
is in [Security model and plugin sandbox](../SECURITY.md).

## What protects you

A plugin you install runs isolated. Depending on the machine, the isolation:

- **limits which files it can touch**: its own folder and the system
  libraries it needs, nothing else (not your media, not your secrets, not
  other plugins);
- **blocks dangerous system actions** at the kernel: debugging other
  processes, loading modules, changing the clock, mounting disks;
- **cuts off direct network access**, always: a plugin that asked for the
  network, and that you approved, reaches only the hosts it declared, through
  the server;
- **caps CPU, memory, disk and process count**, so a plugin that uses too much cannot
  take the server down;
- **keeps your secrets unreadable**: the login-signing key and the backups.

On top of that, plugins are admin-only to install, scanned for suspicious
code, and you approve their permissions before they run.

**Check what is in force:** **Admin -> Plugins -> Security Dashboard** shows
the host sandbox, the mode `auto` resolved to (`landlock`, `bubblewrap` or
`basic`) with the probe results (**Landlock kernel**, **Sandbox launcher**,
**Bubblewrap**). `basic` means the host offers no per-path file isolation:
plugins run with a uid drop and seccomp only, the probes say why, and the
sections below say what to change.

## Pick your situation

| Your server | What you get out of the box | Anything to do? |
|---|---|---|
| **Modern Linux in Docker** (kernel 5.13 or newer, roughly anything from 2021 on), the normal case | the strongest isolation (Landlock), automatically | nothing |
| **Old NAS in Docker** (kernel older than 5.13) | good isolation without the per-folder file restriction; secrets, network and system actions stay locked down | nothing: uid separation (below) turns itself on |
| **Any Linux, no Docker** | the strongest isolation available: Landlock on a modern kernel, otherwise bubblewrap namespaces | follow [Without a container](../deployment-bare-metal.md) |

## The one setting you might change: uid separation

Uid separation makes a plugin run under a different user account than the
server, so it cannot read the server's secret files even if
nothing else stopped it. It only matters on a machine without the
per-folder file restriction (the `basic` case); on a modern machine that restriction
already keep secrets out of reach, and the separation is a redundant extra
layer.

`EVEY_PLUGINS_UID_SEPARATION`:

| Value | Meaning | Use it when |
|---|---|---|
| `auto` (default) | on only when it is needed (the `basic` case), off where it is redundant | always, unless one of the two cases below applies |
| `on` | always on, a second, independent protection on top of the file restriction | you want both protections and accept the small cost below |
| `off` | never | a specific plugin misbehaves when run under it (rare) |

!!! warning
    Uid separation is incompatible with the `no-new-privileges` Docker
    option: pick one. When the drop cannot be granted on an old machine
    (`basic`), the server refuses to start the plugin rather than run it
    next to your secrets; on a modern machine it logs a warning and runs the
    plugin without the extra protection. The image ships everything needed, you
    only set the variable.

The small cost of `on`: the plugin runs as a separate account, so a plugin
that writes scratch files inside its own folder can fail. If one does,
set `off`. That is why the default is `auto`: it only takes that risk where
there is a real benefit.

## What a malicious plugin could do

**Modern Linux (Docker or bare metal):** a malicious plugin is fully isolated. It
cannot read your secrets, cannot see or touch other plugins' files, cannot
make arbitrary network calls and cannot tamper with the system. The only
realistic way out is a serious kernel bug, true of any sandbox that is not a
full virtual machine.

**Old NAS in Docker, no per-folder file restriction (`basic`):**

- your secrets stay protected (login key, backups) by the separate user
  account, which `auto` turns on here;
- the plugin has no direct network; a permitted one reaches only the hosts it
  declared;
- dangerous system actions are still blocked at the kernel;
- but with no per-folder file restriction, a malicious plugin could read ordinary, non-secret
  files, list running processes and, because plugins share one account,
  read or disturb another plugin's files.

Only install plugins you trust on such a host, or move to a modern kernel
for the full file restriction.

## Configuration examples

Each line goes under `environment:` in the compose file, written
`KEY: value` (for example `EVEY_PLUGINS_UID_SEPARATION: "on"`).

```bash title="1. Modern server, recommended (nothing to do)"
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available (default)
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

```bash title="2. Old NAS (nothing to set, shown for clarity)"
# keep no-new-privileges out of the compose security_opt, it would block the drop
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (default) on in the basic case
```

```bash title="3. Both protections on a modern server"
EVEY_PLUGINS_SANDBOX_MODE=auto          # resolves to landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid separation on top
```

```bash title="4. Pin a mode (advanced)"
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable
```

```bash title="5. A plugin breaks under uid separation"
EVEY_PLUGINS_UID_SEPARATION=off
```

```bash title="6. Resource caps (editable later from Admin -> Plugins -> Resource limits)"
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes and threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install-dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

## Change the sandbox mode while running

Change **Sandbox mode** in the **Resource limits** card of **Admin ->
Plugins**. Running plugins are restarted automatically so the new mode takes
effect at once, and the **Security Dashboard** shows the mode in force. A
plugin that cannot run under the new mode (for example `bubblewrap` inside
an unprivileged container) shows up as errored: switch back.

## Verify it is working

```bash
# the launcher carries the capability used for uid separation
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# which user a running plugin runs as (evey = shared with the server,
# evey-plugin = separated); the image has ps, so `docker exec evey-server ps` also works
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

The sandbox mode in force is the badge of **Admin -> Plugins -> Security
Dashboard**.

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/en/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
