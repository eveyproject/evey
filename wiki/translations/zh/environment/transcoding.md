---
source_sha: 6bbcf7c54732dc7eb7955206675ddd1923616486ecdec3c988c6ad0463cf8a04
reviewed: true
---
# 转码与硬件

这些是服务器容量、显卡、画质上限和转码缓存背后的变量。大多数安装一个都不用设置：
服务器会自己测量这台机器，并自己选择硬件，见[转码与服务器容量](../playback/transcoding.md)。
如果某项设置在**管理 → 转码**里也有，就以那里保存的值为准。

## 硬件加速

如何把 GPU 交给容器：见[硬件加速](../administration/hardware-acceleration.md)。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | 空 | `auto`、`off`、`nvenc`、`vaapi`、`qsv` 或 `rkmpp`。为空时：根据下面两个变量推断。 | 转码 → **硬件加速**，实时生效（会重新测量） |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | 旧的开关。 | 转码 → **硬件加速**，实时生效 |
| `EVEY_TRANSCODING_HW_BACKEND` | 空（自动检测） | 固定使用一个后端（`nvenc`、`vaapi`、`qsv`）：只测量这一个。这样，同一台主机上的几个 Evey 容器可以各走一条不同的 GPU 路线。 | 无 |
| `EVEY_TRANSCODING_HW_DEVICE` | 空（全部） | 固定使用一个设备：一个渲染节点（`/dev/dri/renderD129`）或一个 CUDA 编号（`1`）。配合 `EVEY_TRANSCODING_HW_BACKEND`，可以在有多块 GPU 的主机上把一个容器绑定到其中一块。 | 无 |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | 硬件转码在这段时间内还没有产出第一个片段，就被当作 GPU 卡住了，改由处理器接手。 | 无 |
| `EVEY_TRANSCODING_HW_TONEMAP` | 开启 | `0` 关闭 GPU 上的 HDR 色调映射，改由处理器来做，用于已知有问题的驱动。 | 无 |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | 在 GPU 路线上，每个画质各用一个 ffmpeg，而不是所有画质共用一个进程：这样故障不会互相影响，也避开了会让脆弱的 VA 驱动崩溃的多路输出处理流程。 | 转码 → 高级限制 → **每个码率档一个硬件编码进程**，需重启 |

Intel 的低功耗编码器和 AV1 输出（开关和上限）没有对应的变量：只能在**管理 → 转码**里设置。

## 容量和上限

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0`（CPU 数量） | 同时进行的转码会话总数。如果机器勉强只能用软件转码一路 4K HDR，请设为 `1` 或 `2`。 | 转码 → 高级限制 → **最大并发 HLS 会话数**，需重启 |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | 实时播放之外的转码任务数。 | 转码缓存 → **最大并发**，实时生效 |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | 只编码观众正在看的那个画质。`false` 让所有画质共用一次解码，但也会编码没人看的画质；只适合不能用硬件解码、片源又很重的主机。 | 转码 → 高级限制 → **一次只编码一个码率档**，需重启 |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0`（测量得出） | 强制指定处理器的分辨率上限，不用测量出来的值。 | 转码 → 高级限制 → **软件转码高度上限**，需重启 |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0`（测量得出） | 同上，针对在处理器上做色调映射的 HDR 片源。 | 转码 → 高级限制 → **软件 HDR 高度上限**，需重启 |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | 不限 | 每个软件编码器使用的线程数（1 到 64；其他值都表示不限）。硬件编码器不理会它。 | 无 |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0`（跟随片源） | 转码后视频的码率上限。 | 转码 → **视频码率上限**，实时生效 |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0`（不限） | 同样的上限，针对家庭网络以外的客户端。 | 转码 → **视频码率上限（远程观众）**，实时生效 |

## 缓存

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | 整个转码缓存（片段、重封装的文件、准备好的下载）的大小，最久没用的最先删除。 | 服务器配置 → 服务器设置 → **转码缓存预算**，需重启 |
| `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` | `2048` | 缓存所在磁盘上保留的空闲空间。低于这个值时，先清掉闲置的片段，然后拒绝新的片段，而不是把它们写入磁盘；提取出来的字幕不再保存，并通知管理员。`0` 关闭这个余量。见[转码缓存](../playback/transcoding.md#the-transcoding-cache)。 | 无 |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | 缓存存放的位置。由镜像设置。 | 无 |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | 进度条缩略图存放的位置。由镜像设置：它们重新生成的代价很高，所以放在 `/data` 里。 | 无 |

## 校准片段

测量时会编码一段 4K 片段。按优先顺序：您自己的文件、一段只下载一次并用哈希值核对过的片段，或者一段合成的片段。
真实的片段比合成的测得更准。这些变量在面板里都没有对应的设置。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | 空 | 您自己的片段，HDR 和 SDR 各一段。 | 无 |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | 空 | 要下载一次的片段，通过 `https` 下载。 | 无 |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | 空 | 这次下载的 SHA256；使用 URL 时必须提供。 | 无 |

<!-- sources
cmd/evey/wire.go - EVEY_TRANSCODE_CACHE_MIN_FREE_MB (read at boot, not in config.go) ; internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SingleVariant, SoftwareMaxHeightOverride, SoftwareAV1MaxHeightOverride (panel-only), AV1Enabled, AV1MaxHeight, HWLowPowerEnabled
panel overrides: cmd/evey/main.go - loadOverride (transcoding.*) ; cmd/evey/wire.go (transcoding.hw_mode read directly) ; internal/adapters/driving/http/settings_handler.go
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: frontend/apps/web/src/routes/_auth/admin/transcode-cache.lazy.tsx (transcoding.max_concurrent) ; i18n admin.transcodeCache.maxConcurrent
labels: i18n admin.transcoding.advancedTitle, maxSessions, softwareMaxHeight, softwareHDRMaxHeight, singleVariant, hwPerVariant, hwModeLabel, maxBitrateLabel, maxBitrateRemoteLabel ; admin.serverSettings.transcodeCacheMaxGB
-->
