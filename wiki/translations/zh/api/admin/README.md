---
source_sha: 8392e2d20e3a5b3c62e2f39d4743e925c172cdcb73dc639a5502e1bf4c16a66d
reviewed: false
---
# 管理 API

仅限具有管理员权限的用户使用的管理端点。

## 规范

### 系统
| 规范 | 说明 |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | 仪表盘、系统信息、设置向导 |
| [server_config.swagger.json](server_config.swagger.json) | 服务器配置键值存储 |
| [remote_access.swagger.json](remote_access.swagger.json) | 远程访问和网络设置 |
| [backup.swagger.json](backup.swagger.json) | 数据库备份和恢复 |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | 计划备份：cron、保留期、立即运行 |
| [activity.swagger.json](activity.swagger.json) | 播放历史、登录历史、服务器指标 |
| [admin_session.swagger.json](admin_session.swagger.json) | 审计日志 |
| [admin_task.swagger.json](admin_task.swagger.json) | 计划的后台任务 |
| [presence.swagger.json](presence.swagger.json) | 当前谁在线（应用的心跳和管理列表） |

### 媒体与转码
| 规范 | 说明 |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | 删除媒体条目、刷新元数据、生成 trickplay |
| [metadata.swagger.json](metadata.swagger.json) | 元数据提供商、搜索、覆盖 |
| [transcoding.swagger.json](transcoding.swagger.json) | 转码配置、硬件检测、活动任务 |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | 转码缓存管理 |
| [mkv.swagger.json](mkv.swagger.json) | MKV 容器工具（轨道、章节、复用/解复用） |
| [admin_intro.swagger.json](admin_intro.swagger.json) | 片头时间缓存：使单集失效、回填 |

### 用户与策略
| 规范 | 说明 |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | 按用户的媒体库访问控制 |
| [parental.swagger.json](parental.swagger.json) | 家长控制和 PIN 码 |
| [playback_settings.swagger.json](playback_settings.swagger.json) | 全局播放设置 |

## 身份验证

每个端点都需要管理员身份：管理员账户的会话令牌，或始终属于某个管理员的 API 密钥（其 scopes 可以加以限制）。
