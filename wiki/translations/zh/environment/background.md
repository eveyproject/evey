---
source_sha: d0631abae137226c008a3954088887193957c49501d53f8c38282e9314512c14
reviewed: true
---
# 后台工作

这些变量控制服务器自己运行的各项处理，以及它们如何共用处理器。每项处理的作用，以及如何保证播放不受它们影响：
见[后台工作和服务器负载](../operations/background-work.md)。

## CPU 份额

后台任务可以使用的处理器份额，在**管理 → 服务器配置 → 处理能力**里设置，实时生效，不用重启；它没有对应的环境变量。
下面两个变量可以手动设定某个工作池的大小，这个池就不再受这个份额的约束。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | 根据份额 | 扫描时同时分析的文件数。 | 无 |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | 根据份额 | 同时分析片头和片尾的影片数。 | 无 |

## 各项处理

每个变量都有一个键，也可以在**管理 → 服务器配置 → 配置条目**里作为一行添加，那里会提示这些键；这一行优先于环境变量。
同一页面上的**后台工作**卡片可以设置其中主要的几项。它们都在启动时读取，所以不管用哪种方式修改，都要到下次重启才生效。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | 开启 | 有视频在播放时，暂停后台工作。`off` 让它继续运行。 | 配置条目 `transcoding.pause_bg_on_playback`，需重启 |
| `EVEY_KEYFRAME_BACKFILL` | 开启 | 为扫描过的文件建立关键帧索引，并判断它们的帧率类型。直接播放和直接复制都依赖它。`off` 关闭它。 | 配置条目 `transcoding.keyframe_backfill`，需重启 |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | 服务器默认值 | 每一轮处理的文件数（> 0）。 | 配置条目 `transcoding.keyframe_backfill_batch`，需重启 |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | 服务器默认值 | 两个文件之间间隔的毫秒数（>= 0）。 | 配置条目 `transcoding.keyframe_backfill_pause_ms`，需重启 |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | 服务器默认值 | 空闲时两轮之间间隔的秒数（> 0）。 | 配置条目 `transcoding.keyframe_backfill_idle_sec`，需重启 |
| `EVEY_LEVEL_BACKFILL` | 开启 | 对于在开始收集这项信息之前就扫描过的文件，读取它们的 H.264/HEVC 级别（level）。`off` 关闭它。 | 配置条目 `transcoding.level_backfill`，需重启 |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | 服务器默认值 | 同样的节奏设置，用于这项处理。 | 配置条目 `transcoding.level_backfill_batch`、`_pause_ms`、`_idle_sec`，需重启 |
| `EVEY_SNAPSHOT_FASTPATH` | 开启 | 用服务器进程内的快速解码器提取缩略图。`off` 只使用精确的 ffmpeg 提取方式，用来在缩略图显示了错误的画面时排除快速解码器的问题。 | 配置条目 `transcoding.snapshot_fastpath`，需重启 |
| `EVEY_TITLE_PROBE` | 开启 | 在后台以低优先级探测影片，播放时暂停。`off` 关闭它。 | 配置条目 `transcoding.title_probe`，需重启 |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
