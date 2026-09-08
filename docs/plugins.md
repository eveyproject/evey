# Plugin System

Evey plugins extend the server with metadata providers, media processing,
webhooks, notifications, frontend modules, and CSS themes. A plugin runs as an
**isolated subprocess** that talks to the host over gRPC on a Unix domain
socket; any UI it ships is rendered in a **sandboxed iframe** and talks to the
host through a `postMessage` bridge.

This page is a map. The detail lives in three focused documents:

| Doc | What's in it |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | Writing your first plugin: prerequisites, scaffold, the install/iterate loop, signing, submitting to the catalog, troubleshooting. **Start here.** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | The contract: manifest schema, backend IPC + data store, the `evey.*` frontend bridge, the slot list, and every capability (`metadata.provider`, `media.read/write`, `notification.sender`, `webhook.receiver`, `public`, `prefs`). |
| **[plugins/security-model.md](plugins/security-model.md)** | Threat model and defense layers: UID separation, the kernel + LD_PRELOAD seccomp denylist, the sandbox modes (`auto` resolves landlock → bubblewrap → basic; `strict` adds the LD_PRELOAD lib to bubblewrap), the userspace resource monitor, static analysis, the CSS sanitizer, signing/versioning, the slot whitelist, and known limits. |

For the authorization model behind plugin tokens (deny-by-default, hard-locked
scopes, the token-never-in-iframe bridge) see
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md);
for resource caps see [adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md).

## Architecture

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

A plugin can be written in **any language the `entrypoint:` can launch** - the
runner doesn't constrain it. Pre-generated gRPC SDK stubs ship for Python,
Node.js, PHP, Ruby, Dart, Java, and Swift (`sdk/<lang>/`, auto-copied at
install); Go has no bundled stubs, generate them with `protoc-gen-go` and
`protoc-gen-go-grpc`; interpreter languages without bundled stubs (Bash, Lua, Perl, …) work
by invoking the interpreter directly, and compiled binaries (Go, Rust, …) work
with the `native-binary` permission. Per-language setup:
[plugins/dev-setup.md](plugins/dev-setup.md#per-language).

The three pillars:

- **Capabilities & scopes** - a plugin declares the capabilities it needs in its
  manifest; the admin grants them at install. The host enforces them
  deny-by-default on every call. Full catalog in
  [plugins/api-reference.md](plugins/api-reference.md#capabilities).
- **Sandbox** - backend processes run sandboxed (filesystem/syscall/network
  confinement) under a userspace resource monitor; frontend code runs in a
  locked-down iframe. Details in [plugins/security-model.md](plugins/security-model.md).
- **Trust** - plugins are SHA256-pinned and can be Cosign-signed; the server can
  be set to require signatures. See
  [plugins/security-model.md](plugins/security-model.md#update-workflow).

## Enabling the subsystem

Plugins are on by default (`EVEY_PLUGINS_ENABLED=true`). The sandbox mode is
`EVEY_PLUGINS_SANDBOX_MODE` (default `auto`, which resolves to Landlock on the
official image). Resource caps (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
`_DISK_MAX_MB`, `_CPU_NICE`) are admin-editable from **Admin → Plugins**.
See [configuration.md](configuration.md) and [plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox).
