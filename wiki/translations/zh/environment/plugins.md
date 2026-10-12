---
source_sha: 25ed5f4a742c5f5e2457ec15cbc7d5d59bfd30c52c88a5fc2548a86553d13fb5
reviewed: true
---
# 插件与沙箱

这些变量决定插件怎样被隔离和限制。默认值适合标准的 Docker 主机；哪种设置适合哪种主机，
见[插件沙箱设置](../plugins/sandbox-deployment.md)，各层防护怎样工作，见[插件沙箱与威胁模型](../SECURITY.md)。

## 沙箱

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | 插件子系统。`false` 完全关闭它。 | 无 |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`、`landlock`、`bubblewrap`、`strict` 或 `basic`。`auto` 选择主机支持的最强方式；明确指定 `landlock` 时，无法隔离的插件会被拒绝。 | 插件 → 资源限制 → **沙箱模式**，对之后启动的插件生效 |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`、`on` 或 `off`：用一个单独的用户运行插件。`auto` 只在沙箱最终是 `basic` 时才这样做。和容器上的 `no_new_privs` 不能同时使用。 | 无 |

## 资源限制

这些是初始值；在**管理 → 插件 → 资源限制**里保存的值适用于每个插件，并优先于它们。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | 每个插件的内存，单位 MiB。 | 插件 → 资源限制 → **最大内存 (MiB)**，实时生效 |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | 每个插件的进程和线程数。 | 插件 → 资源限制 → **最大进程数**，实时生效 |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | 插件目录的大小，单位 MiB；`0` = 不限。 | 插件 → 资源限制 → **最大磁盘 (MiB)**，实时生效 |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU 优先级，0 到 19；数字越大，优先级越低。 | 插件 → 资源限制 → **CPU nice (0-19)**，对之后启动的插件生效 |

## 签名

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`、`warn` 或 `enforce`：签名检查失败时怎么办。设为 `enforce` 却没有密钥，服务器拒绝启动。 | 无 |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | 空 | 用来核对签名的 PEM 公钥的路径。设为 `warn` 又没有密钥时，什么都不验证，服务器启动时给出警告。 | 无 |

## 日志

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | 插件自己的日志保留的天数。 | 无 |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; WarnValidate (enum values, cosign enforce without key, cosign warn without key)
panel: internal/app/plugin/resource_config.go (plugins.memory_mb, pids_max, disk_max_mb, cpu_nice, sandbox_mode ; live on next sweep / next start) ; i18n admin.plugins.resourceLimits.*
docs/SECURITY.md
-->
