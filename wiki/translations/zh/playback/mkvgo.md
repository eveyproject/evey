---
source_sha: 9be0f9fe12d2d5606cefd03463fe0dd43f1000f645afe396700673ac2c53dcea
reviewed: true
---
# Evey 中的 MKVGo { #mkvgo-in-evey }

**简单来说：**

- MKVGo 是 Evey 的一部分，它读取视频文件（MKV、WebM、MP4、MOV）并重新打包，但不转换画面和声音。
- 在可行的情况下，**重封装**播放由它来提供，字幕由它来提取，扫描时也由它来读取您的文件。凡是需要真正转换的工作，都交给 FFmpeg，一个独立的视频工具。
- 您不用设置任何东西。**管理 → 正在播放**会逐条轨道显示，一次播放是由 MKVGo 还是 FFmpeg 提供的。

本页其余部分写给想知道这些事的读者：服务器不启动 FFmpeg 能做什么，以及哪些地方仍然由 FFmpeg 接手。MKVGo 运行在 Evey 的服务器进程内部。

MKVGo 处理的是文件的结构：文件头、索引、轨道，以及轨道里的数据包。它从不解码或编码画面和声音；凡是需要在服务器上解码的工作，都交给 FFmpeg。它作为一个独立的 Go 库开发：[github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo)。

## 三个层次的工作 { #three-levels-of-work }

| 层次 | 做了什么 | 服务器要付出什么 |
| --- | --- | --- |
| **直接播放** | 文件原样发送。 | 读取文件。 |
| **MKVGo** | 结构上的工作：读取文件、把轨道重新打包成适合流式传输的形式、重建索引、在时间轴上移动一条轨道。 | 服务器进程内部的代码：不会为每个视频流启动 FFmpeg 或 ffprobe，按需打包时也不往磁盘写片段。 |
| **FFmpeg** | 真正的转换：重新编码、色调映射、解码画面。MKVGo 无法处理某个文件时，也由它作为后备。 | 每项工作一个进程。 |

FFmpeg 的复制（`-c copy`）同样不重新编码任何东西，开销也很低；MKVGo 额外省下的，是每次会话要启动的进程，以及写进转码缓存的片段。重新编码才是开销大的情况，而重新编码视频是唯一会占用转码名额的工作：见[转码与服务器容量](transcoding.md)。

开始播放和跳转时，不用等待进程启动，也不用等第一个片段写到磁盘上：片段是在请求时根据文件的索引即时生成的。一个文件会不会使用这种方式，取决于播放器能解码什么：见 [Evey 如何选择播放方式](how-playback-is-decided.md)。

## MKVGo 做什么 { #what-mkvgo-does }

| 工作 | 在哪里体现 | MKVGo 无法处理时 |
| --- | --- | --- |
| **读取文件** | 扫描媒体库时，从文件开头读取编码、分辨率、HDR、码率和时长。 | 由 ffprobe 读取。只要有任何疑问，也由它接手：解析出错，或者在支持 HDR 的编码上，MKVGo 无法读取色彩信息。 |
| **关键帧索引** | 播放可以从哪些位置开始，从文件的索引中读取；有了它，重封装和视频复制才能在关键帧处切分片段。 | ffprobe。 |
| **不重新编码的流式传输** | 重封装：分段 MP4（CMAF）格式的 HLS 片段，根据为每个文件保存的计划，在请求时生成。片段在发送时直接从源文件写到连接上：不会先在内存里组装，也不会写进转码缓存。同样的片段还会生成一份 DASH 清单；应用使用的是 HLS。 | 由 FFmpeg 复制轨道，仍然不重新编码，生成的片段写进缓存。 |
| **给浏览器用的 MP4** | 浏览器无法打开的 Matroska 文件，会被重写一次，变成可以快速开始播放的 MP4，并保存在缓存里。 | - |
| **字幕** | MKV 里内嵌的字幕轨会在请求时提取：文字字幕轨转成 WebVTT，图形字幕轨（PGS）转成图片。如果文件自己的索引（Cues）完整覆盖了某条轨道，服务器只读取索引指向的数据块，而不是整个文件，并把这份数据块索引保存在它的数据目录（`subtitle-index/`）下，供下次请求使用。仍然需要完整读取的轨道，会回复“正在准备”并附上重试间隔，播放器会反复询问，直到准备好为止。 | 由 ffmpeg 从头到尾读取文件，提取这条轨道。 |
| **预览图** | 通过索引找到离请求位置最近的关键帧，FFmpeg 只解码这一帧。当这个关键帧与请求位置相差大约两秒以内时使用。 | FFmpeg 跳转并一直解码到准确的位置。 |
| **剧集串联** | 服务器可以把几集打包成一个连续的视频流（在视频流地址后加 `?gapless=1`）。目前还没有应用提供这个开关。 | 每一集单独播放。 |
| **媒体健康** | 诊断（跳转索引、音频起点、按块存储的轨道）、重建的索引、跳过损坏片段后的重新同步，以及按偏移量移动一条音轨。见[媒体健康](../runbooks/media-health.md)。 | 按块存储的轨道由 FFmpeg 重新封装。 |

