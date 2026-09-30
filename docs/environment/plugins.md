# Plugins and sandbox

How plugins are confined and limited. How the layers work:
[Security model and plugin sandbox](../SECURITY.md).

## Sandbox

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | The plugin subsystem. `false` turns it off entirely. |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` or `basic`. `auto` picks the strongest the host supports; an explicit `landlock` refuses a plugin it cannot confine. Editable in **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` or `off`: runs plugins under a separate user. `auto` does it only when the sandbox resolves to `basic`. Incompatible with `no_new_privs` on the container. |

Which setup fits which host: [Plugin sandbox setup](../plugins/sandbox-deployment.md).

## Resource limits

Starting values; each plugin's limits are then edited in **Admin → Plugins**.

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Memory per plugin, in MiB. |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Processes and threads per plugin. |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Size of a plugin's directory, in MiB; `0` = no limit. |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU priority, 0 to 19; higher is lower priority. |

## Signatures

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` or `enforce`: what a failed signature check does. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | empty | Path to the PEM public key signatures are checked against. With a policy other than `off` and no key, nothing is verified and the server warns at startup. |

## Logs

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Days a plugin's own log is kept. |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; cosign warning in WarnValidate
docs/SECURITY.md
-->
