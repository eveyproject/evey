---
source_sha: ae7eadc681e514dab83967eab7a3ef8dfadf53af27b3d1e4c676d5dd2d1176ec
reviewed: false
---
# 播放分发与下载

流如何封装，以及服务器为下载准备什么。默认值就是推荐的行为；这些开关的存在是为了在排查问题时关闭某条路线，或者启用一条默认关闭的路线。

带有键名的变量也可以在 **Admin → 服务器配置 → 配置条目** 中以该键名添加一行来设置；该行优先于环境变量。面板会提示下载相关的键；其他键按这里的写法手动输入。全部在启动时读取。

## 封装

| 变量 | 面板键名 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | `transcoding.hls_ondemand` | 开启 | 对于轨道可以直接复制的文件，通过即时重新封装来提供，不使用 ffmpeg，也不写入缓存。关闭：这类文件改走 ffmpeg 直接复制路线。 |
| `EVEY_REMUX_COPY` | `transcoding.remux_copy` | 开启 | ffmpeg 直接复制路线：在文件允许时，视频不重新编码而直接复制。关闭：这些文件会被转码。 |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | | `true` | 把音轨作为单独的 rendition 提供，这样播放器切换语言时无需重新获取视频。也让直接复制的视频可以搭配一条单独重新编码的音轨播放。 |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | | `true` | 在直接复制的流旁边加一个低画质的转码档位，这样连接变差的播放器可以降档而不是卡住。在有播放器使用它之前不产生任何开销。 |
| `EVEY_HLS_ONDEMAND_CONCAT` | `transcoding.hls_ondemand_concat` | 开启 | 当网页播放器请求时，在剧集之间无缝播放。无法直接复制的剧集会退回普通的流。 |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | `transcoding.hls_ondemand_track_trim` | 关闭 | 每个流中只放入选中的音轨，而不是全部音轨。 |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | `transcoding.hls_ondemand_encrypt` | 关闭 | 用 AES-128 加密重新封装的流（仅 HLS：此时不提供 DASH 清单）。 |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `transcoding.hls_key_rotation_segments` | `0` | 开启加密时，每 N 个分段更换一次密钥；`0` = 每个条目一个密钥。 |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | | `false` | 让 Chromecast 自己解码杜比数字，而不是接收 AAC。默认关闭，因为它让低端接收器过载，却没有节省任何东西。浏览器不受影响。 |

## 下载

| 变量 | 面板键名 | 默认值 | 作用 |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | `transcoding.compat_download` | 开启 | 准备一个手机能播放其音频（AAC）的下载，视频保持不变。只重新编码音频，需要几分钟。 |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | `downloads.quality_choice` | 关闭 | 为下载提供一个较小的 720p 副本。它会重新编码视频，在性能较弱的机器上大约以实时速度进行，这就是它在你开启之前一直关闭的原因。 |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `downloads.reduce_above_bytes` | `4294967296`（4 GiB） | 超过这个大小时应用会提供较小的副本；当电影放不下时手机也会提供。 |

## 播放器

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | 快进按钮跳过的秒数。 |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | 快退按钮跳过的秒数。 |
