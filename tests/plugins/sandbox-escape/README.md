# Sandbox-escape red-team fixtures

Unlike `tests/fixtures/plugins/malicious/` (which exist to be **caught by the
static analyzer** at install), these plugins are designed to **pass static
analysis** and then attempt a real escape **at runtime** - to validate the
*second* line of defense (the Landlock filesystem/network sandbox + the kernel
and `LD_PRELOAD` seccomp denylists + the userspace resource monitor), i.e. that
a plugin which sneaks past the analyzer is still contained.

Every sensitive target is assembled at runtime (no flagged literal), so each
fixture produces **0 Critical** findings and installs successfully. Each then
runs its probes, logs `[ESCAPE]`/`[MINER] <name>: BLOCKED|BYPASSED`, and exits.
They never bind the plugin socket, so the runner marks them **failed** - that is
expected; the result is in the logs.

Read the results:
```bash
docker exec evey-server sh -c 'grep -hE "\[ESCAPE\]|\[MINER\]" /data/logs/plugins/* /data/logs/*.log' | tail -60
```

## Expected results under `sandbox_mode=landlock` (kernel ABI v4)

| Fixture | Probe | Expected | Enforced by |
|---|---|---|---|
| escape-fs | own_dir_CONTROL | BYPASSED | (control - own dir is allowed) |
| escape-fs | read_server_secret / list_data_root / list_all_plugins / write_into_data | **BLOCKED** | Landlock |
| escape-proc | proc_enumeration / steal_server_env | **BLOCKED** | Landlock (/proc denied) |
| escape-proc | signal_pid1 | **BLOCKED** | kernel + LD_PRELOAD seccomp deny `kill(2)` (and `tkill`/`tgkill` to PID 1) |
| escape-net | connect_* (postgres/meili/server/external) | **BLOCKED** | seccomp `-deny-inet` (no `network` perm) + Landlock v4 TCP filter |
| escape-exec | spawn_subprocess | **BLOCKED** | LD_PRELOAD seccomp denies `execve`/`execveat` (plugin lacks `subprocess` perm) |
| escape-exec | child_reads_secret | **BLOCKED** | spawn is denied; even if it weren't, a child inherits the Landlock domain |
| probe-miner | persist_* / replicate_* | **BLOCKED** | Landlock + DAC |
| probe-miner | detached_survivor | **BLOCKED** | same `execve` denial - a detached child cannot be spawned |
| probe-miner | resource_cpu_burn | de-prioritised | `nice` at spawn (`EVEY_PLUGINS_CPU_NICE`); monitor logs sustained pegging |
| probe-miner | resource_mem_balloon | **STOPPED** past the cap | userspace resource monitor (`EVEY_PLUGINS_MEMORY_MB`) - stops the plugin, server survives |
| escape-datastore | own_data_store_CONTROL | BYPASSED | (control - a plugin owns its own data, no permission needed) |
| escape-deferred | deferred_spawn | **BLOCKED** | seccomp `execve` denial - a payload armed on a timer/after N events is contained exactly like an immediate one (the "passed static, fired later" case) |
| escape-oom | balloon (retained memory grows unbounded) | **STOPPED past the cap** | userspace resource monitor (`EVEY_PLUGINS_MEMORY_MB`) stops the plugin; the server survives |
| escape-datastore | cross_plugin_data_read | **BLOCKED** | data RPCs take the plugin id from the authenticated socket - no API to name another plugin |
| escape-datastore | getmediaitem_without_perm | **BLOCKED** | HostService permission check (no `media.read` declared) |

## Process/syscall gaps - now closed by seccomp

Landlock alone provides **filesystem + network** isolation but **not** process/PID
isolation or syscall filtering. Those gaps (`signal_pid1`, `spawn_subprocess`,
`detached_survivor`) were closed by layering seccomp on top of Landlock, and now
report **BLOCKED**:

- `spawn_subprocess` / `detached_survivor` - the `LD_PRELOAD` `libevey-sandbox.so`
  shim `EPERM`s `execve`/`execveat` for a plugin without the `subprocess`
  permission, so it cannot spawn (detached or otherwise) at all.
- `signal_pid1` - `kill(2)` is denied outright by both the kernel seccomp
  denylist the launcher installs pre-`execve` (so it covers static/compiled
  plugins too) and the `LD_PRELOAD` shim; `tkill`/`tgkill` are additionally
  denied when their target is PID 1. `raise()`/`abort()` and pthread
  self-signalling still work (a plugin's own tid is never 1).

Defence in depth still holds even where a `subprocess`-permitted plugin legitimately
spawns: the child **inherits the Landlock domain** (`child_reads_secret` = BLOCKED),
so the filesystem/network jail is never escaped.

The Go regression test `internal/adapters/driven/pluginrunner/runner_redteam_test.go`
runs these fixtures through the real launcher argv and asserts the verdicts (seccomp
layers everywhere; Landlock-gated probes assert only on a Landlock-capable kernel).
