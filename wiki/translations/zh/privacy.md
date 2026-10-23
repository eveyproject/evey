---
source_sha: 6731543d05eba23817144ccd5d21f648ca65b14ec0dfd93bf782752067af1ad1
reviewed: true
---
# 隐私政策 { #privacy-policy }

最后更新：2026-10-04。

本政策适用于 Android 和 Android TV 版的 Evey 应用。

简单来说：Evey 项目不运行任何接收您数据的服务。不需要在我们这里注册账户，没有使用统计，没有广告，也不会向我们发送崩溃报告。应用只和 Evey 服务器通信，这台服务器由您自己，或者邀请您的人安装和运行。

## 应用发送什么，发给谁 { #what-the-apps-send-and-to-whom }

**发给您的 Evey 服务器，不发给我们运营的任何其他服务**。应用只发送一个媒体播放器正常工作所需的内容：

- 登录时的用户名和密码（或快速连接的代码），之后是一个会话令牌，即用于保持登录状态的密钥；
- 您浏览、搜索、播放、评分、收藏或请求的内容，以及播放进度，这样您才能在另一台设备上继续观看；
- 设备名称（以 Android 报告的为准），这样您才能在**播放到**和服务器的会话列表里分清自己的设备；
- 技术性的播放报告：设备能解码哪些音频和视频格式，以及播放时遇到的错误，这样服务器才能选择设备能播放的流；
- 一张图片，仅在您选它作为个人资料头像时发送；以及您作为管理员自己选择的文件（外挂字幕、插件、备份）。

谁能读取这些数据，由这台服务器的运营者决定。Evey 项目无法访问这些数据。

**只在以下情况下发给第三方：**

- **海报和专辑封面。** 请求页面和音乐里显示的部分图片，直接来自它们的公开来源（The Movie Database、Cover Art Archive、IMDb）。设备会像网页浏览器一样获取这些图片，因此提供图片的主机能看到设备的 IP 地址。
- **投屏。** 投屏到 Chromecast 要经过 Google Cast，它是您手机上 Google Play 服务的一部分。Chromecast 会从 `cast.eveyproject.org` 加载 Evey 的接收端页面，这个页面托管在 GitHub Pages 上。影音内容本身直接从您的服务器传输。
- **Android TV 上的语音搜索。** 麦克风按钮会调用电视自带的语音识别（通常是 Google 的）。应用只收到识别出的文字，从不接收录音。
- **您打开的链接。** 预告片、The Movie Database 或 IMDb 的页面，或者用于更新的 Play 商店页面，会在相应的应用或浏览器中打开，适用该服务自己的政策。
- **插件。** 插件由服务器管理员安装。插件可能会显示来自外部服务的内容（比如天气插件）。

## 留在设备上的内容 { #what-stays-on-the-device }

服务器地址、您的会话令牌（保存在 Android 的加密存储中）、您的偏好设置，以及您下载用于离线观看的电影和音乐。卸载应用会把它们一起删除。生物识别解锁（指纹或人脸）在您开启后由 Android 处理：应用只知道验证是否成功。

## 权限 { #permissions }

| 权限 | 用途 |
| --- | --- |
| 互联网、Wi-Fi 状态 | 连接您的服务器，区分您的家庭网络和外部网络。 |
| 相机 | 扫描快速连接的二维码。不会录制或发送任何内容。 |
| 通知、前台服务 | 显示播放和下载进度，屏幕关闭后继续播放。 |
| 生物识别 | 可选的应用锁。 |

手机应用没有麦克风权限。

## 本网站 { #this-website }

`eveyproject.org` 托管在 GitHub Pages 上，GitHub Pages 能看到访客的 IP 地址。本网站自己提供字体和脚本，所以浏览器不会连接其他任何服务。网站不设置自己的 Cookie，也没有访问统计。

## 第三方服务 { #third-party-services }

本产品使用 TMDB API，但未经 TMDB 认可或认证。图片和元数据来自 [TMDB](https://www.themoviedb.org/)。

## 儿童 { #children }

这些应用不是面向儿童设计的。儿童个人资料由服务器的运营者创建，相关数据也由运营者保管。

## 联系方式 { #contact }

关于本政策的问题：[contact@eveyproject.org](mailto:contact@eveyproject.org)。关于 Evey 服务器上保存的数据，请联系运行这台服务器的人。

<!-- sources
no tracker: frontend/apps/mobile/package.json, frontend/apps/tv/package.json
permissions: frontend/apps/mobile/android/app/src/main/AndroidManifest.xml
device name: frontend/apps/mobile/src/hooks/useDeviceRegistration.ts
playback reports: frontend/packages/client/src/native/useAudioCapsBeacon.ts
outside posters: internal/app/mediarequest/service.go - trustedPosterHosts
cover art: frontend/packages/client/src/lib/artwork.ts - coverArtUri
cast receiver: docs/adr/0025-server-driven-cast.md
voice search: frontend/packages/voice-search/android/src/main/java/com/evey/voicesearch/EveyVoiceSearchModule.kt
store page: frontend/apps/tv/src/lib/appUpdate.ts - openStorePage
-->
