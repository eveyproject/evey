---
source_sha: 4ea9a6d07b743691fa611b4154d0b15ea314d413a0aa20fafe038abaf561a972
reviewed: false
---
# 备份、日志与监控

备份、服务器日志、历史保留多长时间，以及供外部监控系统使用的指标。

## 备份

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | 数据库备份写入的位置。 |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0`（关闭） | 每 N 小时备份一次。只在 **Admin → 备份与恢复 → 计划任务** 中没有设置计划时使用，而后者才是通常的方式。 |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | 比这更旧的备份会在启动时以及由上面的计划删除。在面板中设置的计划有它自己的保留期。 |

见[备份与更新](../backups.md)。

## 日志

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`、`info`、`warn` 或 `error`。**Admin → 服务器配置 → 服务器设置** 会覆盖它。 |
| `EVEY_LOG_DIR` | `/data/logs` | 每日日志文件 `evey-YYYY-MM-DD.log` 写入的位置。由镜像设置。这个文件是参考依据：`docker logs` 显示的是同一个流，但 Docker 会轮转它。 |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | 日志文件轮转前的大小。 |
| `EVEY_LOG_RETENTION_DAYS` | `90` | 日志文件保留的天数。 |

## 历史保留

每一项都会被 **Admin → 服务器配置 → 服务器设置** 中对应的字段覆盖，在下一次每日清理时生效。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0`（永久） | 历史中已结束的播放会话。 |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0`（永久） | 管理操作的审计日志。 |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | 服务器的资源采样（处理器、内存、带宽）。 |
| `EVEY_QOE_RETENTION_DAYS` | `30` | 播放器的流媒体质量报告。 |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | 资源采样的频率。 |

## 外部监控

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | 空 | 除管理员会话之外，`GET /metrics` 还接受的一个静态令牌，这样 Prometheus 或 Grafana 无需续期登录即可抓取。为空：只接受管理员会话。 |
| `EVEY_OTLP_ENDPOINT` | 空 | 发送追踪数据的 OpenTelemetry 端点。为空：不追踪。 |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | 追踪中的服务名称。 |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | 被追踪的请求比例，0 到 1。 |

## 仅用于开发

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | 开发者模式：启动时的安全检查只给出警告，插件可以在没有沙箱库的情况下运行。绝不要在真实服务器上使用；设置了 `EVEY_ENV=production` 时服务器会拒绝启动。 |
