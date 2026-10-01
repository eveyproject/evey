---
source_sha: daa61595acb5937c7640f83d1baaec5b538968a8324e20be12058340e8e77c15
reviewed: false
---
# 媒体库与元数据

扫描、元数据来源和片头检测。其中大多数在管理面板中都有对应项，一旦在那里保存了值，就以面板为准。

## 扫描

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | 实时文件监视。 |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | 多久检查一次文件夹，以发现监视器遗漏的变化（网络共享）。**Admin → 服务器配置 → 服务器设置** 会覆盖它。 |
| `EVEY_LIBRARY_SCAN_WORKERS` | 根据 CPU 份额 | 扫描期间并行分析的文件数。手动设置后，扫描会绕过 **分配的算力** 份额。 |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | 媒体健康扫描在重启之间保存其结果和正在运行的任务的位置。为空：不保存。 |

## 元数据

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | 标题和简介的语言。以设置向导或 **Admin → 服务器配置 → 语言和地区** 中选择的语言为准。 |
| `EVEY_TMDB_API_KEY` | 内置密钥 | 你自己的 TMDB 密钥，使用你自己的配额。以 **Admin → 服务器配置 → API 密钥** 中设置的密钥为准。 |
| `EVEY_OMDB_API_KEY` | 空 | OMDb 密钥，电影的第二个来源。同样可被覆盖。 |
| `EVEY_OPENSUBTITLES_API_KEY` | 空 | OpenSubtitles 密钥，用于通过 API 搜索字幕。同样可被覆盖。 |
| `EVEY_OPENSUBTITLES_USERNAME`、`EVEY_OPENSUBTITLES_PASSWORD` | 空 | OpenSubtitles 账户，用于登录其 API。 |

## 片头、回顾与片尾

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | 在服务器自己的检测之前，先使用社区数据库（先 TheIntroDB，再 IntroDB）。以 **Admin → 服务器配置 → 片头跳过提供商** 为准；修改后需重启。 |
| `EVEY_INTRO_TIDB_API_KEY` | 空 | TheIntroDB 密钥：每天 500 次查询，而不是 100 次。 |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | 每个播放器都自动跳过检测到的片头。从面板实时生效。 |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | 还提供跳过下一集预告的选项。从面板实时生效。 |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | 根据 CPU 份额 | 同时分析的条目数。手动设置后，这项任务会绕过 **分配的算力** 份额。 |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | 检测期间每个 ffmpeg 进程的线程数（1 到 16）。 |
