---
source_sha: 318633ab461ab7429280d1503d8bccfc464413f5ccc3377a7588c10f3d316c75
reviewed: true
---
# 音频 { #audio }

**简单来说：**

- 如果电影有您个人资料设定的语言，您听到的就是这种语言：在**设置 → 播放 → 默认音频语言**里设置。您在播放器里手动选择的音轨始终优先。
- 如果您的设备能播放，声音会原样发送。否则，服务器会把它转换成一种通用格式（AAC）。很多时候只转换声音，画面保持原样。
- 用手机扬声器、耳机或蓝牙时，Android 应用播放的是立体声。用 Android 或 Android TV 应用通过 HDMI 输出时，电视或功放能接收多少个声道，您就能得到多少个声道（例如 5.1）。
- 声音和画面不同步？见[声音不同步](#sound-out-of-sync)。

本页其余部分说明播放哪条音轨、什么时候直接复制、什么时候转换，以及有几个声道。这里提到的个人资料设置都在**设置 → 播放**里。

## 不同设备上您会听到什么 { #what-you-will-hear-by-device }

| 您的设备 | 收到的是什么 |
| --- | --- |
| 浏览器 | 浏览器能解码的音轨原样发送；AC-3 和 E-AC-3 在网页里解码（见下文）；其他的转成 AAC。环绕声的 AAC 音轨由浏览器按您的输出设备混缩。 |
| 使用扬声器、耳机或蓝牙的 Android 手机 | 立体声。应用声明的输出是立体声，所以环绕声音轨会由服务器重新编码成立体声，并做音量均衡的混缩。应用自己能解码的杜比数字会原样发送，由应用来混缩。 |
| 通过 HDMI 输出的 Android 手机或 Android TV | HDMI 接收设备报告能接收多少个声道，就给多少个：5.1 的音轨在 5.1 功放上仍是 5.1。 |
| 投屏接收端 | 取决于接收端对自己输出的报告：见[投屏到 Chromecast](casting.md)。 |

Android 应用会在每次开始播放时检测输出：插上耳机后，从下一部影片、下一次切换轨道或切换画质开始生效，不会在一个场景的中途切换。

## 播放哪条音轨 { #which-track-plays }

在影片页面或播放器里手动选择的音轨始终优先。否则，播放器先按个人资料的**默认音频语言**选，再选文件标记为默认的音轨，最后选第一条。如果文件里既有解说音轨也有普通音轨，标记为解说的音轨会被跳过，选用普通音轨。

只看正在播放的那条音轨：您没在听的 DTS 音轨，永远不会导致转码。

## 直接复制还是重新编码 { #copied-or-re-encoded }

如果播放器能解码这条音轨的编码，它的输出又有足够的声道，并且没有超出个人资料的**最大音频声道**（**立体声 (2.0)**、**环绕声 (5.1)** 或**环绕声 (7.1)**），这条音轨就直接复制。否则，它会被重新编码成 AAC：

- 声道数尽量多，以片源、设备和个人资料允许的为准，最多 5.1（7.1 的片源会混缩成每个播放器都能解码的 5.1；立体声的片源保持立体声）；
- 采样率最高 48 kHz：高于 48 kHz 的音轨仅凭这一点就会被重新编码（`audio_samplerate_exceeded`）；等于或低于 48 kHz 时，采样率不是理由。

在转码或重封装的视频流里，直接复制的音轨还必须是播放器在视频流片段中能接受的格式。AAC、AC-3 和 E-AC-3 总是可以；其他编码只有在播放器表示接受时才行。不满足时，这条音轨会被重新编码，流信息显示**音轨无法在流中原样传输**：播放器可能能解码文件里的 MP3，却不接受视频流里的 MP3。

## 浏览器里的杜比数字 { #dolby-digital-in-a-browser }

大多数浏览器不能解码 AC-3 或 E-AC-3。网页播放器不让服务器重新编码这条音轨，而是用一个在网页里运行的解码器自己解码：音轨原样发送，流信息显示它为**客户端解码**。能原生解码杜比数字的浏览器（例如 Safari）会把它当作普通音轨接收；选择哪种方式，是通过测试浏览器来决定的，而不是依据浏览器的类型。环绕声音轨只有在个人资料的**最大音频声道**允许时，才会在网页里解码：设为立体声的个人资料会改为收到重新编码的 AAC，它可以把声道混缩。

网页如何解码，以及无法解码时会发生什么：[Evey 中的 ac3go](ac3go.md)。

!!! warning
    如果反向代理设置了自己的 Content-Security-Policy，就必须在 `script-src` 里允许 `'wasm-unsafe-eval'`，否则解码器无法启动，电影会没有声音。见[反向代理配置](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own)。

## 声音不同步 { #sound-out-of-sync }

播放器里的**音频同步**可以为当前影片移动声音（正值让声音延后播放）。当视频流由服务器以重封装或转码的方式提供时，才有这个选项；在直接播放时，这个控件会显示**仅在转码播放时可用**。应用一个偏移量后，视频流会从当前位置重新开始。直接复制的音轨无法移动，所以服务器会重新编码它，流信息显示**已请求音频重新同步**。

如果偏移就在文件本身里（音轨比画面开始得晚，或者几条轨道之间互相不一致），**管理 → 媒体健康**会报告它，但从不自动改写文件：开始得晚的偏移，通常是配音特意对齐的结果。在这个文件那一行点击**检查同步**，您可以通过观看来判断，并应用一个偏移量，这个改动可以在它的历史记录里撤销。见[媒体健康](../runbooks/media-health.md#checking-the-sync-by-eye)。

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
device channel ceiling: frontend/packages/client/src/native/outputChannels.ts - resolveMaxOutputChannels (STEREO_ONLY_TYPES, hdmi.maxChannelCount) ; frontend/packages/client/src/native/playSession.ts - playerDecodesCopiedAudio ; internal/adapters/driving/http/client_capabilities.go - maxAudioChannelsQuery
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/app/transcoding/source_facts.go - AudioCopyCodecAllowedFor ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK ; internal/app/transcoding/decision.go - wasmAudioApplies ; frontend/packages/client/src/web/codec.ts
sample rate: internal/app/transcoding/decision.go - evaluateAudio (audio_samplerate_exceeded, maxCopiedAudioSampleRate) ; internal/app/transcoding/decision_samplerate_test.go
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; frontend/apps/web/src/features/playback/HLSPlayer.tsx (audioOffsetAvailable=transcodeMode) ; frontend/apps/mobile/app/(auth)/player/[id].tsx (audioOffsetAvailable)
media health: i18n admin.syncPreview.btn ; docs/runbooks/media-health.md
labels: i18n auth.settings.playback.maxAudioChannels, channelsStereo, channels51, channels71, audioLanguage ; player.settings.audioSync, audioSyncHint, audioSyncUnavailable ; player.streamInfo.decisionWasm, reason.audio_passthrough_unsupported, reason.audio_resync_requested
plain opening: frontend/packages/client/src/hooks/usePlayerTrackSelection.ts ; i18n auth.settings.playback.audioLanguage
-->
