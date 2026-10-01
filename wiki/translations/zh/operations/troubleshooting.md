---
source_sha: 5c66f9a9802a391b1f183ae3f0864b667ba8bd97f8da08ace12cfd8995aecde8
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

[所有登录都返回 401](../runbooks/incident-runbook.md#2-all-logins-return-401)。常见原因是 `./evey-data` 中的签名密钥丢失或被替换。

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

## 搜索没有结果

从数据库重建索引：`docker exec evey-server evey admin reindex-search`。索引随时都可以重建；不会丢失任何东西。

## 投放会话卡住或没有声音

[投放卡住、没有声音，或者手机一直在转圈](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning)。

## 某个插件停止工作

[插件崩溃](../runbooks/incident-runbook.md#7-plugin-crashed)。它自己的日志在 **Admin → 插件** 中它的卡片上。

## 磁盘被填满

通常是转码缓存造成的：[转码缓存磁盘已满](../runbooks/incident-runbook.md#6-transcode-cache-disk-full)。可以随时在 **Admin → 转码缓存** 中清空它。
