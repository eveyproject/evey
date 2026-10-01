---
source_sha: 25e92baf53807987435c8781d87f2af02a8a02782951e049a23e404082ad7980
reviewed: false
---
# 后台任务

服务器自行运行的处理过程，以及它们如何共享处理器。它们的行为：[后台任务与服务器负载](../operations/background-work.md)。

## CPU 份额

后台任务可以使用的处理器份额在 **Admin → 服务器配置 → 分配的算力** 中设置，实时生效，无需重启。它没有对应的环境变量。下面两个变量手动设定某个池的大小，并为它绕过这个份额：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | 根据份额 | 扫描期间并行分析的文件数。 |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | 根据份额 | 同时分析片头和片尾的条目数。 |

## 处理过程

每个变量都有一个键名，也可以在 **Admin → 服务器配置 → 配置条目** 中作为一行设置，面板会提示这些键名；该行优先。全部在启动时读取，同一界面中的 **后台工作** 可以设置其中主要的几个。

| 变量 | 面板键名 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | `transcoding.pause_bg_on_playback` | 开启 | 播放流时暂停后台任务。`off` 让它们继续运行。 |
| `EVEY_KEYFRAME_BACKFILL` | `transcoding.keyframe_backfill` | 开启 | 为已扫描的文件建立关键帧索引并对其帧率分类。直接播放和直接复制依赖于此。`off` 关闭它。 |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | `transcoding.keyframe_backfill_batch` | 服务器默认值 | 每轮处理的文件数（> 0）。 |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | `transcoding.keyframe_backfill_pause_ms` | 服务器默认值 | 文件之间的毫秒数（>= 0）。 |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | `transcoding.keyframe_backfill_idle_sec` | 服务器默认值 | 空闲时两轮之间的秒数（> 0）。 |
| `EVEY_LEVEL_BACKFILL` | `transcoding.level_backfill` | 开启 | 读取在收集这项信息之前扫描的文件的 H.264/HEVC 级别。`off` 关闭它。 |
| `EVEY_LEVEL_BACKFILL_BATCH`、`_PAUSE_MS`、`_IDLE_SEC` | `transcoding.level_backfill_batch`、`_pause_ms`、`_idle_sec` | 服务器默认值 | 同样的节奏设置，用于这个处理过程。 |
| `EVEY_SNAPSHOT_FASTPATH` | `transcoding.snapshot_fastpath` | 开启 | 用进程内的快速解码器提取缩略图。`off` 只使用精确的 ffmpeg 提取器，用于在缩略图显示错误帧时排除它。 |
| `EVEY_TITLE_PROBE` | `transcoding.title_probe` | 开启 | 以低优先级在后台探测条目，播放期间暂停。`off` 关闭它。 |
