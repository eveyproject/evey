---
source_sha: b13886b3c7cb855952109b4bf5bdeae9ee280eefbef1cd5d102784c95b2a0863
reviewed: true
---
# HDR 与杜比视界 { #hdr-and-dolby-vision }

**简单来说：**

- **HDR** 是一种比普通画面（称为 **SDR**）高光更亮、色彩更丰富的画面。杜比视界是 HDR 的一种。HDR 需要一块能显示它的屏幕。
- 在 HDR 屏幕上使用 Android 或 Android TV 应用时（设备还要能解码 HEVC），只要没有别的原因迫使转换，您看到的就是原始的 HDR 画面。
- 在任何不是 HDR 的屏幕上，服务器都会把画面转换成 SDR。这是有意为之：HDR 画面显示在 SDR 屏幕上，会显得灰暗、发白。SDR 显示器上的网页浏览器得到 SDR；报告自己是 HDR 显示器并能解码 HEVC 的浏览器则保留 HDR10 和 HLG。杜比视界仍然会被转换。
- 把 HDR 转换成 SDR，是处理器上开销最大的工作。显卡可以分担这项工作：见[硬件加速](../administration/hardware-acceleration.md)。

本页其余部分逐一说明每种情况：保留原始画面、重新编码但保留 HDR，或者转换成 SDR。播放的一般规则见 [Evey 如何选择播放方式](how-playback-is-decided.md)。

## 不同设备上您会看到什么 { #what-you-will-see-by-device }

| 您的屏幕 | 收到的是什么 |
| --- | --- |
| Android TV 或 Android 手机上的 HDR 屏幕，并且有 HEVC 解码器 | 原始的 HDR10 或 HLG 画面，不做任何改动，除非别的原因（码率、分辨率、要内嵌的字幕）迫使重新编码。 |
| 浏览器 | 在 SDR 显示器上转换成 SDR。当浏览器报告 HDR 显示器（`dynamic-range: high`）并能解码 HEVC 时，保留 HDR10 和 HLG；杜比视界仍然会被转换。 |
| 投屏接收端 | 取决于接收端对自己屏幕的报告：见[投屏到 Chromecast](casting.md)。 |
| 任何地方的 SDR 屏幕 | 转换成 SDR。 |

不经转换就把 HDR 显示在 SDR 屏幕上，颜色会发白、发灰，所以服务器从不这样发送。Android 应用会在每次开始播放时检测显示屏，而不是只在安装时检测一次：给机顶盒接上一台 HDR 电视后，从下一部影片、下一次切换轨道或切换画质开始生效。

## 什么时候保留原始画面 { #when-the-original-is-kept }

当播放器报告它的屏幕能显示 HDR，并且能解码这种编码，同时没有其他原因（码率、分辨率、容器）需要重新编码时，HDR10 或 HLG 视频会原样发送。

## 无论如何都要重新编码时 { #when-the-video-must-be-re-encoded-anyway }

码率限制、分辨率限制，或者要把字幕内嵌到画面里，都可能迫使 HDR 片源重新编码。如果下面这些条件全部满足，HDR 就会被保留：

- 播放器报告有 HDR 屏幕，并且能解码 HEVC；
- 服务器有一块显卡，在测量时证明了它能编码带 HDR 的 10 位 HEVC；
- 这块显卡此刻可以用来处理这个视频流。

结果是 10 位 HEVC，并沿用片源的 HDR 标记（BT.2020 色彩，PQ 或 HLG 传输特性）。HDR10+ 的动态元数据不会保留：在这种处理方式下，HDR10+ 影片会按 HDR10 播放。

只要有一个条件不满足，处理器或显卡就会先把画面色调映射成 SDR，再进行编码。这时流信息会列出 **HDR 已转换为 SDR（色调映射）**。

如果原始画面本可以发送，但服务器暂时还不能以直接复制的方式提供它（例如它的跳转索引还在构建中），视频会在这样的显卡上重新编码，并保留 HDR，流信息会显示 **无法直接复制，重新编码时保留 HDR**。

## 屏幕是 SDR 时 { #when-the-screen-is-sdr }

播放器没有报告 HDR 屏幕时，收到的是经过色调映射的 SDR 画面。如果显卡已经证明它能做色调映射（例如带 OpenCL 的 VAAPI），就在显卡上做；否则在处理器上做：如果 ffmpeg 版本提供了 SIMD 优化的滤镜（AVX2），就用它，否则用一组可移植的滤镜。

!!! note
    在处理器上，色调映射是开销最大的一种转码。服务器会为 HDR 单独测量一个分辨率上限，通常更低，所以在同一台机器上，HDR 影片转换后的分辨率可能比 SDR 影片低。见[转码](transcoding.md#the-server-measures-itself)。

## 杜比视界 { #dolby-vision }

播放器的报告只说明它的屏幕能否显示 HDR，不说明它是否支持杜比视界；Evey 也从不根据“支持 HDR”去推断“支持杜比视界”：把杜比视界发给只支持 HDR10 的屏幕，颜色会出错。只有当视频流带有一个不需要任何杜比视界解码就能正确显示的基础层时，才会直接复制：

| 杜比视界配置 | HDR 播放器 | SDR 播放器 |
| --- | --- | --- |
| **8.1**（基础层兼容 HDR10） | 按 HDR10 处理：适用上面的规则，可以保留原始画面。 | 色调映射成 SDR。 |
| **7**（UHD 蓝光） | HDR10 基础层被重新编码为 10 位 HEVC，并保留 HDR。流信息显示：**Dolby Vision 基础层已重新编码（保留 HDR）**。 | 色调映射成 SDR。 |
| **5**（没有 HDR10 基础层） | 色调映射成 SDR。 | 色调映射成 SDR。 |

## 查看实际情况 { #checking-what-happened }

**流信息**会显示对视频的处理，并在原因中列出色调映射或保留了 HDR。所有做色调映射的处理方式都会在那里报告，只有一个例外：如果视频流开始的那一刻显卡正好已满负荷，视频流会在计算出流信息之后才转到处理器上。**管理 → 正在播放**会显示实际使用的后端。

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateVideo (hdr_tonemapping_needed, dovi_base_layer, doViBaseLayerIsHDR10)
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable) ; internal/app/transcoding/resolver_route.go - naturalRoute, passthroughWanted, hwTonemapWanted
hdr signalling carried: internal/adapters/driven/ffmpeg/hls_args_hw.go - hwHDRPassthroughArgs (no HDR10+ metadata)
dovi copy rule: internal/app/transcoding/copy_eligibility.go (dovi_profile, DoViBLCompat)
who declares an HDR screen: frontend/packages/client/src/native/displayHdr.ts - probeDisplayHdr ; frontend/packages/client/src/native/playSession.ts (supportsHdr) ; frontend/apps/web/src/cast-receiver/receiverLogic.ts (hdr=1) ; frontend/packages/client/src/web/codec.ts - canDisplayHdr (dynamic-range: high)
tonemapx, software HDR ceiling: internal/adapters/driven/ffmpeg/tonemapx.go ; internal/pkg/hdr/hdr.go ; docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.* ; admin.nowPlaying
plain opening: rules below ; tone mapping cost: internal/app/transcoding/calibration_service.go (separate HDR ceiling)
-->
