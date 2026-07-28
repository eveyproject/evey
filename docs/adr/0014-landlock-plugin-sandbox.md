# ADR-0014: Landlock plugin sandbox via a pre-exec launcher shim

- Status: Accepted
- Date: 2026-05-24
- Pairs with: [ADR-0013](0013-userspace-plugin-resource-monitor.md) (resource caps), [ADR-0012](0012-plugin-token-deny-by-default.md) (token authz)

## Context

Plugins run as separate OS processes ([ADR-0003](0003-plugin-scoped-tokens.md)). They need filesystem, syscall, and network confinement so that a plugin which slips past the install-time static analyzer still cannot read `/data` (JWT key, DB socket, backups), call dangerous syscalls, or exfiltrate over the network.

The original mechanism was **bubblewrap** (3 modes: basic/bubblewrap/strict). It works, but it needs `mount`/`unshare` of new namespaces, which Docker's **default** seccomp + AppArmor profile blocks (`mount` is gated on `CAP_SYS_ADMIN`). Making bwrap work inside a container therefore required relaxing that profile (`security_opt: seccomp=…` allowing `clone`/`unshare`/`mount`/…) - a container-level security downgrade. Evey ships as a prebuilt image that self-hosters run with `docker compose` on machines like a NAS, where we can rely on neither `--privileged` nor a relaxed seccomp profile. bwrap effectively worked only bare-metal, i.e. almost nowhere for our users.

We need confinement that works in an **ordinary non-privileged container with zero host setup**, degrades gracefully on old kernels, and is auditable.

## Decision

Default the sandbox to **Landlock**, applied by a tiny pre-exec launcher shim, and layer independent defenses on top so no single one is the perimeter.

