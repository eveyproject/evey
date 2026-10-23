---
source_sha: e039606b33ab2207d3774f9bacb71ee3d18f0d12f52d9dc02bec1e9817e1c91e
reviewed: true
---
# 转码与服务器容量 { #transcoding-and-server-capacity }

**简单来说：**

- **转码**就是把视频转换成设备能播放的形式。这是服务器唯一的高负载工作：原样播放文件（直接播放）或者重新打包（重封装）几乎没有开销。见 [Evey 如何选择播放方式](how-playback-is-decided.md)。
- 服务器能同时服务多少人，不用您来设置。它会在第一次空闲时自行测量，只需几分钟，而且从不接受超出能力的播放。
- 能力不够时，新的播放会得到较低的画质，或者从忙碌的显卡换到处理器上。如果两边都没有空位，播放器会显示**服务器正忙于转码其他流**。
- 您可能想做的事：给服务器配一块显卡（[硬件加速](../administration/hardware-acceleration.md)），并在存放缓存的磁盘上留出空闲空间（[转码缓存](#the-transcoding-cache)）。

本页其余部分写给想了解细节的管理员：服务器如何测量自己能编码什么，如何分配这份能力，以及满载时观众会看到什么。

## 服务器自行测量 { #the-server-measures-itself }

容量不是一个要您估算的设置，而是由服务器测量出来的：

- **什么时候。** 不是在启动时。服务器启动时先沿用以前知道的结果（第一次启动时，用一个保守的 720p 软件默认值），然后在第一次空闲时进行测量，也就是没有人在观看、处理器大部分时间空闲的时候。测量需要几分钟。如果测量时机器有负载，结果会被标记为临时的，等机器空闲时再测一次。
- **测什么。** 用一段 4K 片段做真实的编码，SDR 和 HDR 各一次，在处理器和服务器检测到的每个加速器上都做，方式和真实播放一样。结果是每个加速器和处理器能实时持续处理的最高分辨率，以及每种视频流的开销。
- **再测一次。** 硬件变化之后会重新测量；在**管理 → 转码 → 编码校准**里点击**重新校准**也会重新测量。有人正在观看时，这个按钮会拒绝执行；只有自动的重新测量才会等到服务器空闲。

测量进行期间，需要转码的播放会显示**服务器正在自我校准**，并被要求稍后再试。直接播放和重封装不受影响。

如果处理器连一种分辨率都无法持续处理，服务器仍然提供 720p（SDR）和 480p（HDR），同时能提供多少路视频流，由视频会话限制器的预算决定。

测量按这个顺序选用片段：您提供的片段；一段只下载一次、并用 SHA256 校验的参考片段；或者当场生成的合成片段。真实片段测得更准确。合成片段会高估开销，可能把一台机器的上限定得比它实际能力低：

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## 分配这份能力 { #sharing-the-capacity }

**实时播放容量**（**管理 → 转码**）是可以同时编码的视频流数量。它由测量结果得出，硬件变化时会自动调整。**限制并发视频流数量**可以把它调低，但不能调高。

- 只重新编码音频的转码（视频直接复制）不占名额。
- 观众切换画质时，仍然只占一个名额。
- 只编码观众实际在看的那个画质，而不是全部各档画质。

每个视频流都交给已经证明能服务它、而且开销最低的处理路径：一个按这个流的需要测量过的加速器（例如 HDR 色调映射，或者输出 HEVC），或者处理器。处理器和加速器是两个独立的资源池。在显卡上处理的视频流，它的音频和打包仍然要用一点处理器。

空位不够时，服务器按这个顺序应对：

1. **降低画质。** 如果一个新视频流的最高画质无法容纳，就以能容纳的最高画质接纳它，这个上限在整个播放过程中保持不变。
2. **改用处理器。** 加速器已满时，如果处理器还有空位，新的视频流就改到处理器上。
3. **拒绝。** 两边都没有空位时，播放器显示**服务器正忙于转码其他流**，并且可以重试。

如果显卡在某部影片上编码失败，这部影片会有一段时间改用处理器，对所有观众都是如此。只有当一次与媒体无关的测试编码在这块显卡上也失败时，显卡本身才会被停用，之后还会再次尝试：见[硬件加速](../administration/hardware-acceleration.md)。

有人观看时，服务器自己的后台任务（缩略图、片头检测、媒体分析）会暂停，以便播放优先使用机器资源：见[后台任务与服务器负载](../operations/background-work.md)。

如果尽管如此，某一类片源仍然总是让观众等待，服务器会在一段时间内降低它的最高画质，之后再自动取消这个限制。见[监控与日志](../operations/monitoring.md#how-playback-actually-went)。

## 编码格式 { #what-is-encoded }

| | 默认 |
| --- | --- |
| 视频编码 | H.264。当某条经过实测的路径证明可行且播放器能解码时，输出 HEVC（HDR 直通路径，或者个人资料的**首选视频编码**要求 HEVC）；如果开启了 AV1（**AV1 输出**，仅限硬件）并且播放器能解码，就用 AV1。 |
| 分辨率 | 和片源一样，除非有限制：播放器的最大值、码率限制，或者服务器能实时编码的上限。 |
| 码率 | 跟随片源，并按分辨率换算。**视频码率上限**限制其最大值；对本地网络之外的观众，**视频码率上限（远程观众）**会把上限定得更低。 |
| 音频 | AAC。见[音频](audio.md)。 |
| 分发方式 | HLS，使用分段 MP4 片段。 |

<figure class="evey-shot" markdown="span">
  ![管理 → 转码中的两个视频码率上限](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption><strong>管理 → 转码</strong>里的两个上限：一个针对所有观众，一个针对本地网络之外的观众（界面为英文）。</figcaption>
</figure>

## 转码缓存 { #the-transcoding-cache }

编码好的片段保存在磁盘上的 `evey-cache` 卷里。当另一位观众，或者同一位观众之后，以相同的处理方式播放同一部影片时，就会重复使用这些片段。**管理 → 转码缓存**显示缓存的大小，也可以清空它；**转码缓存预算**（**管理 → 转码**）为它设定上限。**清除全部**还会删除从您的文件中提取出来的字幕，它们保存在 `./evey-data/subtitles` 里：有需要时会重新提取。

**磁盘快满时。** 服务器会在存放缓存的磁盘上保留一段空闲空间作为余量：默认 2 GiB，用 `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` 设置（`0` 表示不保留余量）。空闲空间低于这个余量时，先删除没有人在看的、最旧的片段。如果这样腾出的空间还不够，对于还不存在的片段，服务器会拒绝请求，并告诉播放器多久之后可以再次请求，而不是启动一个会因空间不足而失败的编码器；已经在磁盘上的片段照常提供。保存在 `./evey-data` 里的提取字幕也遵守同样的余量：低于余量时，新提取的字幕不再保存，每次请求都会重新提取，管理员会收到**字幕缓存已满**（每小时最多一次，也会以 `transcoding.derived_cache_full` 发送给 webhook）。

!!! tip
    如果某次更新改变了视频的编码方式，请在更新后清空缓存，这样就没有人会收到旧版本生成的片段。有人正在观看时清空缓存，会让他的视频流重新缓冲一次，然后继续播放。

## 查看实际情况 { #checking-what-happens }

- **管理 → 正在播放**：每个视频流、逐条轨道的处理方式，以及正在为它编码的后端。
- **管理 → 转码 → 编码校准**：测得的上限，以及正在使用的加速器。
- **管理 → 流媒体质量**：真实观众的播放情况如何。
- **负载测试**（**管理 → 转码**）：模拟观众，测量服务器能承受多少。有人在观看时，它会拒绝运行。

<!-- sources
disk floor: cmd/evey/wire.go (EVEY_TRANSCODE_CACHE_MIN_FREE_MB, DefaultMinFreeBytes, SetWriteFloor, SetSubtitleCacheWriteFloor, OnSubtitleCacheFloorHit) ; internal/app/transcoding/segment_cache_admit.go - AdmitWrite, tryOnDemandEvict, ErrCacheFull ; internal/adapters/driving/http/transcode_vod_admission.go (503 + Retry-After) ; internal/domain/transcoding/events.go - DerivedCacheFull ; i18n notifications.cache_full
clear all empties extracted subtitles: internal/app/transcoding/service.go - WithDerivedCachePurge ; internal/adapters/driving/http/subtitle_cache.go - SubtitleCachePurger (/data/subtitles)
calibration: internal/app/transcoding/calibration_service.go - EnsureCalibrated, Recalibrate, idleProbes, idleCPUPercent ; internal/domain/transcoding/capability.go - SafeFallback ; internal/adapters/driven/ffmpeg/benchmark.go - swBenchLadderSpecs (floors 720/480), sample order (path > url+sha256 > synthetic) ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md ; i18n player.calibrating, admin.calibration.contendedLine, transcodesActiveToast
capacity: i18n admin.transcoding.livePlaybackTitle, maxVideoSessionsDesc, videoSessionsAutoAdjusts ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper, StreamPlan ; internal/app/transcoding/video_session_limiter.go - VideoSessionLimiter (PoolCPU, PoolGPU) ; internal/app/transcoding/resolved_route.go - SpillPoolFull, SpillTitleBlocked ; internal/app/transcoding/hw_route_blocklist.go ; CalibrationService.DemoteCapability
busy: i18n errors max_concurrent_reached
output codec: internal/app/transcoding/resolver_route.go ; internal/app/transcoding/decision.go ; internal/app/transcoding/testdata/route_golden.txt ; i18n admin.calibration.av1Label, av1Hint, maxBitrateLabel, maxBitrateRemoteLabel
single variant: internal/config/config.go (EVEY_TRANSCODING_SINGLE_VARIANT, default on)
bench samples: internal/config/config.go (EVEY_BENCH_{SDR,HDR}_SAMPLE{,_URL,_SHA256})
cache budget, clear while live: i18n admin.serverSettings.transcodeCacheMaxGB, admin.transcodeCache.clearConfirmLiveSessions ; automatic limits: admin.qoe.overrides.*
load test: i18n admin.loadtest.idleWarning, errorActiveStreams
plain opening, busy message: i18n errors max_concurrent_reached ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper.GrantRoute
-->
