---
source_sha: 9ec00e9464587406ecdffdd125543fdaf6988a5989c62a715f8a88417d946b9f
reviewed: true
---

# 已知限制 { #known-limitations }

Evey 目前还做不到的事，在这里说清楚，免得有人在想看电影的那天晚上才发现。每一条都说明了可以改用什么。

## 没有 iPhone、iPad 或 Apple TV 应用 { #no-app-for-iphone-ipad-or-apple-tv }

Evey 有一个适用于 Android 手机和平板的应用，还有一个适用于 Android TV 和 Google TV 的应用。iPhone、iPad 和 Apple TV 都没有应用。在这些设备上，请在网页浏览器中打开服务器地址。

延伸阅读：[连接应用](apps.md)。

## Android 应用正在等待 Google 的审核 { #the-android-apps-are-waiting-for-googles-review }

Android 和 Android TV 应用已提交到 Google Play。审核通过之前，公开的应用页面不可见，只有受邀参加测试的帐户才能安装；[应用](apps.md)页面上的链接会在审核通过时生效。网页应用和电视浏览器中的 `/tv` 页面无需安装，所有人都能使用。

延伸阅读：[连接应用](apps.md)。

## 离线观看：仅限 Android 应用 { #watching-without-a-connection-android-app-only }

只有 Android 应用会在设备上保存电影或剧集的副本（**下载**），在 30 天内无需连接服务器即可播放。网页播放器、Android TV 应用和 `/tv` 页面都从服务器播放，全程需要网络连接。

延伸阅读：[每个应用的额外功能](apps.md#what-each-app-adds)。

## 应用关闭时手机上没有通知 { #no-notifications-on-the-phone-while-the-app-is-closed }

Android 和 Android TV 应用在打开时，会在**通知**页面中显示通知。应用关闭时，手机和电视上都不会有提醒。您在应用之外收到的通知来自网页浏览器（开启后，需通过 `https://` 访问），或者来自邮件（需由运行 Evey 的人设置好）。

延伸阅读：[通知出现在哪里](../notifications.md#where-notifications-appear)。

## 个人资料在网页或手机上管理 { #profiles-are-managed-on-the-web-or-on-the-phone }

Android TV 应用可以在个人资料之间切换，也能修改当前个人资料的部分设置，但不能创建或删除个人资料。请在网页应用或 Android 应用中操作。

延伸阅读：[用户、个人资料和家长控制](../administration/users.md#profiles-kids-profiles-and-the-pin)。

## 投屏：从 Chrome 或 Android 应用 { #casting-from-chrome-or-the-android-app }

**投屏**按钮在 Chrome 的网页播放器和 Android 应用中提供。Firefox 没有这个按钮，Android TV 应用也没有，因为它本身就在电视上。Safari 以及 Android 手机上的 Chrome 使用浏览器自带的远程播放，工作方式不同。

延伸阅读：[投屏到 Chromecast](../playback/casting.md)。

## Evey 从不自动更新 { #evey-never-updates-itself }

服务器会检查是否有新版本，并在管理仪表板上提示，但不会自行安装任何东西：运行它的人在方便的时候用两条命令更新。Android 应用在比服务器旧时会显示提示。

延伸阅读：[更新 Evey](../backups.md#updating-evey) 和[让应用保持最新](apps.md#keeping-the-apps-up-to-date)。
