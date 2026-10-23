---
source_sha: 8225f3cc5ff5a9bf19187b30ea0a5cd214bea962f504708f3de910c38a198b9a
reviewed: true
---
# 监控和日志 { #monitoring-and-logs }

本页面向想查看服务器状况、而不是修复问题的管理员：在哪里看服务器正在做什么，从此刻正在播放的内容，到本月的统计数据，中间还有日志。如果要排查某个具体症状，请从[故障排除](troubleshooting.md)开始。

## 正在播放什么 { #what-is-playing-now }

**管理 → 正在播放**列出所有正在进行的流：谁、在看什么、用哪台设备，以及每条音视频轨是怎么处理的（直接复制、转码，以及负责编码的后端）。管理员可以**停止**某个会话，或者给观众**发送消息**。

仪表盘上的**在线用户**卡片按个人资料列出此刻在线的人，以及每个应用上一次联系服务器是多久以前。如果某台 Chromecast 的电视一直加载着 Evey 的接收端，那么即使它的播放已经结束、已从**正在播放**中消失，它也会继续出现在列表里；**停止此 Chromecast**会给它发送停止命令，电视大约会在 30 秒内执行。

**管理 → 系统健康**显示处理器、内存、带宽和存储，以及转码容量用了多少。它还列出机器的各个磁盘，每个磁盘都带有最近一分钟内一次请求的平均等待时间、忙碌度以及读写速率。等待时间才是要盯住的数字：一块磁盘可以 100% 忙碌却仍然跟得上，而让每次请求比空闲时多等好几倍的磁盘，即便处理器看起来空闲，也会把播放变成缓冲。当磁盘的等待时间至少 20 ms 且至少是空闲等待的 2.5 倍，并且磁盘忙碌或处理器在等它时，该磁盘会被标为**饱和**。CPU 卡片会说明处理器有多少时间花在等待磁盘上。

实时卡片下方的**趋势**根据服务器每分钟记录的样本绘制最近 24 小时、7 天或 30 天：处理器与磁盘等待、内存、带空闲线和饱和线的磁盘等待、磁盘忙碌度与吞吐量、与实际提供的串流并列的网络进出流量、串流与转码，以及媒体库磁盘的占用情况。处理器、磁盘和网络是整台机器的数据：读取同一磁盘的复制任务或相邻服务也会计入。仪表板上有一个**磁盘**卡片，显示等待最久的磁盘。

<figure class="evey-shot" markdown="span">
  ![演示服务器上的“管理 → 系统健康”](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>管理 → 系统健康</strong>，下方是正在运行的后台工作（界面为英文）。</figcaption>
</figure>

## 日志 { #logs }

**管理 → 日志与调试**可以搜索服务器日志，也可以实时跟踪（**实时**），还能用**区域**缩小到某个方面（播放器和转码、播放跳转索引、字幕、片头检测、Chromecast、身份验证、基础设施……）。用户报告问题时，首先应该看这里。

同样的日志也保存在磁盘上，每天一个 JSON 文件，位于 `./evey-data/logs/`（容器里是 `/data/logs/evey-YYYY-MM-DD.log`），当某个文件超过 50 MB（`EVEY_LOG_MAX_SIZE_MB`）时，还会有一个带小时后缀的文件（`evey-YYYY-MM-DD-HH.log`），默认保留 90 天。`docker compose logs evey` 只显示容器在启动过程中输出的内容，服务器无法启动时应查看这些内容。见[故障排除](troubleshooting.md#the-logs)。

## 播放的实际情况如何 { #how-playback-actually-went }

**管理 → 流媒体质量**根据播放器在播放过程中上报的数据生成（缓冲水平、重新缓冲、吞吐量，采用 CMCD 格式）：观众多久等待一次、最常卡顿的影片，以及哪类设备播放困难，时间范围是最近一天或一周。

服务器也会据此采取行动。如果某一类片源总是让观众等待，服务器会在校准得出的上限之外，暂时降低这类片源的最高质量，之后再自动解除限制。正在生效的限制列在**自动质量限制**下，附有原因和到期时间；**重置**会立即取消其中一项。

## 历史和统计 { #history-and-statistics }

- **管理 → 分析**：今天和本周的播放次数、活跃用户、最近 30 天观看最多的影片。
- **管理 → 活动**：播放记录和管理操作的历史，以及正在进行的后台工作。

每种历史记录保留多久，在**管理 → 服务器配置 → 数据与保留**里设置（播放会话、审计日志、服务器指标、流媒体质量）。修改会在下一次每日清理时生效。

## Prometheus 指标 { #prometheus-metrics }

`GET /metrics` 以 Prometheus 文本格式提供服务器的计数器，名称都是 `evey_...`。它接受在 `EVEY_METRICS_SCRAPE_TOKEN` 里设置的固定令牌（以 `Authorization: Bearer <token>` 发送），这样 Prometheus 或 Grafana 无需登录就能从任何地方抓取数据；管理员会话只接受来自容器自身回环地址的请求（[备份、日志和监控](../environment/logs-monitoring.md)）。值得先做成图表的几组指标：

| 指标 | 说明什么 |
| --- | --- |
| `evey_transcodes_active`、`evey_transcode_queue_depth` | 后台转码任务（实时播放以外的）：正在运行的，以及在等待空位的。 |
| `evey_hlsvod_sessions_active`、`evey_hlsvod_sessions_rejected_total` | 正在生成播放片段的 FFmpeg 进程，以及因为服务器已达上限而被拒绝的启动。 |
| `evey_hlsvod_cache_size_bytes`、`evey_hlsvod_cache_evicted_bytes_total` | 磁盘上片段缓存的大小，以及清理程序释放的空间。 |
| `evey_mkvgo_walks_in_flight`、`evey_mkvgo_plan_stats` | 进程内的封装器：正在读取的窗口、排队的读取（`stat="slot_waits"`），见 [Evey 中的 MKVGo](../playback/mkvgo.md#load-replaced-files-and-updates)。 |
| `evey_http_requests_total`、`evey_http_request_duration_seconds` | 按方法和路径统计的请求数和延迟。 |
| `evey_watcher_alive`、`evey_scan_queue_depth` | 每个媒体库的文件夹监视器是否响应了上一次检查（1 或 0），以及等待扫描的文件。 |

## 命令行 { #command-line }

`evey admin doctor` 用一份报告检查整个服务器；其他诊断命令列在[命令行](../runbooks/cli.md)里。

<!-- sources
who's online: frontend/apps/web/src/features/admin/DashboardOnline.tsx (presence heartbeat, profile names, idle, SendCastReceiverCommand) ; i18n admin.online.*
logs: internal/pkg/logger/logger.go - New (file only outside DevMode), rotatingWriter ; config EVEY_LOG_RETENTION_DAYS ; docker-compose.yml ./evey-data:/data
labels: i18n admin.sessions.*, admin.systemHealth.*, admin.logs.where.*, admin.logs.live, admin.qoe.*, admin.analytics.*, admin.activity.*, admin.serverSettings.*RetentionDays, retentionSweepHint
automatic limits: i18n admin.qoe.overrides.*
metrics: internal/pkg/metrics/metrics.go (names) ; internal/adapters/driving/http/auth_middleware.go - httpScrapeOrAdminMiddleware (Bearer scrape token or admin JWT) ; docs/adr/0018-cmcd-qoe-pipeline.md
-->
