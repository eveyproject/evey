# Sandbox-escape red-team fixtures

Unlike `tests/fixtures/plugins/malicious/` (which exist to be **caught by the
static analyzer** at install), these plugins are designed to **pass static
analysis** and then attempt a real escape **at runtime** — to validate the
*second* line of defense (the Landlock sandbox + cgroups), i.e. that a plugin
which sneaks past the analyzer is still contained.

Every sensitive target is assembled at runtime (no flagged literal), so each
fixture produces **0 Critical** findings and installs successfully. Each then
runs its probes, logs `[ESCAPE]`/`[MINER] <name>: BLOCKED|BYPASSED`, and exits.
They never bind the plugin socket, so the runner marks them **failed** — that is
expected; the result is in the logs.

Read the results:
```bash
docker exec evey-server sh -c 'grep -hE "\[ESCAPE\]|\[MINER\]" /data/logs/plugins/* /data/logs/*.log' | tail -60
```

## Expected results under `sandbox_mode=landlock` (kernel ABI v4)

| Fixture | Probe | Expected | Enforced by |
|---|---|---|---|
| escape-fs | own_dir_CONTROL | BYPASSED | (control — own dir is allowed) |
| escape-fs | read_server_secret / list_data_root / list_all_plugins / write_into_data | **BLOCKED** | Landlock |
| escape-proc | proc_enumeration / steal_server_env | **BLOCKED** | Landlock (/proc denied) |
| escape-proc | signal_pid1 | ⚠️ BYPASSED | gap: no PID-namespace isolation |
| escape-net | connect_* (postgres/meili/server/external) | **BLOCKED** | Landlock v4 (no `network` perm) |
| escape-exec | spawn_subprocess | ⚠️ BYPASSED | gap: execve not blocked in landlock mode |
| escape-exec | child_reads_secret | **BLOCKED** | Landlock (child inherits the domain) |
| probe-miner | persist_* / replicate_* | **BLOCKED** | Landlock + DAC |
| probe-miner | detached_survivor | ⚠️ BYPASSED | gap: spawn + detached escapes the process group |
| probe-miner | resource_cpu_burn | de-prioritised | `nice` at spawn (`EVEY_PLUGINS_CPU_NICE`); monitor logs sustained pegging |
| probe-miner | resource_mem_balloon | **STOPPED** past the cap | userspace resource monitor (`EVEY_PLUGINS_MEMORY_MB`) — stops the plugin, server survives |
| escape-datastore | own_data_store_CONTROL | BYPASSED | (control — a plugin owns its own data, no permission needed) |
| escape-datastore | cross_plugin_data_read | **BLOCKED** | data RPCs take the plugin id from the authenticated socket — no API to name another plugin |
| escape-datastore | getmediaitem_without_perm | **BLOCKED** | HostService permission check (no `media.read` declared) |

## Known gaps these surface (and the fix)

The ⚠️ items all stem from Landlock providing **filesystem + network** isolation
but **not** process/PID isolation or syscall filtering:

- `spawn_subprocess` / `detached_survivor` — closed by layering the existing
  `LD_PRELOAD` seccomp filter (which `EPERM`s `execve`) on top of Landlock.
- `signal_pid1` — needs the plugin to run under a different UID than the server
  (so `kill()` is `EPERM`), or true PID-namespace isolation.

Note that even where spawn succeeds, the child **inherits the Landlock domain**
(`child_reads_secret` = BLOCKED), so the filesystem/network jail still holds.
