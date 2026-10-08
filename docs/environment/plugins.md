# Plugins and sandbox

The variables behind how plugins are confined and limited. The defaults fit
a standard Docker host; which setup fits which host is in
[Plugin sandbox setup](../plugins/sandbox-deployment.md), and how the layers
work in [Plugin sandbox and threat model](../SECURITY.md).

## Sandbox

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | The plugin subsystem. `false` turns it off entirely. | none |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` or `basic`. `auto` picks the strongest the host supports; an explicit `landlock` refuses a plugin it cannot confine. | Plugins → Resource limits → **Sandbox mode**, applies to plugins started afterwards |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` or `off`: runs plugins under a separate user. `auto` does it only when the sandbox resolves to `basic`. Incompatible with `no_new_privs` on the container. | none |

## Resource limits

These are the starting values; the ones saved in **Admin → Plugins →
Resource limits** apply to every plugin and win over them.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Memory per plugin, in MiB. | Plugins → Resource limits → **Max memory**, live |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Processes and threads per plugin. | Plugins → Resource limits → **Max processes**, live |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Size of a plugin's directory, in MiB; `0` = no limit. | Plugins → Resource limits → **Max disk**, live |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU priority, 0 to 19; higher is lower priority. | Plugins → Resource limits → **CPU nice**, applies to plugins started afterwards |

## Signatures

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` or `enforce`: what a failed signature check does. `enforce` without a key refuses to start. | none |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | empty | Path to the PEM public key signatures are checked against. With `warn` and no key, nothing is verified and the server warns at startup. | none |

## Logs

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Days a plugin's own log is kept. | none |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; WarnValidate (enum values, cosign enforce without key, cosign warn without key)
panel: internal/app/plugin/resource_config.go (plugins.memory_mb, pids_max, disk_max_mb, cpu_nice, sandbox_mode ; live on next sweep / next start) ; i18n admin.plugins.resourceLimits.*
docs/SECURITY.md
-->
