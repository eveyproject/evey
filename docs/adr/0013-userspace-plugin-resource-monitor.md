# ADR-0013: Userspace plugin resource monitor (cgroups abandoned)

- Status: Accepted
- Date: 2026-05-23

## Context

Each plugin runs as a child process and needs per-plugin caps on memory, process/thread count, disk footprint, and CPU. The obvious mechanism is cgroups v2.

It does not work in our deployment. Evey ships as a prebuilt image (`gravityz/evey`) that self-hosters run with `docker compose` on machines like a NAS - **non-privileged containers**. cgroup v2 control requires either `--privileged` or explicit cgroup delegation to the container, and we can rely on neither on an arbitrary user's box. Earlier attempts went the wrong way:

- A `su-exec` root-drop entrypoint to manage cgroups - forced the server to start as root.
- `mem_limit` / `cpus` in the default compose - caps the *whole container*, not individual plugins, and is a deployment knob, not a per-plugin policy.

Neither gives per-plugin enforcement, and both push complexity/root onto the operator.

## Decision

Drop cgroups entirely. Enforce caps from **userspace, inside the server process**:

- A monitor goroutine sweeps each plugin's process tree every ~2s, reading `/proc/<pid>/{statm,status,stat}` for RSS, thread/PID count and CPU ticks, and walking the plugin directory for disk usage. Over a cap ⇒ the offending plugin is stopped.
- **CPU** is shaped at spawn with `nice` (default niceness 10) rather than a hard quota - plugins yield to the server under contention.
- The server runs **non-root by default**; the default compose carries **no `mem_limit` / `cpus`**.
- Caps are **admin-editable** (`plugins.*` server settings: memory MB, max PIDs, disk MB, CPU nice, sandbox mode), resolved at boot (`ResolveResourceConfig`) and applied live on change (`ApplyResourceConfig` via the `ServerConfigChanged` subscriber). Startup logs the armed values.

## Consequences

- **Works in a non-privileged container, as a non-root user** - the only environment we can guarantee. No operator setup, no `--privileged`.
- **Enforcement granularity is the sweep interval (~2s).** A plugin can briefly exceed a cap before the next sweep kills it. Acceptable: this is a trust-but-verify boundary for a self-hoster's own plugins, not a hostile multi-tenant scheduler.
- **Disk sizing walks the plugin dir each sweep** (`dirSize`, O(files)). Fine for plugin-sized trees; revisit if a plugin stores large data.
- **Decision of record: do NOT re-attempt cgroups in non-privileged Docker.** Validated on a staging instance (monitor armed, memory cap stops over-limit plugins, process runs `uid=1000`, `nice=10`). Pairs with the Landlock/seccomp filesystem-and-syscall sandbox; this ADR covers resource caps only.

## References

- `internal/adapters/driven/pluginrunner/runner_resmon.go` - sweeper, `checkResources`, `killOverLimit`, `applyNice`, `/proc` readers, `dirSize`
- `internal/adapters/driven/pluginrunner/runner.go` - `limitsSnapshot`, `Set{ResourceLimits,CPUNice,DiskQuota,SandboxMode}`
- `internal/app/plugin/resource_config.go` - `ResolveResourceConfig` / `ApplyResourceConfig`, `plugins.*` keys
- `internal/adapters/driving/http/settings_handler.go` - admin-editable caps
- `frontend/apps/web/src/features/admin/PluginResourceLimits.tsx` - admin UI
