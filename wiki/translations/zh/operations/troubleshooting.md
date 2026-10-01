---
source_sha: 61f4d1f588a54015fae1a0ed84c6418d67bac574fc6ddfb9782f3a32596b4ba4
reviewed: false
---
# 故障排除

从症状入手。每一项都说明首先要检查什么，以及详细步骤在哪里。

## 首先检查

两条命令可以回答关于一台运行异常的服务器的大多数问题：

```bash
docker compose logs --tail=200 evey
docker exec evey-server evey admin doctor
```

`evey admin doctor` 会检查数据库及其架构、签名密钥、搜索索引、媒体挂载（包括服务器能否写入每个媒体库）、缓存目录、ffmpeg、远程访问和可用磁盘空间。它为每项检查输出一行并附带补救办法，任何一项失败时以非零状态退出。

在管理面板中，**日志与调试** 显示同样的日志，可以搜索并按领域（播放、转码、扫描……）筛选。

## 服务器无法启动

| 日志内容 | 含义 |
| --- | --- |
| 一条配置校验消息 | 使用外部数据库时，服务器拒绝默认密码 `evey`；使用外部 Meilisearch 时，拒绝空的或占位的密钥。修正消息中指出的值。 |
| 以 `evey:` 开头、关于 Postgres 的行 | 内置数据库拒绝启动：来自另一个 Postgres 主版本的集群（[Postgres 主版本升级](advanced-deployments.md#postgres-major-upgrade)）、不是 Postgres 集群的数据目录、磁盘已满，或者 `./evey-data/postgres` 位于网络共享上。 |
| 在 `/cache/...` 上出现 `permission denied`，并不断重启 | `evey-cache` 卷属于与服务器运行用户不同的用户。更新镜像；对于 2026-09-17 之前的镜像，运行一次 `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`。 |

找回丢失的管理员密码、处理泄露的签名密钥或损坏的数据库：[恢复](../runbooks/recovery.md)。

## 在家庭之外无法访问

服务器在家里能响应但在别处不行，或者应用显示远程访问已禁用的消息：见[远程访问与 HTTPS](../administration/remote-access.md)。当服务器可以从互联网访问而远程访问是关闭的时，管理页面 **远程访问** 会用红色提示。

完全没有响应，即使在家里也不行：[服务器无法访问](../runbooks/incident-runbook.md#1-server-not-reachable)。

## 所有人都被登出，或者每次登录都失败

[所有登录都返回 401](../runbooks/incident-runbook.md#2-all-logins-return-401)。常见原因是数据库（每个请求都会向数据库核对账户，数据库无法应答时请求会被拒绝；日志会说明这一点），以及反向代理未列入 **受信任的代理**，导致所有人共用同一个登录频率限制。仅仅丢失或替换签名密钥不会让任何人登出：应用会续期它们的令牌。

## SSO 登录失败

登录页显示“SSO 登录失败”：

- 该用户已经拥有一个电子邮件或用户名相同的本地账户；
- 其地址不在 `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` 之内，或提供方没有将其标记为已验证（日志显示 `oidc: email domain not allowed` 或 `oidc: unverified email rejected under domain allowlist`）。

如果是提供方自己显示的错误页，通常意味着在那里注册的重定向 URI 与 Evey 使用的不一致。见 [OIDC / SSO 设置](../oidc.md)。

## 收不到电子邮件

**Admin → 服务器配置 → 邮件 / SMTP** 中的 **发送测试邮件** 会用已保存的设置向你自己的邮箱发送一封邮件，失败时显示邮件服务器的应答。TLS 模式与端口不匹配（`starttls` 对应 587，`ssl` 对应 465）是常见错误。卡片中保存的主机会取代所有 `EVEY_SMTP_*` 变量。发送失败的通知邮件会以 `email: send notification` 记录。见[通知](../notifications.md)。

## 你本以为是直接播放的条目却在转码

在播放器中打开 **流信息**：它会说明播放方式和原因。如何解读它们，见[Evey 如何选择播放方式](../playback/how-playback-is-decided.md)。

## 播放停止、缓冲或无法开始

- 只有一个条目：打开 **Admin → 媒体健康**。没有跳转索引的文件、损坏的片段或按块存储的轨道都会导致卡顿和跳转缓慢，其中大多数可以就地修复。见[媒体健康](../runbooks/media-health.md)。
- 多个条目、多人观看时：服务器可能已达到转码容量。见[转码与服务器容量](../playback/transcoding.md)。
- 转码播放完全失败：[转码失败](../runbooks/incident-runbook.md#3-transcoding-fails--videos-wont-play-transcoded)。
- 一直缓冲而日志没有说明原因：[诊断步骤](../runbooks/incident-runbook.md#3c-it-buffers--it-cut-out-and-the-logs-do-not-say-why)。
- **Admin → 流媒体质量** 显示真实观众在过去一天或一周内播放等待的频率，以及受影响的条目和设备。

## 没有声音，或声音不同步

- 只在浏览器中、并且是杜比数字（AC-3/E-AC-3）音轨：设置了自己的 Content-Security-Policy 的反向代理可能会阻止网页播放器使用的解码器。见[音频](../playback/audio.md#dolby-digital-in-a-browser)。
- 某个条目不同步：**Admin → 媒体健康** 会检测在画面之后才开始的音频，以及互不一致的音轨。

## 字幕不显示

见[字幕](../playback/subtitles.md)：播放器能绘制哪些格式，以及哪些格式必须由服务器烧录进画面。

## 海报缺失或损坏

海报和背景图会在扫描期间复制到服务器的 `./evey-data/artworks` 中，缺失的每小时重试一次。**Admin → 图片缓存** 显示还有多少没有本地副本；**下载缺失图片** 会立即获取它们，**清理损坏的变体** 会删除缩放失败后留下的空白海报缩放版本，这些版本之后会在需要时重新生成。

## 搜索没有结果

从数据库重建搜索索引：`docker exec evey-server evey admin reindex-search`。搜索索引随时都可以重建；不会丢失任何东西。

## 投放会话卡住或没有声音

[投放卡住、没有声音，或者手机一直在转圈](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning)。

## 某个插件停止工作

[插件崩溃](../runbooks/incident-runbook.md#7-plugin-crashed)。它自己的日志在 **Admin → 插件** 中它的卡片上。

## 磁盘被填满

通常是转码缓存造成的：[转码缓存磁盘已满](../runbooks/incident-runbook.md#6-transcode-cache-disk-full)。可以随时在 **Admin → 转码缓存** 中清空它。