## 仍然由 FFmpeg 做的事 { #what-stays-with-ffmpeg }

- 所有重新编码，包括 HDR 色调映射。
- 解码画面：进度条上的缩略图、预览图的那一帧。
- 需要解码后数据的分析：隔行检测、用于片头检测的音频指纹。
- MKV/WebM 和 MP4/MOV 以外的容器，例如 AVI 或 MPEG-TS。

## 负载、被替换的文件和更新 { #load-replaced-files-and-updates }

**一次读多少。** 生成一个片段要读取源文件的一个窗口，视频和音频一起读：1080p 文件是 6 到 10 MiB，UHD 重封装大约 15 MiB。服务器同时读取的窗口数和它的处理器数一样多；内存小的机器上会更少：所有正在读的窗口加起来，必须容纳在服务器为自身设定的内存的四分之一以内（容器限制的一半，或者主机内存的一半）。超出的请求要等空位，所以占用的内存不会随观众人数增长；在小主机上，如果很多人同时开始看，他们的第一批片段会一个接一个地到达，而不是让服务器内存耗尽。

**播放过程中文件被替换。** 每个片段的地址都写明了它是按哪个文件规划的。当磁盘上的文件被替换时（换了更好的版本，或者修复过），这时请求的片段会根据新文件重新生成一次；旧文件播放列表里的地址会回复 `410 Gone`，而不是把两个文件混在一起；重新打开该影片，播放的就是新文件。

**更新之后。** 片段地址里带有 MKVGo 的版本号，所以浏览器或代理缓存永远不会提供旧版本生成的片段；这条路径的片段也不会保存在磁盘上，不需要清理。为浏览器制作的 MP4 副本以其格式的版本命名：格式变化时，每个副本会在下次被请求时重新生成。

**监控它。** 在 `GET /metrics` 上（见[监控与日志](../operations/monitoring.md#prometheus-metrics)），`evey_mkvgo_walks_in_flight` 是此刻正在读取的窗口数，`evey_mkvgo_plan_stats` 按 `stat` 汇总打包器的各项计数。如果 `walks_in_flight` 一直停在上限，同时 `stat="slot_waits"` 不断上升，说明观众在排队等处理器；`stream_fallbacks` 统计的是无法以流式方式发送、只能先在内存中组装的片段数。

## 查看是哪个引擎提供的视频流 { #seeing-which-engine-served-a-stream }

**管理 → 仪表盘**和**管理 → 正在播放**会标明每条轨道用的引擎：`mkvgo` 或 `ffmpeg`。一次重封装可能两个都有，例如视频由 MKVGo 打包，音频由 FFmpeg 复制。

mkvgo 以 MIT 许可证发布。

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
streamed segments: internal/app/transcoding/ondemand.go - OnDemandResource (Bytes nil, WriteRange) ; internal/adapters/driving/http/transcode_ondemand_stream.go - servePlanResourceStreamed
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driving/http/subtitle_handler.go - extractSubtitleVTT ; subtitle_pgs.go ; subtitle_index_cache.go - fromCues, resolveAndStore, newSubtitleIndexCache ; subtitle_preparing.go (202 + Retry-After) ; subtitle_prewarm.go - ensureIndex
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
reads at once: internal/adapters/driven/mkvgo/budget.go - BudgetFor (one walk per CPU, walks within memLimit/4 at 15 MiB each) ; cmd/evey/wire_infra.go (applied after the memory limit) ; cmd/evey/memlimit.go - applyMemoryLimit (half the cgroup or host budget)
replaced file: internal/adapters/driving/http/transcode_ondemand.go - planSourceParam, servePlanSourceGone (410), replay once on a rebuilt plan ; internal/adapters/driving/http/transcode_vod_manifest_rebake.go - stampPlanSourceURIs
after an update: packager version in segment URLs (?v=) ; internal/adapters/driving/http/remux_handler.go - remuxCacheFormat, remuxCacheName
metrics: internal/pkg/metrics/metrics.go - MkvgoWalksInFlight, MkvgoPlanStats ; cmd/evey/wire.go - startMkvgoMetrics
plain opening: rules below ; i18n admin.sessions.track.engine
-->
