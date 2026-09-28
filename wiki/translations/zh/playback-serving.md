---
source_sha: 2ff1222cb6483b259b30b0a40ec51e6e4e8d1e7a7f4a5877a9761a0186adff2d
---

# 送出一段视频 - HLS 播放路径

这里讲的是**读取路径**：播放器要一段视频时都发生了什么，从第一次请求 `master.m3u8`
一直到分片的字节。它与 [ARCHITECTURE.md](ARCHITECTURE.md) 里的扫描流水线（那条为读模型
备好数据的写入路径）互为对照，也是 [CODEBASE_MAP.md](CODEBASE_MAP.md) 中“黄金路径 4”
背后的详解。

播放路径**至关重要**，而且散落在 `http` 适配器的大约 8 个文件里。
没有哪一个函数跑完整条流程 - 本文就是那张地图。

## 心智模型：3 个问题、2 台引擎、1 把钥匙

送出一段视频，归根结底就是回答三个问题，再把答案打包，而这一切都由同一个哈希来寻址：

- **问题 1 - 能不能廉价地读到这个文件？** 探测的分发按**文件扩展名**决定：
  `.mkv/.webm` 和 `.mp4/.mov` 交给 **mkvgo**（只读文件头，不起进程）；
  其余一切，以及快速路径的任何一次失手，都交给 **ffprobe**（最终权威）。
- **问题 2 - 客户端能不能原样播放？** 这是播放**决策**，*逐轨*作出：视频是 `copy`
  还是 `transcode`，音频是 `copy`、`transcode` 还是走 wasm 的 AC-3 路径。
  视频和音频**各自独立**决定 - 最常见的情形是“视频直接拷贝，音频重新编码成 AAC”。
- **问题 3 - 这台主机能编码多少？** 由启动时的基准测试校准出的**能力**，
  再加上**准入**（哪一档阶梯放得下，还有没有空位）。

贯穿始终的是两台引擎：

- **mkvgo** - 读取与重新打包（重新封装 / 按需打包），全程不起进程。**免费。**
- **ffmpeg / ffprobe** - 解码、重新编码、探测，都要起一个进程。**昂贵。**

一把钥匙：一切（缓存目录、ffmpeg 会话、分片 URL）都由**决策指纹**寻址 - 那是决策本身的哈希。
同样的字节 ⇒ 同样的哈希 ⇒ 共用一次编码。整套系统的正确性，都押在这个哈希在
播放器手里那份清单和它之后取的每一个分片之间**保持稳定**上。

## 阅读顺序 - 跟着一次请求穿过代码

依次打开下面这些文件，就能从头到尾追完一次播放请求：

1. **入口** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` 先认证，再分发到 master / variant / segment / audio。
2. **问题 2 · 决策** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs`（纯函数）：逐轨判断拷贝还是转码的大脑。
   外面包着 `http/transcode_vod_fingerprint.go` 里的 `makeDecision`，
   由它叠加请求级的固定项（`vd=` / `ad=`）、音频重新对齐的覆盖值等等。
3. **问题 3 · 能力** · `internal/app/transcoding/calibration_service.go` -
   `Current()` 返回 `domain/transcoding.Derive(report, policy)`：从上一次基准测试
   推导出的本机上限。
4. **阶梯成形** · `http/transcode_vod_profiles.go` - `profilesForDecision`：
   复杂度分级 → 生产者档位 → 源文件与能力的上限 → CMCD/av1 的上限 →
   准入的路由和天花板，然后把**成形后的阶梯冻结**在准入固定项上
   （之后每次调用都原样读取 - 把缓存哈希的不变量做成了结构上的保证）。
5. **准入** · `internal/app/transcoding/admission_shaper.go`
   （不可变的 `StreamPlan`）加上 `video_session_limiter.go`：决定路由
   （含硬件→软件的溢出）和容量名额。
6. **指纹** · `http/transcode_vod_fingerprint.go`（`buildFingerprint`）构造出一个
   `DecisionFingerprint`（类型和 `Hash()` 在 `app/transcoding/segment_cache.go`）。
   这个哈希就是缓存键。
