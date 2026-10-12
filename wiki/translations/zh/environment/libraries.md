---
source_sha: 0bcab9ec2d5eb546c48ee32a751bbd41320d9cb2c60cab549b010ec73e26f367
reviewed: true
---
# 媒体库与元数据

这些是扫描、元数据来源和片头检测背后的变量。大多数在管理面板里都有一个对应的字段，
一旦在那里保存了值，就以面板为准；面板这一侧的说明见[媒体库与元数据](../administration/libraries.md)。

## 扫描

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | 实时监视文件变化。 | 服务器配置 → 服务器设置 → **媒体库文件监视**，需重启 |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | 多久检查一次文件夹，找出监视遗漏的内容（网络共享常有这种情况）。 | 服务器配置 → 服务器设置 → **定期扫描间隔**，需重启 |
| `EVEY_LIBRARY_SCAN_WORKERS` | 根据 CPU 份额 | 扫描时同时分析的文件数。手动设置后，扫描就不再受**分配的算力**份额的约束。 | 无 |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | 媒体健康扫描保存结果和进行中任务的位置，重启后仍然保留。为空时：不保存。 | 无 |

## 元数据

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | 标题和简介的语言。在设置向导里选择的语言优先于它。 | 服务器配置 → 语言和地区 → **元数据语言**，实时生效 |
| `EVEY_TMDB_API_KEY` | 内置密钥 | 您自己的 TMDB 密钥，使用您自己的配额。 | 服务器配置 → API 密钥 → **TMDB API 密钥**，需重启 |
| `EVEY_OMDB_API_KEY` | 空 | OMDb 密钥，电影的第二个来源。 | 服务器配置 → API 密钥 → **OMDB API 密钥**，需重启 |
| `EVEY_OPENSUBTITLES_API_KEY` | 空 | OpenSubtitles 密钥，用于通过它的 API 搜索字幕。 | 服务器配置 → API 密钥 → **OpenSubtitles API 密钥**，需重启 |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | 空 | OpenSubtitles 账户，用来登录它的 API。 | 无 |

## 片头、前情提要和片尾

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | 在服务器自己检测之前，先查社区数据库（先 TheIntroDB，再 IntroDB）。 | 服务器配置 → 片头跳过提供商 → **启用外部片头跳过提供商**，需重启 |
| `EVEY_INTRO_TIDB_API_KEY` | 空 | TheIntroDB 密钥：每天可以查询 500 次，而不是 100 次。 | 服务器配置 → API 密钥 → **TheIntroDB API 密钥**，需重启 |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | 每个播放器都自动跳过检测到的片头。 | 服务器配置 → 服务器设置 → **自动跳过片头**，实时生效 |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | 同时提供跳过下一集预告的选项。 | 服务器配置 → 服务器设置 → **跳过预告片段**，实时生效 |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | 根据 CPU 份额 | 同时分析的影片数。手动设置后，这项任务就不再受**分配的算力**份额的约束。 | 无 |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | 检测时每个 ffmpeg 进程使用的线程数（1 到 16）。 | 无 |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
watcher / scan interval: cmd/evey/wire_events.go (loadOverride library.watcher_enabled, library.periodic_scan_interval) ; internal/adapters/driving/http/settings_handler.go (requiresRestart)
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin, resolveAPIKey ; cmd/evey/wire_services.go (opensubtitles) ; i18n admin.serverConfig.apiKeys.*
language: cmd/evey/wire_infra.go (metadata.language wins) ; internal/app/config/locale_defaults.go
intro: cmd/evey/wire.go (intro_providers.enabled, api.tidb_key, EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go ; i18n admin.serverConfig.introProvidersRestart ; admin.serverSettings.introAutoSkip, skipPreview
-->
