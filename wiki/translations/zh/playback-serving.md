---
source_sha: fbe5c63d26023c75b078bb43d1151792e32f425b107270e73ddeebd67c742e4a
reviewed: false
---
# 提供视频：HLS 播放路径

这是**读取路径**：播放器请求一个视频时发生的事情，从第一个 `master.m3u8` 请求一直到某个分段的字节。它是 [ARCHITECTURE.md](ARCHITECTURE.md) 中扫描流水线（为读取模型准备数据的写入路径）的对应部分，也是 [CODEBASE_MAP.md](CODEBASE_MAP.md) 中 "Golden path 4" 背后的深入说明。

播放路径是**至关重要的**，分布在 `http` 适配器的大约 8 个文件中。没有一个函数能运行整个流程：本文档就是地图。

## 心智模型：3 个问题、2 个引擎、1 个键

提供一个视频可以归结为回答三个问题，然后封装答案，所有内容都由一个哈希来寻址：

- **问题 1：我能低成本地读取文件吗？** 按**文件扩展名**分派探测：`.mkv/.webm` 和 `.mp4/.mov` 交给 **mkvgo**（只读文件头，不启动进程）；其他所有文件，以及任何快速路径失败的情况，都交给 **ffprobe**（权威来源）。
- **问题 2：客户端能原样播放它吗？** 播放**决定**，*按轨道*做出：视频 `copy` 还是 `transcode`，音频 `copy`、`transcode` 还是 wasm AC-3 路线。视频和音频**独立**决定：常见情况是"直接复制视频，把音频重新编码为 AAC"。
- **问题 3：这台主机能编码多少？** 校准得到的**能力**（来自在第一个空闲时刻进行的测量）加上**准入**（哪个档位放得下、是否有空闲名额）。

两个引擎贯穿其中：

- **mkvgo**：读取并重新封装（重封装 / 按需）而不启动进程。**免费。**
- **ffmpeg / ffprobe**：通过启动进程来解码、重新编码和探测。**昂贵。**

一个键：所有东西（缓存目录、ffmpeg 会话、分段 URL）都由**决定指纹**寻址，即决定的哈希。相同的字节 ⇒ 相同的哈希 ⇒ 一次共享的编码。整个系统的正确性取决于这个哈希在播放器持有的清单和它之后获取的每个分段之间保持**稳定**。

## 阅读顺序：沿着一个请求穿过代码

按顺序打开这些文件，从头到尾追踪一个播放请求：

1. **入口** · `internal/adapters/driving/http/transcode_handler_vod.go`：`serveHTTP` 进行身份验证，然后分派到 master / variant / segment / audio。
2. **计划** · `http/transcode_vod_plan.go` 把请求转换成一个 `PlaybackRequest`；`app/transcoding/playback_plan_resolver.go`：`PlanResolver.Resolve` 返回一个不可变的 `PlaybackPlan`：决定（问题 2）、阶梯类别、调整后的阶梯、硬件路线、封装器和指纹。会话中的每个清单和分段都依据它提供（[ADR-0030](adr/0030-one-plan-one-decision.md)）。`http/transcode_vod_fingerprint.go` 中的 `makeDecision` 和 `buildFingerprint` 是为没有计划的调用点（预热、按需 master）准备的薄封装。
3. **问题 2 · 决定** · `internal/app/transcoding/decision.go`：`MakePlaybackDecisionWithPrefs`（纯函数）：按轨道决定直接复制还是转码的核心。解析器在其上叠加请求时的固定项（`vd=` / `ad=`）、音频重新同步覆盖等。
4. **问题 3 · 能力** · `internal/app/transcoding/calibration_service.go`：`Current()` 返回 `domain/transcoding.Derive(report, policy)`（`derive.go`）：由最近一次校准推导出的主机上限。
5. **阶梯与准入** · 解析器调整阶梯（复杂度类别 → 生产者配置 → 源和能力上限 → CMCD/AV1 上限 → 准入路线和上限），然后在准入固定点上**冻结**它（`admission_shaper.go`，`FreezeLadder`），之后的每次调用都原样读取它：缓存哈希不变量被做成了结构。`video_session_limiter.go` 持有容量名额；满时路线会从硬件溢出到软件。
6. **指纹** · 一个 `DecisionFingerprint`（类型 + `app/transcoding/segment_cache.go` 中的 `Hash()`），由解析器组装。这个哈希就是缓存键。
7. **生成字节** · 直接复制 → **mkvgo** `app/transcoding/ondemand.go`（端口）/ `adapters/driven/mkvgo/ondemand.go`；转码 → **ffmpeg** `adapters/driven/ffmpeg/hls_transcoder.go`（`BuildFFmpegArgs` 是纯函数；分段落在 `/cache/transcode/segments/<hash>/`）。
8. **封装** · `app/transcoding/manifest_generator.go`：`master.m3u8` / `stream_N.m3u8`；`manifest_generator_dash.go`：`manifest.mpd`。