7. **产出字节** · 拷贝走 **mkvgo** `app/transcoding/ondemand.go`（端口）/
   `adapters/driven/mkvgo/ondemand.go`；转码走 **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go`（`BuildFFmpegArgs` 是纯函数；
   分片落在 `/cache/transcode/segments/<hash>/`）。
8. **打包** · `app/transcoding/manifest_generator.go` - 产出 `master.m3u8` /
   `stream_N.m3u8`（走 mkvgo 时还有 `manifest.mpd`）。

## 两台引擎，按开销划分

**探测分发器**（`adapters/driven/prober/dispatcher.go`）是一条以扩展名为键的责任链：
先试 mkvgo，一有疑问就回落到 ffprobe（解析出错、panic、结果无法映射、MP4 里丢了一条
媒体轨、Dolby Vision profile 5、色彩信息读不出来）。两条快速路径始终开启
（`wire.go` 里的开关是 nil）；ffprobe 是安全网，永远不会被绕过。
另有一条一模一样的分发器（`keyframe_dispatcher.go`）负责跳转索引。

| 操作 | mkvgo（免费） | ffprobe / ffmpeg（昂贵） |
|---|---|---|
| 元数据探测（编码格式、分辨率、HDR、码率、时长） | `.mkv/.webm/.mp4/.mov`，只读文件头 | 其余一切，或作为回落 |
| 帧率模式（cfr/vfr） | `AnalyzeFrameRateMode` | - （已迁移） |
| MKV→MP4 重新封装（不重新编码） | `RemuxToMP4` | - |
| 按需 HLS/DASH（拷贝档） | `OnDemandPackager` | - |
| 视频重新编码 / HDR 色调映射 | - | `hls_transcoder.go` |
| 隔行检测（idet）、跳转落点、chromaprint、Dolby Vision P5 | - （需要解码或附带数据） | ffmpeg / ffprobe |

在 MKV/MP4 源上，仍有三处会 fork 出 ffprobe，而它们要的数据 mkvgo 其实已经给出了
（`trickplay.go` 里的时长、`complexity_classifier.go` 里的码率和分辨率、
`audio_delay.go` 里播放用的音频延迟）- 这几处适合迁移；能省下的 CPU 不多，主要是接线活。

## 音频与视频分开决策

`makeDecision` 各自独立地产出 `VideoDecision` 和 `AudioDecision`。“拷贝档”
（`isCopyRequest`）只看**视频**这一侧；哪怕视频是直接拷贝的，旁边通常也挂着一条
重新编码出来的 AAC 轨。音频决策有三种结果：拷贝、转成 AAC（走 ffmpeg，开销很轻），
或者 - 当浏览器无法原生解码 **AC-3 / E-AC-3** 时 - 走 **wasm 路径**：母版播放列表做成只有视频，
带一个 `#EVEY-AUDIO-URI` 标签，AC-3 则被**拷贝**出来（计划能建起来就用 mkvgo 的
`od_audio`，否则用 ffmpeg `-c:a copy`），再由 `ac3go.wasm` 在浏览器里解码成 PCM
（`WasmAudioEngine.ts`），并与视频锁定时钟。Safari 被排除在外靠的是能力而不是名字：
只有当浏览器报告自己*不能*原生播放 AC-3 时，`canWasmDecodeAC3()` 才返回 true -
而 Safari 报告的是能。

## 打包：CMAF、HLS、DASH

容器格式永远是 **CMAF**（fMP4）：每一档一个 `init.mp4`，外加若干 `.m4s` 分片。
（`hls_transcoder.go` 里那条 MPEG-TS 分支是死代码 - `needsFMP4Segments` 恒返回 true。）
在*同一批*分片之上：