- **`evey-plugin-launcher`** - a dependency-free Go binary (`flag` + `x/sys/unix` only) prepended to the plugin argv. Before `execve`-ing the plugin it: drops to UID 1001 if requested (carries `cap_setuid`; the server stays unprivileged), applies optional rlimits, installs the **Landlock** filesystem allowlist (system dirs read-only; the plugin's own dir + socket dir read-write; everything else, notably `/data` and sibling plugins, denied) and the Landlock v4 TCP egress filter, then installs the **kernel seccomp denylist** (`-seccomp`, plus `-deny-inet` for non-network plugins and `-deny-fork` for non-`subprocess` ones - process creation blocked, thread creation untouched). The rlimit flags stay operator-only and are never injected automatically: `RLIMIT_NPROC` is per-UID and `RLIMIT_AS` caps virtual address space, so both are unusable as per-plugin quotas on a shared uid with a V8 runtime (see `runner_sandbox.go`). Landlock self-confines the calling thread from *inside* the process - no `mount`, no `unshare`, no privilege - so it runs cleanly under Docker's default seccomp + AppArmor.
- **Seccomp installed by the launcher, pre-`execve`** so it also constrains **static/compiled plugin binaries**, which never load the `LD_PRELOAD` `libevey-sandbox.so` shim and used to bypass that layer. The `LD_PRELOAD` shim is kept as belt-and-suspenders for dynamic plugins and additionally denies `execve`/`execveat` for plugins without the `subprocess` permission.
- **`auto` is the default mode** and resolves once at first plugin start, **landlock → bubblewrap → basic**: Landlock when the kernel enforces it and the launcher is present; bubblewrap only where Landlock is absent *and* a throwaway bwrap probe builds (bare-metal / privileged); basic otherwise. `basic` is not "no isolation" - UID separation, the kernel + `LD_PRELOAD` seccomp, `-deny-inet`, best-effort Landlock, and the resource monitor still apply; it only loses *per-path FS confinement*.
- **bubblewrap is retained as an advanced opt-in** (`sandbox_mode=bubblewrap`/`strict`) for bare-metal operators who want namespace isolation; the relaxed seccomp profile and its Dockerfile `COPY` are kept out of the default compose.

## Consequences

- **Works in a non-privileged container with zero operator setup** - the only environment we can guarantee. No `security_opt` relaxation, no `--privileged`, no bind-mounts, no two-step install.
- **Fail-closed where it matters.** `sandbox_mode=landlock` (and `auto` once it has resolved to landlock) refuses to start a plugin if Landlock can't be enforced; for an interpreted plugin without `subprocess`, a missing `libevey-sandbox.so` also refuses the start.
- **Graceful degradation on kernels < 5.13** (e.g. Synology/QNAP): `auto` falls back to `basic`, where secrets stay protected by UID separation + `PR_SET_DUMPABLE=0` + the kernel seccomp denylist (incl. `-deny-inet`), but there is no per-path FS confinement. The resolved mode is logged at first plugin start and surfaced in the admin security dashboard.
- **Direct network egress is cut for ALL plugins; egress is brokered** (see the 2026-07-11 update below). This supersedes the earlier per-port TCP filter + `external_fetch` DNS allowlist, which could not stop IP-literal egress to link-local (`169.254.169.254`) or LAN addresses.
- **Defense in depth, not a hostile-multi-tenant jail.** This is a trust-but-verify boundary for community/third-party plugin code on a self-hoster's own box, layered with the resource monitor ([ADR-0013](0013-userspace-plugin-resource-monitor.md)) and token authz ([ADR-0012](0012-plugin-token-deny-by-default.md)). It is not a guarantee against a determined attacker exploiting a kernel LSM/seccomp 0-day.

## Update (2026-07-11): direct egress cut, host `Fetch` broker

The per-port TCP filter + `external_fetch` DNS allowlist left an SSRF hole: a plugin with the `network` permission could connect to a **literal IP** (cloud metadata `169.254.169.254`, or any LAN host), which `/etc/hosts` (host-based) and Landlock (port-based) do not stop. Kernel LSM primitives cannot filter egress by resolved IP without privilege the target container lacks, and an `LD_PRELOAD` `connect()` hook is bypassable by static binaries.

Decision: **remove direct plugin egress entirely and broker it host-side.**

- Every sandboxed plugin now runs with `--unshare-net` (bwrap) and `-deny-inet` + `-restrict-net` with no `-net-connect` grant (landlock), **regardless of the `network` permission**. The plugin process can open no IP socket at all - which also covers static/compiled plugins, since there is no network namespace to abuse.
- A plugin that needs the network calls the new `HostService.Fetch` RPC over its **AF_UNIX** host socket (unaffected by the net-namespace unshare). The host (`internal/adapters/driving/pluginhost/fetch.go`) enforces, in Go: the plugin's `external_fetch` allow-list (now a real per-host contract, not DNS theatre), http/https + GET/HEAD only, the SSRF-safe dialer (`internal/pkg/ssrf`, rejects private/reserved IPs after DNS resolution at connect time, so redirects are re-checked), no ambient credentials, and a response-size cap. The RPC is gated on the `network` permission (fail-closed) in `methodPermissions`.
- The obsolete DNS-whitelist plumbing (`buildHostsFile`, `landlockConnectPorts`, `runner_dns.go`) was removed.

**Residual + operator layer.** The broker relocates the SSRF surface to the host, so its Go validation is the security-critical chokepoint. Because a code bypass should not reach internal infrastructure, operators SHOULD add an independent egress layer on the `evey-server` container that does not trust the code: drop `169.254.0.0/16` (metadata/link-local) outright and restrict the internal subnet to the DB + Meili endpoints. A code bug then still hits a network wall, and the two layers fail orthogonally. The worst-case failure of the egress cut itself is benign - a `network` plugin that cannot reach the broker degrades gracefully (e.g. weather-v2 falls back to its cached image).

## References

- `cmd/evey-plugin-launcher/{main.go,landlock_linux.go,seccomp_linux.go,droppriv_linux.go}` - the shim
- `sandbox/sandbox.c` - the `LD_PRELOAD` seccomp shim (`libevey-sandbox.so`)
- `internal/adapters/driven/pluginrunner/runner_landlock.go` - `buildLauncherArgv`, `applyLandlockSandbox`, `resolveAutoMode`
- `internal/adapters/driven/pluginrunner/runner_bwrap.go` - the retained bubblewrap backend
- `internal/adapters/driving/pluginhost/fetch.go` - the host `Fetch` broker (allow-list + SSRF-safe egress)
- `internal/pkg/ssrf/dialer.go` - the SSRF-safe HTTP client the broker dials through
- `cmd/evey/hardening_linux.go` - server `PR_SET_DUMPABLE=0`
- `docs/SECURITY.md` (Sandbox modes), `docs/plugins/security-model.md`
- Regression: `internal/adapters/driven/pluginrunner/runner_redteam_test.go`, `tests/plugins/sandbox-escape/`
