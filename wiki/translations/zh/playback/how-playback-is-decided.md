---
source_sha: 6edbda5d141c4bdf890a36b49ddaf14c02a6bf242923470295dde702a2d0d3b5
reviewed: true
---
# Evey 如何选择播放方式

为什么同一部电影在电视上原样播放，在手机上却要经过转换？

**简单来说：**

- 您从来不用选择画质。按下播放时，服务器会看一看文件、您的设备能播什么，以及它自己有多忙，然后选一种最省力又能播的方式。
- 方式一共有三种：**直接播放**（文件原样发送）、**重封装**（画面和声音不变，换装进另一种文件格式）和**转码**（转换电影的一部分，常常只是声音）。
- 所以同一部电影可以原样发给一台设备，同时为另一台设备做转换；服务器忙的时候，同一台设备上的播放方式也可能不同。
- 通常您什么都不用做。想知道用的是哪种方式、为什么，可以在播放器里打开**流信息**：见[看懂流信息](#reading-stream-info)。

本页先介绍这三种方式，以及怎么看懂**流信息**。后面是服务器使用的具体规则，写给想弄明白或想调整它们的读者。

## 三种结果

| 在流信息里 | 播放器收到什么 | 服务器要付出什么 |
| --- | --- | --- |
| **直接播放** | 原始文件，一个字节都不变。 | 读取文件。 |
| **重封装** | 原始的视频和音频，装进另一种容器。不重新编码任何内容。 | 重新打包。能做的时候由服务器内部的 [MKVGo](mkvgo.md) 完成，否则用 FFmpeg 复制。 |
| **转码** | 至少有一条轨道被重新编码。常常只是音频，视频仍是原始的。 | 一次编码。只有重新编码视频才会占用一个[转码名额](transcoding.md#sharing-the-capacity)。 |

服务器按这个顺序尝试。即使要转码，它也只重新编码需要重新编码的轨道。

## 看懂流信息

**流信息**（播放器控制栏里的信息按钮 (i)；在电视上，位于播放器的选项菜单里）会先显示**方法**，然后列出每条轨道和对它的处理（**直接复制**、**已转码**，或者**客户端解码**，最后这种是指由网页播放器自己解码的音频），并在**为何无法直接播放**下面列出原因。

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![网页播放器中的流信息，重封装的情况](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>重封装：打包方式变了，视频和音频轨道保持原样，下面是文件不能原样播放的唯一原因。“直接复制”说的是轨道，不是播放方式（界面为英文）。</figcaption>
</figure>

原因分为四类：

| 原因从哪里来 | 显示的例子 |
| --- | --- |
| 播放器无法原样使用这条轨道 | 此播放器无法解码的视频编码 · 此播放器无法解码的音频编码 · 分辨率超出播放器上限 · 音频声道数超出此播放器支持范围 · 播放器无法解码的像素格式（10 位 H.264、4:2:2） · 容器无法直接播放（已重新封装为 HLS） |
| 这次播放受到的限制 | 码率超出允许上限 · 客户端测得的线路无法承载直拷 |
| 对画面或声音的影响 | HDR 已转换为 SDR（色调映射） · 字幕已内嵌到画面 · 已请求音频重新同步 |
| 某项设置，或者已经做出的决定 | 个人资料中已关闭直接播放 · 个人资料中已关闭重封装 · 首选视频编码（个人资料设置） · 本次会话已固定该选择 |

只看正在播放的那条音轨。如果文件里有一条没被选中的解说音轨是 DTS 格式，文件不会因此被转码。

流信息是在播放开始时计算的。只有一种情况它无法预见：视频流开始的那一刻，显卡正好满了。这时视频流会改由处理器来做，**管理 → 正在播放**会显示实际使用的是哪一个。

## 每一方提供什么

**文件。** 编码、分辨率、位深、HDR 格式、码率，以及音轨和字幕轨，这些在扫描文件时就读取好了。

**播放器。** 播放开始时，应用会报告它所在的设备能解码什么：支持哪些编码系列、能打开哪些容器、最高分辨率、屏幕能否显示 HDR、能输出几个音频声道，有些设备还会报告 H.264 更低的上限或级别。网页应用去问浏览器；Android 和 Android TV 应用去问设备的解码器和显示屏。Evey 不保存“支持哪些设备”的清单，它只用播放器报告的内容。投屏时，如果接收端能报告自己的能力，就由它自己来描述；否则使用一份保守的配置：见[投屏到 Chromecast](casting.md)。

**这次播放。** 个人资料里的偏好（最大码率、最大音频声道数、字幕处理方式），以及适用于这位观众的限制，见下文。

**服务器。** 它能不能实时完成这次编码（用显卡或处理器），以及有没有空闲的转码名额。这是实际测量出来的，不是假设的：见[转码与服务器容量](transcoding.md)。

## 同一个文件，三种结果

一部 4K HEVC 电影，HDR10，码率 70 Mb/s，带一条杜比 TrueHD 7.1 音轨，在一台 Android TV 上播放。这台电视的解码器支持 HEVC，屏幕能显示 HDR：

- **没有码率限制。** 电视能解码视频，也能显示 HDR，所以视频保持原样。它不能解码 TrueHD，所以只重新编码音频。流信息显示**转码**，视频为**直接复制**。
- **限制为 20 Mb/s，而显卡能编码带 HDR 的 10 位 HEVC。** 视频必须重新编码才能降到限制以内。服务器用 HEVC 重新编码，并保留 HDR，因为电视和显卡都支持。
- **同样的限制，但显卡正忙，或者这部片子不能用显卡。** 改由处理器来做。处理器编码成 H.264，在这里 H.264 不带 HDR，所以画面会经过色调映射转成 SDR，画质取决于处理器能稳定做到的水平。

文件和电视都没有变。变的是这次播放的限制和服务器的状态。

## 这次播放的限制

如果一部片子的码率（视频加上正在播放的音轨）高于下面这些限制中最低的那个，就会被转码降到这个限制：

- 播放器报告的最大值；
- 账户的**最大码率**，由管理员设置；
- 个人资料的**最大串流码率**；手机报告自己在用移动网络时，还有它的**最大蜂窝数据码率**；
- **最大远程码率**（**管理 → 远程访问**）和**视频码率上限（远程观众）**（**管理 → 转码**），取两者中较低的那个。只有当观众的地址是公网地址、并且不在 **LAN 网络**之内时才适用。私有地址永远不受限制。

如果正在播放的音轨的声道数多于播放器报告的数量，或多于个人资料里的**最大音频声道**，这条音轨就会被重新编码。重新编码后的音轨最多保留 5.1 声道，不会更多：见[音频](audio.md)。

## 服务器自己加的限制

有几项限制目前播放器不会报告，所以由服务器作为一条保守的兼容规则来执行，对所有播放器都一样：10 位 H.264，以及所有 4:2:2、4:4:4 或 12 位的视频，都会被重新编码；而 10 位及以下、4:2:0 的 HEVC、VP9 和 AV1，则看播放器是否支持这种编码。杜比视界只会以播放器能正确显示的形式保留：见[HDR 与杜比视界](hdr.md)。

## 为什么播放方式很少在中途改变

决定在播放开始时做出，并在整个会话中保持不变，这样播放器就不会收到来自两种不同编码的片段。如果切换音轨或字幕需要改变处理方式，或者重新开始播放这部片子，就会重新做一次决定。

## 强制使用某种方式

在**设置 → 播放**里，每个个人资料都有**允许直接播放**、**允许重封装**和**允许转码**。关掉其中一项，服务器就会跳过这种方式，流信息会显示原因。

!!! note
    这是一个诊断工具，不是画质设置：强制转码的效果永远不会比原始文件更好。

播放器设置里的**画质**是另一回事：它是在转码后的视频流的几种画质之间做选择。**自动**会根据网络情况调整；选一个高度（1080p、720p...）就固定在这个画质；**原画质**会取消播放器本来会要求的高度限制。

## 深入了解

- 详细标准：[HDR 与杜比视界](hdr.md) · [音频](audio.md) · [字幕](subtitles.md) · [投屏到 Chromecast](casting.md)
- 服务器如何分配它的能力：[转码与服务器容量](transcoding.md)
- 服务器内部不靠 FFmpeg 完成的工作：[Evey 中的 MKVGo](mkvgo.md)
- 具体实现：[播放链路](../playback-serving.md)

<!-- sources
decision: internal/app/transcoding/decision.go - MakePlaybackDecisionWithPrefs, evaluateVideo, pixelFormatPlayable, evaluateAudio, selectedAudioSpec, servedBitrateBps, resolveMaxBitrate, checkBitrate, applyAudioChannelTarget (aacMaxTargetChannels), applyDirectPlayPrefs
session: internal/app/transcoding/playback_plan_resolver.go - Decide, Reported, resolveDecided ; internal/app/transcoding/resolved_route.go - SpillReason (pool_full, title_blocked)
remote cap: internal/adapters/driving/grpcapi/playback_handler.go - resolvePlaybackPrefs ; internal/config/config.go - RemoteBitrateCapKbps
client caps: internal/domain/transcoding/playback_decision.go - ClientProfile ; frontend/packages/client/src/web/codec.ts ; frontend/packages/media-caps/src/castUrl.ts
example: internal/app/transcoding/testdata/route_golden.txt - "nvenc-pass | 2160p-hevc-hdr10-70m | hdr-tv{,-20m} | {plain,gpu-full,title-blocked}"
tone-map reported: internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.* ; player.quality.* ; auth.settings.playback.* ; admin.yaml maxBitrate (user limit) ; admin.remoteAccess.maxRemoteBitrate, lanNetworks ; admin.calibration.maxBitrateRemoteLabel
entry point: frontend/apps/web/src/features/playback/StreamInfoPanel.tsx ; frontend/apps/web/src/features/playback/PlayerSettingsPanel.tsx ; frontend/apps/mobile/src/components/player/PlayerControlsRow.tsx ; frontend/apps/tv/src/components/TrackOptionsMenu.tsx
adr: 0024, 0029, 0030
plain opening: docs/playback/transcoding.md ; i18n player.streamInfo.title
-->