- **HLS**（`master.m3u8` → `stream_N.m3u8` → `EXT-X-MAP` 的 init 加 `.m4s`）- 两台引擎都支持。
- **DASH**（`manifest.mpd`）- 在 mkvgo 拷贝路径上，是打包器自己的清单（`OnDemandPlan.HasDASH`，
  字节完全一致，不占存储，AES-128 下关闭）；在 ffmpeg 路径上，是一份**依据计划声明**的清单，
  覆盖 HLS 播放列表所指向的同一批分片（`ManifestGenerator.MPD`），在任何分片生成之前就已写好。
  让它名副其实的是**分段网格**（`domain/transcoding.SegmentGrid`）：每一档转码都按同样的整数帧数切分
  （23.976 fps 时 72 帧，25 时 75，29.97 时 90），因此一个分段在视频轨的时基上是精确的，
  在整组切换档位中完全一致；拷贝回退路径则声明自己的关键帧计划。帧率未知或可变的源保留旧的 3 秒时钟，
  继续使用 HLS，并对 `manifest.mpd` 返回 404。没有任何客户端被切换到 DASH：HLS 在所有地方仍是默认。
  对产出分片运行 mkvgo 的 `DASHFromCMAF` 作为一致性基准（`internal/pkg/cmafparity`，
  `TestDASHGridIntegration`，覆盖 23.976/25/29.97 fps：冷启动、跳转、两档切换组、
  音频分离且误差在一个 AAC 帧之内）。

按会话生成的 URL 会被重新烘焙一遍（`transcode_vod_manifest_rebake.go`）以带上流令牌，
HLS 和 DASH 一视同仁，连没有 Cookie 的 Chromecast 也包括在内。

## 用到的设计模式

- **端口与适配器（六边形架构）** - 应用层只面向接口说话（`StreamProber`、
  `BenchmarkRunner`、`OnDemandPackager`、`StreamProducer`），由 ffmpeg 和 mkvgo 去实现。
  加一台新引擎，就是加一个新适配器。
- **策略模式** - `StreamProducer`（软件 / 硬件 / 拷贝），由 `ProducerSelector` 挑选。
- **带回落的责任链** - 探测和关键帧两条分发链（mkvgo → 权威的 ffprobe）。
- **内容寻址的缓存** - `DecisionFingerprint` 的哈希既是缓存键，也是 ffmpeg 会话的键。
- **纯函数内核 / 命令式外壳** - `Derive`、`MakePlaybackDecision` 和成本模型都是纯函数，
  不需要 ffmpeg 也不需要数据库就能做单元测试。
- **不可变值对象** - `StreamPlan`：准入决策一次产出，绝不修改，
  于是“URL 与字节一一对应”这条不变量在构造上就成立。
- **优雅降级** - 每一步都有安全的回落：默认 720p 的能力、硬件→软件的溢出、
  拷贝→转码、mkvgo→ffprobe。

## 决策大脑：应用层的 PlanResolver

处理请求的那个大脑 - 决策固定项、阶梯成形、硬件路由、指纹拼装、拷贝档的路由、
生产者的挑选 - 都住在应用层：`PlanResolver`（`app/transcoding/playback_plan_resolver.go`），
具体实现是 `PlaybackPlanResolver`
（[`playback_plan.go`](../internal/app/transcoding/playback_plan.go)）。
每次请求只解析**一次**：http 适配器把 `*http.Request` 整理成一个与传输层无关的
`PlaybackRequest`（`http/transcode_vod_plan.go`），随后就照着返回的那份不可变
`PlaybackPlan` 提供服务。所有使用方 - 清单的输入、卸载判定、过期 URL 的封印、
生产者的挑选、分片的键 - 读的都是这同一次解析结果，所以*在一次请求之内*，
缓存哈希的不变量在构造上就成立（分片的热路径也从大约 5 次配置文件偏好查询降到了 1 次）。
读取源文件事实的那些辅助函数（编码格式、HDR、SAR、10 bit 等）就放在旁边的
`app/transcoding/source_facts.go` 里，适配器只保留一层薄薄的别名。
跨请求时，这条不变量依旧靠 URL 烘焙来维持：**决策的每一项输入都必须烘进子 URL**，
否则播放途中的一次轮询就可能在播放器脚下把计划换掉（见 `manifest_generator.go`
里的 `querySuffix`）。
