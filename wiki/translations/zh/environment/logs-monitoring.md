---
source_sha: 84d4aedb514ecf260e351de19cd0ddda1771d2b1ed9173179da8a7b9ff4e192a
reviewed: true
---
# 备份、日志和监控 { #backups-logs-and-monitoring }

这些是数据库备份、服务器日志、历史记录保留多久，以及外部监控系统所用接口背后的变量。
日常操作见[备份和更新](../backups.md)和[监控和日志](../operations/monitoring.md)。

## 备份 { #backups }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | 数据库备份写入的位置。 | 无 |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0`（关闭） | 每 N 小时备份一次，与面板里的计划相互独立；通常使用面板里的计划。 | 数据库备份 → **计划任务**（cron 表达式），实时生效 |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | 设置了上面的间隔时，超过这个天数的备份会被删除，删除发生在启动时和每次按间隔备份之后。面板里的计划有它自己的**保留天数**。 | 数据库备份 → 计划任务 → **保留天数**，实时生效 |

## 日志 { #log }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`、`info`、`warn` 或 `error`。 | 服务器配置 → 常规 → **日志级别**，实时生效 |
| `EVEY_LOG_DIR` | `/data/logs` | 每天的日志文件 `evey-YYYY-MM-DD.log` 写入的位置。由镜像设置（文件超过 50 MB 时会带上小时后缀）。以这个文件为准：`docker logs` 显示的是同样的内容，但 Docker 会对其进行轮换。 | 无 |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | 日志文件达到多大时轮换。 | 无 |
| `EVEY_LOG_RETENTION_DAYS` | `90` | 日志文件保留的天数。 | 无 |

## 历史记录保留 { #history-retention }

每天的清理都会重新读取这些保留期限，所以在**管理 → 服务器配置 → 数据与保留**里保存的值，会在下一次清理时生效。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0`（永久） | 历史记录里已结束的播放会话。 | 服务器配置 → 数据与保留 → **播放会话历史保留期**，下次清理时生效 |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0`（永久） | 管理操作的审计日志。 | 服务器配置 → 数据与保留 → **审计日志保留期**，下次清理时生效 |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | 服务器的资源采样数据（处理器、内存、带宽）。 | 服务器配置 → 数据与保留 → **服务器指标保留期**，下次清理时生效 |
| `EVEY_QOE_RETENTION_DAYS` | `30` | 播放器上报的流媒体质量报告。 | 服务器配置 → 数据与保留 → **播放质量（QoE）保留期**，下次清理时生效 |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | 多久采样一次资源数据。 | 无 |

## 外部监控 { #external-monitoring }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | 空 | 一个固定的令牌，除了管理员会话之外，`GET /metrics` 也接受它，这样 Prometheus 或 Grafana 不用反复登录就能抓取数据。为空时：只接受管理员会话。 | 无 |
| `EVEY_OTLP_ENDPOINT` | 空 | 接收追踪数据的 OpenTelemetry 接口。为空时：不做追踪。 | 无 |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | 追踪数据里的服务名称。 | 无 |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | 被追踪的请求所占的比例，0 到 1（是比例，不是百分数）。 | 无 |

## 仅用于开发 { #development-only }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | 开发者模式：启动时的安全检查只给出警告，插件可以在没有沙箱库的情况下运行。只有同时设置 `EVEY_ENV=dev` 才会生效；单独设置时会被忽略并给出警告。绝不要在真正的服务器上开启。 | 无 |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode, sample ratio)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days) ; internal/adapters/driving/http/settings_handler.go (log.level live via logger.SetLevel ; retention keys) ; i18n admin.serverSettings.logLevel, *RetentionDays, retentionSweepHint
backup: cmd/evey/main.go (StartScheduler env interval + DB cron scheduler both started) ; internal/app/backup/service.go - StartScheduler (returns when interval <= 0 ; CleanupOldBackups first, then each tick) ; i18n admin.backup.schedule.*
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
