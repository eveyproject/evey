---
source_sha: ca793f290f46505f8ca71b8dd23267c3488a2611b736e01fe78edbb4254c840d
reviewed: false
---
# 插件与沙箱

插件如何被隔离和限制。各层如何工作：[安全模型与插件沙箱](../SECURITY.md)。

## 沙箱

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | 插件子系统。`false` 完全关闭它。 |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`、`landlock`、`bubblewrap`、`strict` 或 `basic`。`auto` 选择主机支持的最强模式；显式的 `landlock` 会拒绝它无法隔离的插件。可以在 **Admin → 插件** 中修改。 |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`、`on` 或 `off`：以单独的用户运行插件。`auto` 只在沙箱解析为 `basic` 时这样做。与容器上的 `no_new_privs` 不兼容。 |

哪种设置适合哪种主机：[插件沙箱设置](../plugins/sandbox-deployment.md)。

## 资源限制

初始值；之后每个插件的限制在 **Admin → 插件** 中修改。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | 每个插件的内存，单位 MiB。 |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | 每个插件的进程和线程数。 |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | 插件目录的大小，单位 MiB；`0` = 不限。 |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU 优先级，0 到 19；数值越高优先级越低。 |

## 签名

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`、`warn` 或 `enforce`：签名检查失败时怎么做。 |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | 空 | 用于检查签名的 PEM 公钥的路径。策略不是 `off` 且没有密钥时，不会验证任何东西，服务器会在启动时发出警告。 |

## 日志

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | 插件自己的日志保留的天数。 |
