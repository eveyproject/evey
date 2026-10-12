---
source_sha: 58ae53ccc12bc8dbe6e691e81203c38189833f7fe97b8f072c82a6641ae48c30
reviewed: true
---
# 故障排除

遇到问题？请在下表里找到您看到的情况。中间一列告诉您先看哪里，最后一列指向解决办法。如果还不够，日志和 `evey admin doctor`（见下文）会提供更多信息。

## 按症状查找该看哪里

| 您看到的情况 | 先看 | 然后 |
| --- | --- | --- |
| 服务器没有任何应答，在家里也一样 | Evey 在运行吗？先运行 `docker compose ps`，再看它输出的最后几行：`docker compose logs --tail=200 evey` | 下文的[服务器无法启动](#the-server-does-not-start)；[服务器没有应答](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| 应用显示**无法连接到服务器**或**连接失败** | 应用里输入的地址 | [连接应用](../getting-started/apps.md#the-server-address-to-enter)；仅在家庭网络以外无法连接时：[远程访问和 HTTPS](../administration/remote-access.md) |
| 所有人都被登出，或者所有人都无法登录 | **管理 → 远程访问**中的受信任代理；然后看日志 | 下文的[所有人都无法登录](#nobody-can-sign-in) |
| 电影没有画面、无法开始播放，或者播放中断 | 播放器里的**流信息**，然后是该影片在**管理 → 媒体健康**里的情况 | 下文的[播放问题](#playback-problems) |
| 卡顿或者一直在加载 | **管理 → 流媒体质量**（哪些影片和设备在等待）和**管理 → 系统健康**（服务器是否太忙） | [转码与服务器容量](../playback/transcoding.md) |
| 海报不对、标题不对，或者电影未被识别 | 该影片上的**编辑元数据** | [纠正错误或缺失的影片](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| 海报缺失或损坏 | **管理 → 图片缓存** | 下文的[媒体库和海报](#library-and-posters) |
| 新文件没有出现 | **管理 → 媒体库**里该媒体库那一行上的扫描结果 | 下文的[媒体库和海报](#library-and-posters) |
| 收不到邮件 | **管理 → 服务器配置 → 邮件 / SMTP**里的**发送测试邮件** | 下文的[收不到邮件](#e-mails-do-not-arrive) |
| 某个插件出错 | 它的**日志**，在**管理 → 插件**里它的卡片上 | [插件崩溃了](../runbooks/incident-runbook.md#a-plugin-crashed) |
| 投屏卡住或没有声音 | **管理 → 正在播放**：这次播放还在列表里吗？ | [投屏卡住、没有声音，或者手机一直显示加载中](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| 磁盘被占满 | **管理 → 转码缓存** | [缓存磁盘已满](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## 日志

日志记录服务器的运行情况：每个错误都会连同时间一起记录在其中。

**简单的方法：管理 → 日志与调试。** 它显示日志，带一个搜索框、一个跟踪新内容的**实时**视图，以及一个按方面筛选的**区域**过滤器（播放器和转换、字幕、片头检测、Chromecast、登录……）。有人报告问题时，请先来这里，查看对方说的那个时间前后的记录。

??? info "日志文件在哪里"

    - 文件位于 `./evey-data/logs/`，就在您的 compose 文件旁边（容器里是 `/data/logs/evey-YYYY-MM-DD.log`）。每天一个文件，JSON 格式，默认保留 90 天（`EVEY_LOG_RETENTION_DAYS`）。
    - `docker compose logs evey` 显示的是另一类内容：容器在启动过程中输出的内容（启动脚本的 `evey:` 行、数据库、启动横幅）。服务器无法启动时，请查看它。服务器运行起来以后，日志就在文件和管理面板里。

## `evey admin doctor`

一条命令就能检查整个服务器。每项检查输出一行，`ok`、`WARN` 或 `FAIL`，并说明该怎么做：

```bash
docker exec evey-server evey admin doctor
```

??? info "它检查什么"

    数据库及其结构、签名密钥、搜索索引（以及它是否与数据库一致）、媒体文件夹（包括 Evey 能否在每个媒体库里写入）、缓存和日志文件夹、`ffmpeg` 和 `mkvgo`、受信任的代理、远程访问，以及剩余磁盘空间。`--repair` 只执行安全的修复（创建缺失的文件夹并设置正确的所有者）。有检查失败时，它会以错误码退出。其他命令见[命令行](../runbooks/cli.md)。

## 服务器无法启动

| `docker compose logs evey` 显示的内容 | 含义 |
| --- | --- |
| 一条配置校验消息 | 使用外部数据库时，服务器拒绝默认密码 `evey`；使用外部 Meilisearch 时，拒绝空的或占位用的密钥。请修改消息里指出的那个值。 |
| 以 `evey:` 开头、关于 Postgres 的几行 | 内置数据库拒绝启动：数据来自另一个 Postgres 大版本（[升级 Postgres 大版本](advanced-deployments.md#move-to-a-newer-postgres-major)）、数据目录不是 Postgres 集群、磁盘已满，或者 `./evey-data/postgres` 位于网络共享上。 |
| `/cache/...` 上出现 `permission denied`，并且反复重启 | `evey-cache` 卷属于另一个用户，而不是服务器运行时所用的用户。请更新镜像；如果镜像早于 2026-09-17，请运行一次 `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`。 |

管理员密码丢失、签名密钥泄露或数据库损坏：见[恢复](../runbooks/recovery.md)。

## 所有人都无法登录

两个常见原因：

- **数据库没有应答。** Evey 每次登录都要查询数据库，无法查询时就拒绝登录。日志里会写明。
- **受信任的代理里漏掉了反向代理**（**管理 → 远程访问**）。这样一来，每个访客看起来都来自代理的地址，所有人共用同一个登录尝试次数上限。

逐步操作见：[每次登录都返回 401](../runbooks/incident-runbook.md#every-login-returns-401)。

!!! note
    只是丢失或更换签名密钥，并不会让任何人被登出：应用会更新自己的令牌。要有意结束所有会话，请使用 `evey admin rotate-jwt-key`，见[恢复](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens)。

服务器在家里有应答，在别处却没有，或者应用说远程访问已禁用：见[远程访问和 HTTPS](../administration/remote-access.md)。如果服务器能从互联网访问，而远程访问却是关闭的，**管理 → 远程访问**会用红字提示。

### SSO 登录失败

登录页会写明原因：

- **域名不被允许**：该地址不在卡片里允许的电子邮件域名之内；
- **没有匹配的账户**：注册已关闭；请开启**为新的 SSO 用户创建账户**，或者手动创建账户；
- **账户已被禁用**；
- **账户存在但无法关联**：提供方没有确认这个电子邮件、关联功能已关闭，或者该账户已关联到另一个身份；
- **不可用**：无法连接到提供方；卡片上的**状态**和**测试连接**会显示错误。

如果是提供方自己显示的错误页面，通常说明在那里登记的重定向 URI 与卡片上显示的不一致。见 [OIDC / SSO](../oidc.md)。

## 收不到邮件

**管理 → 服务器配置 → 邮件 / SMTP**里的**发送测试邮件**会用已保存的设置给您自己的地址发一封邮件，失败时显示邮件服务器的回复。常见的错误是 TLS 模式与端口不匹配（`starttls` 配 587，`ssl` 配 465）。卡片里保存的主机会取代所有 `EVEY_SMTP_*` 变量。发送失败的通知会在日志里记为 `email: send notification`。见[通知](../notifications.md)。

## 播放问题

先看播放器里的**流信息**。它会逐条音视频轨说明电影是怎么发送的（原样发送、复制到另一种格式，还是转换），以及原因。如果您以为某个影片会原样播放，它却被转换了：见[播放方式是如何决定的](../playback/how-playback-is-decided.md)。

- **某一个影片**播放中断、一直在加载，或者跳到另一个时间点很慢：打开**管理 → 媒体健康**。常见原因是文件没有用于跳转的索引、有一段损坏，或者音视频轨是按大块存放的。这些问题大多可以修复。见[媒体健康](../runbooks/media-health.md)。
- **多个影片，在多人同时观看时**：服务器可能已经达到了转换能力的上限。**管理 → 系统健康**显示它有多忙。见[转码与服务器容量](../playback/transcoding.md)。
- 转码播放直接失败：[转码失败](../runbooks/incident-runbook.md#transcoded-playback-fails)。
- 一直缓冲，日志里却没有说明原因：[诊断步骤](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why)。
- 只有在浏览器里没有声音，而且音轨是杜比数字（AC-3/E-AC-3）：自己设置 Content-Security-Policy 的反向代理，可能会拦截网页播放器所用的解码器。见[音频](../playback/audio.md#dolby-digital-in-a-browser)。
- 某个影片声音和画面不同步：**管理 → 媒体健康**能检测出比画面晚开始的音频，以及彼此不一致的音视频轨。
- 字幕不显示：[字幕](../playback/subtitles.md)一页说明了哪些格式由播放器绘制，哪些需要服务器内嵌到画面里。
- **字幕准备中…**持续好几分钟：文件里没有这条字幕轨的索引，服务器要把它从头到尾读一遍，只读一次。见[字幕第一次显示时](../playback/subtitles.md#the-first-time-a-subtitle-is-shown)。
- 需要转码的影片无法开始或卡住，而已经播放过的影片仍然正常，或者管理员收到**字幕缓存已满**：磁盘的剩余空间低于预留值。见[转码缓存](../playback/transcoding.md#the-transcoding-cache)。

## 媒体库和海报

海报和背景图会在扫描时复制到服务器上的 `./evey-data/artworks` 里，缺失的每小时重试一次。**管理 → 图片缓存**显示还有多少张没有本地副本；**下载缺失图片**会马上获取它们；**清理损坏的变体**会删除缩放失败而留下的空白海报，之后需要时会重新生成。

如果海报或标题属于另一部电影，那是元数据匹配的问题，不是图片的问题：见[纠正错误或缺失的影片](../administration/libraries.md#fix-a-wrong-or-missing-title)。

新文件通常几分钟内就会出现（见[新文件是怎样被发现的](../administration/libraries.md#how-new-files-are-picked-up)）。如果没有，请在**管理 → 媒体库**里该媒体库那一行点击**扫描**，然后查看结果。

如果扫描结束时显示**扫描不完整 - 这些文件夹无法读取：**，后面跟着一些路径，说明 Evey 无法读取这些文件夹（权限问题，或者网络共享断开了）。媒体库的其余部分照常扫描，无法读取的文件夹下的影片会被保留，而不是删除。请检查挂载和所有者（`PUID`/`PGID`，见[安装](../installation.md#common-variations)），然后重新扫描。

搜索什么都找不到：用 `docker exec evey-server evey admin reindex-search` 根据数据库重建搜索索引。不会丢失任何东西：索引随时都可以重建。

## 报告问题

请写信到 **contact@eveyproject.org**。一份可以据此处理的报告应该写明：

- 您做了什么、期望发生什么、实际发生了什么；
- 发生的时间，精确到分钟：日志是按时间查找的；
- 设备和应用：Android 应用里的**设置 → 关于 → 应用版本**；网页版则写明浏览器及其版本；
- 服务器的版本：`docker exec evey-server evey version`；
- 如果是播放问题，写明片名，并附上**流信息**的截图（见[读懂流信息](../playback/how-playback-is-decided.md#reading-stream-info)）：它写明了所选的播放方式及其原因，这已经是诊断的大部分内容。

管理员可以附上那个时间前后的日志（**管理 → 日志与调试**，见[日志](#the-logs)），以及 [`evey admin doctor`](#evey-admin-doctor) 的报告。日志里会出现账户名和网络地址：发送前请先看一遍。

安全漏洞不要在公开场合报告：请写信到 **security@eveyproject.org**，并在公开讨论之前留出修复的时间。

<!-- sources
symptom table: i18n mobile.serverConnect.unreachable / connectionFailed ; i18n media.rematchMetadata, admin.library.refreshMetadataTooltip
logs: internal/pkg/logger/logger.go - New (file handler only outside DevMode), rotatingWriter.rotate (evey-YYYY-MM-DD.log), StartCleanup (90 days) ; internal/adapters/driving/http/admin_handler_logs.go - currentLogPath ; docker-compose.yml EVEY_LOG_DIR=/data/logs, ./evey-data:/data ; i18n admin.logs.where.*, admin.logs.live
docker compose logs: docker/entrypoint.sh (evey: lines), cmd/evey/banner.go
doctor: cmd/evey/admin_commands.go - doctor ; cmd/evey/admin_ops.go - checkResult names (database, schema, jwt.key, meilisearch, search index, media <root>, trusted proxies, remote access, disk), checkTool ffmpeg/mkvgo
sso: internal/adapters/driving/http/auth_handler.go /login?oidc_error=<reason>, i18n auth.login.ssoError.* ; smtp: internal/adapters/driving/http/smtp_test_handler.go (test send), internal/adapters/driven/email/notification_emailer.go (log)
partial scan: internal/app/library/scan_service.go - scanDirectories (unreadable root = failed path, protected from pruning) ; i18n admin.libraries.scanPartial
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
rotate-jwt-key ends sessions: cmd/evey/admin.go - endAllSessions
startup failures: docker/entrypoint.sh ; cache ownership: docker/entrypoint.sh (re-own /cache)
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
reporting: docs/privacy.md (contact@eveyproject.org) ; docs/SECURITY.md (security@eveyproject.org) ; app version: frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx, i18n mobile.settings.appVersion ; server version: cmd/evey/cli.go
new files row: i18n admin.libraries.scan, scanPartial ; docs/administration/libraries.md (How new files are picked up) ; e-mail row: i18n admin.serverConfig.sendTestEmail
-->
