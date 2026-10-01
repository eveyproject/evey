---
source_sha: 943149aa81fcdc21ea4425ec9e43d282740de164c026d29cc366577b71c0fd72
reviewed: false
---
# 转码与硬件

容量、显卡、画质限制和转码缓存。大多数安装都不设置其中任何一项：服务器会测量机器并自己选择硬件（见[转码与服务器容量](../playback/transcoding.md)）。当某项设置在 **Admin → 转码** 中存在时，以那里保存的值为准；表格中会注明。

## 硬件加速

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | 空 | `auto`、`off`、`nvenc`、`vaapi`、`qsv` 或 `rkmpp`。为空：由下面两个变量推导。被面板中的 **硬件加速** 覆盖，后者会实时重新测量。 |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | 旧的开/关开关。被面板覆盖。 |
| `EVEY_TRANSCODING_HW_BACKEND` | 空（自动检测） | 固定一个后端（`nvenc`、`vaapi`、`qsv`）：只测量这一个。可以让同一主机上的多个 Evey 容器各自使用不同的 GPU 路线。 |
| `EVEY_TRANSCODING_HW_DEVICE` | 空（全部） | 固定一个设备：一个渲染节点（`/dev/dri/renderD129`）或一个 CUDA 索引（`1`）。与 `EVEY_TRANSCODING_HW_BACKEND` 一起使用，可在多 GPU 主机上把一个容器绑定到一块 GPU。 |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | 在这个时间内没有生成第一个分段的硬件会话会被视为 GPU 卡住，并转到处理器上。 |
| `EVEY_TRANSCODING_HW_TONEMAP` | 开启 | `0` 会关闭 GPU 上的 HDR 色调映射，改用处理器完成，适用于已知有问题的驱动。 |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | 在 GPU 路线上每个画质一个 ffmpeg，而不是所有画质一个进程：隔离故障，并避免会让脆弱的 VA 驱动崩溃的多输出图。被 **高级限制** 覆盖；启动时读取。 |

把 GPU 暴露给容器：[硬件加速](../administration/hardware-acceleration.md)。

## 容量与限制

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0`（CPU 数量） | 并发转码会话的总数。在一台勉强能维持一路 4K HDR 软件转码的机器上，设置为 `1` 或 `2`。被 **高级限制** 覆盖；启动时读取。 |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | 实时播放之外的转码任务。被 **Admin → 转码缓存** 中的 **最大并发** 覆盖。 |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | 只编码观众正在观看的画质。`false` 让所有画质共享一次解码，但会编码没有人观看的画质；只适用于没有硬件解码且源文件很重的主机。被 **高级限制** 覆盖；启动时读取。 |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0`（测量值） | 强制设定处理器的分辨率上限，而不是使用测量值。被 **高级限制** 覆盖；启动时读取。 |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0`（测量值） | 同上，针对在处理器上进行色调映射的 HDR 源。 |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | 不限 | 每个软件编码器的线程数（1 到 64）。硬件编码器会忽略它。 |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0`（跟随源） | 转码视频码率的上限。被 **视频码率上限** 覆盖。 |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0`（无） | 针对家庭网络之外客户端的同类上限。被 **视频码率上限（远程观众）** 覆盖。 |

## 缓存

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | 整个转码缓存（分段、重封装、预先准备的下载）的大小，最久未使用的先被删除。卷上始终保留 2 GiB 可用空间。被 **服务器设置** 中的 **转码缓存预算** 覆盖；启动时读取。 |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | 它所在的位置。由镜像设置。 |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | 进度条缩略图所在的位置。由镜像设置：它们重新生成的代价很高，所以保存在 `/data` 中。 |

## 校准片段

测量会编码一段 4K 片段。按优先顺序：你的文件、一次性下载并通过哈希校验的片段，或者合成片段。真实片段的测量比合成片段更准确。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`、`EVEY_BENCH_SDR_SAMPLE` | 空 | 你自己的 HDR 和 SDR 片段。 |
| `EVEY_BENCH_HDR_SAMPLE_URL`、`EVEY_BENCH_SDR_SAMPLE_URL` | 空 | 通过 `https` 一次性下载的片段。 |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`、`EVEY_BENCH_SDR_SAMPLE_SHA256` | 空 | 该下载的 SHA256；使用 URL 时必须提供。 |
