---
source_sha: 3bd3ddf0df0a475d6ddf4417b5aa5b3630c871184888584df522bdae5daee7d4
reviewed: true
---
# 播放分发与下载

这些开关决定视频流怎样打包，以及服务器为下载准备什么。默认值就是推荐的做法：
这些开关的用途，是在排查问题时关闭某种处理方式，或者主动开启一种默认关闭的处理方式。
播放方式最初是如何选择的：见[Evey 如何选择播放方式](../playback/how-playback-is-decided.md)。

有面板键的变量，也可以在**管理 → 服务器配置 → 配置条目**里用这个键添加一行来设置；这一行优先于环境变量。
面板会提示下载相关的键；其他的键请按这里写的样子输入。它们都在启动时读取，
所以不管用哪种方式修改，都要到下次重启才生效。

## 打包

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | 开启 | 如果文件的轨道可以直接复制，就在发送时即时重新打包，不用 ffmpeg，也不写入缓存。`off`：这类文件改用 ffmpeg 的复制方式。 | 配置条目 `transcoding.hls_ondemand`，需重启 |
| `EVEY_REMUX_COPY` | 开启 | ffmpeg 的复制方式：文件允许时，视频直接复制，不重新编码。`off`：这些文件改为转码。 | 配置条目 `transcoding.remux_copy`，需重启 |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | 把各条音轨作为单独的音频版本提供，这样播放器换语言时不用重新获取视频。也使直接复制的视频可以与另外重新编码的音轨一起播放。 | 无 |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | 在直接复制的视频流旁边，再加一个转码出来的低画质，这样网络突然变差时，播放器可以降低一档画质，而不是卡住。在播放器用到它之前，不消耗任何资源。 | 无 |
| `EVEY_HLS_ONDEMAND_CONCAT` | 开启 | 网页播放器请求时，剧集之间无缝衔接播放。不能直接复制的一集会改用普通的视频流。 | 配置条目 `transcoding.hls_ondemand_concat`，需重启 |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | 关闭 | 每个视频流里只放选中的那条音轨，而不是全部音轨。 | 配置条目 `transcoding.hls_ondemand_track_trim`，需重启 |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | 关闭 | 用 AES-128 加密重新打包的视频流（只限 HLS：开启后不再提供 DASH 清单）。 | 配置条目 `transcoding.hls_ondemand_encrypt`，需重启 |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | 开启加密时，每 N 个片段换一次密钥；`0` = 每部影片一个密钥。 | 配置条目 `transcoding.hls_key_rotation_segments`，需重启 |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | 让 Chromecast 自己解码杜比数字（Dolby Digital），而不是接收 AAC。默认关闭，因为它会让低端接收器负担过重，又没有带来任何好处。浏览器不受影响。 | 无 |

## 下载

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | 开启 | 准备一个手机能播放音频（AAC）的下载版本，视频保持不变。只有音频重新编码，需要几分钟。 | 配置条目 `transcoding.compat_download`，需重启 |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | 关闭 | 为下载提供一个更小的 720p 版本。它要重新编码视频，在性能弱的机器上大约和片长一样久，所以在您开启之前它是关闭的。 | 配置条目 `downloads.quality_choice`，需重启 |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296`（4 GiB） | 超过这个大小，应用就会提供更小的版本；手机存储空间放不下电影时，手机也会提供它。 | 配置条目 `downloads.reduce_above_bytes`，需重启 |

## 播放器

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | 快进按钮跳过的秒数。 | 无 |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | 后退按钮跳过的秒数。 | 无 |

<!-- sources
packaging flags: internal/adapters/driving/http/transcode_vod_wire.go (serverFlagEnabled, resolveServerFlag: hls_ondemand, remux_copy, hls_ondemand_concat, hls_ondemand_track_trim, hls_ondemand_encrypt, hls_key_rotation_segments)
renditions, fallback rung, cast wasm: internal/config/config.go - AudioRenditionEnabled, CopyFallbackRungEnabled, CastWasmAudioEnabled
downloads: cmd/evey/http.go (compat_download, quality_choice, reduce_above_bytes via resolveFlag)
config entries catalogue: frontend/apps/web/src/features/admin/operatorFlags.ts ; i18n admin.serverConfig.entriesTitle, appliedOnRestart
skip: internal/config/config.go - EVEY_PLAYBACK_SKIP_*
-->