## 两个引擎，按开销划分

**探测分派器**（`adapters/driven/prober/dispatcher.go`）是一个以扩展名为键的责任链：它先尝试 mkvgo，只要有任何疑问就退回 ffprobe（解析错误、panic、无法映射的结果、被丢弃的 MP4 媒体轨道、DoVi 配置 5、无法读取的色彩信息）。两条快速路径始终开启（`wire.go` 中的 nil 闸门）；ffprobe 是安全网，从不被绕过。第二个完全相同的分派器（`keyframe_dispatcher.go`）负责跳转索引。

| 操作 | mkvgo（免费） | ffprobe / ffmpeg（昂贵） |
|---|---|---|
| 元数据探测（编码、尺寸、HDR、码率、时长） | `.mkv/.webm/.mp4/.mov` 只读文件头 | 其他所有文件，或者作为后备 |
| 帧率模式（cfr/vfr） | `AnalyzeFrameRateMode` | -（已迁移） |
| 重封装 MKV→MP4（不重新编码） | `RemuxToMP4` | - |
| 按需 HLS/DASH（直接复制档位） | `OnDemandPackager` | - |
| 视频重新编码 / HDR 色调映射 | - | `hls_transcoder.go` |
| 隔行检测（idet）、跳转落点、chromaprint、DoVi P5 | -（需要解码 / 附加数据） | ffmpeg / ffprobe |

还有三处 ffprobe 调用仍在 mkvgo 已经能提供数据的 MKV/MP4 源上运行（`trickplay.go` 中的时长、`complexity_classifier.go` 中的码率和尺寸、`audio_delay.go` 中的播放音频延迟）：可以迁移的候选项；CPU 收益很小，主要是接线工作。

## 音频与视频分开决定

`makeDecision` 独立地产生 `VideoDecision` 和 `AudioDecision`。"直接复制档位"（`isCopyRequest`）只取决于**视频**；即使是直接复制的视频，通常也会附带一条重新编码的 AAC 音轨。音频决定有三种结果：直接复制、转码为 AAC（ffmpeg，开销小），或者，对于在无法原生解码它的浏览器上的 **AC-3 / E-AC-3**，走 **wasm 路线**：master 被做成只有视频并带有一个 `#EVEY-AUDIO-URI` 标签，AC-3 被**直接复制**（计划能构建时用 mkvgo `od_audio`，否则用 ffmpeg `-c:a copy`），由 `ac3go.wasm` 在浏览器中把它解码为 PCM（`WasmAudioEngine.ts`），并与视频时钟锁定。Safari 是按能力而不是按名称排除的：只有当浏览器报告它*不能*原生播放 AC-3 时，`canWasmDecodeAC3()` 才返回 true，而 Safari 报告它能。

## 封装：CMAF、HLS、DASH

容器始终是 **CMAF**（fMP4）：每个变体一个 `init.mp4` 加上 `.m4s` 分段。（`hls_transcoder.go` 中的 MPEG-TS 分支已经是死代码：`needsFMP4Segments` 总是返回 true。）在*同样的*分段之上：

