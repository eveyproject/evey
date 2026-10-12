---
source_sha: cc4cecd7aee5e32086fae7de125434b98b804bf28bed8ea336efed0bb38c02bb
reviewed: true
---
# 管理 API

只有管理员才能调用的端点：网页应用里的**管理**调用的就是它们。每个服务一个文件，按领域分组。

### 系统
| 规范 | 说明 |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | 仪表板、系统信息、设置向导 |
| [server_config.swagger.json](server_config.swagger.json) | 服务器配置的键值存储 |
| [remote_access.swagger.json](remote_access.swagger.json) | 远程访问和网络设置 |
| [backup.swagger.json](backup.swagger.json) | 数据库备份和恢复 |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | 定时备份：cron、保留数量、立即运行 |
| [activity.swagger.json](activity.swagger.json) | 播放历史、登录历史、服务器指标 |
| [admin_session.swagger.json](admin_session.swagger.json) | 审计日志 |
| [admin_task.swagger.json](admin_task.swagger.json) | 定时后台任务 |
| [presence.swagger.json](presence.swagger.json) | 此刻谁在线（应用发来的心跳和管理员看到的列表） |

### 媒体与转码
| 规范 | 说明 |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | 媒体健康（扫描、修复、历史和撤销）、删除媒体条目、刷新元数据、生成 trickplay 预览图 |
| [metadata.swagger.json](metadata.swagger.json) | 元数据提供方、搜索、手动覆盖 |
| [transcoding.swagger.json](transcoding.swagger.json) | 转码配置档、硬件检测、正在进行的任务 |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | 转码缓存管理 |
| [mkv.swagger.json](mkv.swagger.json) | MKV 容器工具（轨道、章节、封装/解封装） |
| [admin_intro.swagger.json](admin_intro.swagger.json) | 片头时间缓存：让某一集失效、补算 |

### 用户与规则
| 规范 | 说明 |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | 按用户控制媒体库访问 |
| [parental.swagger.json](parental.swagger.json) | 家长控制和 PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | 全局播放设置 |

## 身份验证

每个端点都需要管理员身份：管理员账户的会话令牌，或者一个 API 密钥。API 密钥总是属于某位管理员，
并且必须带有该方法要求的权限范围（没有任何权限范围的密钥到不了任何管理方法）。
插件令牌在这里一律被拒绝，无论它带有什么权限范围（[服务器 API](../README.md#authentication)）。

<!-- sources
admin methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - MethodRoles ; api key scope on admin methods: interceptors/auth.go - authorize ; plugin token floor: auth.go - authorizePluginToken
-->
