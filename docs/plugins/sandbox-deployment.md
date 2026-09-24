# Plugin sandboxing - which setup for your server

Evey runs third‑party plugins inside a sandbox. **For almost everyone the answer
is: leave the defaults.** The server detects your machine and picks the strongest
isolation it can, automatically. Read on only if you run an **old NAS**, run
Evey **without Docker**, or want to **tighten things further**.

For the full technical model see [SECURITY.md](../SECURITY.md). This page is the
plain‑language decision guide.

---

## What protects you (in plain words)

A plugin you install is boxed in. The box, depending on your machine:

- **Limits which files it can touch** - it sees its own folder and the system
  libraries it needs to run, nothing else (not your media, not your secrets, not
  other plugins).
- **Blocks dangerous system actions** - debugging other processes, loading kernel
  modules, changing the clock, mounting disks, etc. are refused at the kernel.
- **Cuts off the network** unless the plugin explicitly asked for it (and you
  approved that permission).
- **Caps CPU, memory, disk and process count**, so a runaway plugin can't take
  the server down.

**Check what's actually in force:** Admin → Plugins → Security Dashboard shows
the *host sandbox* - the mode `auto` resolved to (`landlock`, `bubblewrap` or
`basic`) with the individual probe results. If it shows **basic**, your host
offers no per-path filesystem isolation (plugins run with UID-drop + seccomp
only); the probes tell you why, and the sections below tell you how to fix it.
- **Keeps your secrets unreadable** - the login‑signing key and backups.

On top of that: plugins are **admin‑only to install**, they're **scanned** for
suspicious code, and you **approve their permissions** before they run.

---

## Pick your situation

| Your server | What you get out of the box | Anything to do? |
|---|---|---|
| **Modern Linux in Docker** (kernel ≥ 5.13 - basically anything from ~2021 on) - *the normal case* | **Strongest isolation** (Landlock), automatically. | **Nothing.** Leave defaults. |
| **Old NAS in Docker** (e.g. older Synology, kernel < 5.13) | Good isolation, **minus the per‑folder file walls**. Secrets, network and system actions are still locked down. | Optional: turn on **uid separation** (below) for belt‑and‑braces secret protection. |
| **Any Linux, no Docker** (bare‑metal) | **Strongest isolation**, automatically (Landlock on a modern kernel, otherwise bubblewrap namespaces). | Follow the [bare‑metal guide](../deployment-bare-metal.md). |

How to know which one you got: open **Admin → Plugins** - the Security panel's
badge shows the sandbox mode actually in force: `landlock` or `bubblewrap` means
strong, `basic` means the "old NAS" case.

---

## The one knob you might touch: uid separation

**What it does, plainly:** makes a plugin run under a **different user account**
than Evey itself, so it physically cannot read Evey's secret files - even if
nothing else stopped it.

**When it matters:** only on an **old machine without the per‑folder file walls**
(i.e. the `basic` case). On modern machines the file walls already keep secrets
out of reach, so uid separation there is just a redundant extra layer.

**The setting** - `EVEY_PLUGINS_UID_SEPARATION`:

| Value | Meaning | Use it when |
|---|---|---|
| `auto` *(default)* | Turns itself **on only when it's actually needed** (the `basic` case). Off where it's redundant. | **Leave this** - it does the right thing per machine. |
| `on` | **Always on**, even on modern machines - a second, independent secret wall on top of the file walls. | You want maximum belt‑and‑braces and accept the small cost below. |
| `off` | **Never.** | A specific plugin misbehaves when run under it (rare). |

**The gotcha:** uid separation is **incompatible with the `no-new-privileges`
Docker hardening option** - pick one. If your setup can't grant it on an old
machine (`basic`), Evey **refuses to start the plugin** rather than run it next to
your secrets; on a modern machine it logs a warning and runs the plugin normally
(you just don't get the extra wall). The official image already ships everything needed; you only set
this variable.

**Small cost of `on`:** the plugin runs as a separate account, so a plugin that
writes scratch files *inside its own folder* could trip up. If one does, set
`off`. (This is why the default is `auto` - it only takes that risk where there's
a real benefit.)

---

## The risks, in plain words

What a **malicious** plugin could and couldn't do - by scenario:

**Modern Linux (Docker or bare‑metal):**
A bad plugin is fully boxed in. It **cannot** read your secrets, **cannot** see
or touch other plugins' files, **cannot** make arbitrary network calls, and
**cannot** tamper with the system. The only realistic way out is a serious Linux
**kernel** bug - true of *any* sandbox that isn't a full virtual machine.

**Old NAS in Docker, no per‑folder walls (`basic`):**
- ✅ Your **secrets stay protected** (login key, backups) - *especially if you
  turn uid separation `on`*.
- ✅ The plugin gets **no network** unless permitted.
- ✅ **Dangerous system actions are still blocked** at the kernel.
- ⚠️ But with no per‑folder walls, a bad plugin **could read ordinary, non‑secret
  files**, **list running processes**, and - because plugins share one account -
  potentially **read or disturb another plugin's files**.
- **Bottom line:** only install plugins you trust on such a host, or move to a
  modern kernel for the full file walls.

---

## Config examples (copy/paste)

These go in your `.env` (Docker compose) or as `EVEY_*` environment variables.

**1. Normal modern server - recommended (do nothing):**
```bash
# Defaults are correct. Optionally make it explicit:
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

**2. Old NAS, maximise secret protection:**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=on          # force the secret wall on
# and do NOT set no-new-privileges in your compose security_opt
```

**3. Maximum belt‑and‑braces on a modern server (stack both walls):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid wall on top
```

**4. Pin a specific mode (advanced):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable (fail‑closed)
```

**5. A plugin breaks under uid separation:**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Resource caps (all admin‑editable later from Admin → Plugins):**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes/threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install‑dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

---

## Changing the sandbox mode while running

You can change the mode from the **Resource limits** card on **Admin → Plugins**.
Running plugins are **restarted automatically** so the new mode takes effect immediately
(no manual disable/enable). The Security badge shows the mode actually in force.
If a plugin can't run under the new mode (e.g. you picked `bubblewrap` on Docker,
where it needs privileges), it shows up as **errored** in the dashboard - just
switch back.

---

## Verify it's working

```bash
# the launcher carries the capability used for uid separation:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# which user a running plugin runs as (evey = shared with server, evey-plugin = separated):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# the sandbox mode actually in force: the Security badge on the Admin → Plugins page.
```

---

## See also

- [SECURITY.md](../SECURITY.md) - the full plugin security model (every layer, the
  per‑mode table, known limits).
- [deployment-bare-metal.md](../deployment-bare-metal.md) - running without Docker.
- [installation.md](../installation.md) - the Docker self‑host walkthrough.