- **HLS**（`master.m3u8` → `stream_N.m3u8` → `EXT-X-MAP` 初始化段 + `.m4s`）：两个引擎都支持。
- **DASH**（`manifest.mpd`）：在 mkvgo 直接复制路径上，使用封装器自己的清单（`OnDemandPlan.HasDASH`，逐字节相同，不占存储，在 AES-128 下禁用）；在 ffmpeg 路径上，是一个**根据计划声明**的清单，指向 HLS 播放列表所指向的同一批分片（`ManifestGenerator.MPD`），在任何分片存在之前就写好。让它可信的是**分段网格**（`domain/transcoding.SegmentGrid`）：每个转码档位都按相同的整数帧数切分（23.976 fps 时为 72 帧，25 时为 75 帧，29.97 时为 90 帧），所以一个分段在视频轨道的时间刻度上是精确的，并且在整个切换集合中完全相同；直接复制的后备档位会声明它的关键帧计划。帧率未知或可变的源保持旧的 3 秒时钟，保持 HLS，并对 `manifest.mpd` 返回 404。没有客户端被切换到 DASH：HLS 在所有地方仍是默认。对生成的分片运行 mkvgo 的 `DASHFromCMAF` 是一致性的判定依据（`internal/pkg/cmafparity`，`TestDASHGridIntegration` 在 23.976/25/29.97 fps 下测试：冷启动、跳转、两个档位的切换集合、在一个 AAC 帧内对齐的单独音频）。

每个会话的 URL 会被重新烘焙（`transcode_vod_manifest_rebake.go`）以携带流令牌，HLS 和 DASH 都是如此，包括不使用 Cookie 的 Chromecast。

## 用到的设计模式

- **端口与适配器（六边形）**：应用层与接口（`StreamProber`、`BenchmarkRunner`、`OnDemandPackager`、`StreamProducer`）对话；ffmpeg 和 mkvgo 实现它们。新的引擎就是新的适配器。
- **策略**：`StreamProducer`（软件 / 硬件 / 直接复制），由 `ProducerSelector` 选择。
- **带后备的责任链**：探测和关键帧分派器（mkvgo → ffprobe 权威）。
- **内容寻址缓存**：`DecisionFingerprint` 哈希作为缓存和 ffmpeg 会话的键。
- **纯核心 / 命令式外壳**：`Derive`、`MakePlaybackDecision` 和开销模型都是纯函数，无需 ffmpeg 或数据库即可进行单元测试。
- **不可变值对象**：`StreamPlan`，即准入决定，只发出一次，从不修改，所以 URL↔字节的不变量在构造上就成立。
- **优雅降级**：每一步都有安全的后备：720p 默认能力、硬件→软件溢出、直接复制→转码、mkvgo→ffprobe。

## 决定的核心：应用层的 PlanResolver

处理请求的核心（决定固定项、阶梯调整、硬件路由、指纹组装、直接复制档位路由、生产者选择）位于应用层：`PlanResolver`（`app/transcoding/playback_plan_resolver.go`），即具体的 `PlaybackPlanResolver`（[`playback_plan.go`](../internal/app/transcoding/playback_plan.go)）。每个请求只解析**一次**：http 适配器把 `*http.Request` 转换成一个与传输无关的 `PlaybackRequest`（`http/transcode_vod_plan.go`），并依据返回的不可变 `PlaybackPlan` 提供服务。每个使用方（清单输入、负载削减探测、过期 URL 封印、生产者选择、分段键）都读取这同一次解析结果，所以*在一个请求之内*缓存哈希不变量在构造上就成立（分段热路径也从约 5 次读取个人资料偏好减少到 1 次）。源信息辅助函数（编码、HDR、SAR、10 位……）就放在旁边的 `app/transcoding/source_facts.go` 中；适配器保留了薄的别名。在请求之间，这个不变量仍然依赖于 URL 烘焙：**决定的每一项输入都必须烘焙进子 URL**，否则播放中途的一次轮询可能会在播放器之下改变计划（`manifest_generator.go` 中的 `querySuffix`）。
