---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
---
# 管理 API

管理类接口，仅限拥有管理员权限的用户。

## 规范

### 系统
| 规范 | 说明 |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | 仪表盘、系统信息、安装向导 |
| [server_config.swagger.json](server_config.swagger.json) | 服务器配置的键值存储 |
| [remote_access.swagger.json](remote_access.swagger.json) | 远程访问与网络设置 |
| [backup.swagger.json](backup.swagger.json) | 数据库备份与恢复 |
| [activity.swagger.json](activity.swagger.json) | 播放历史、登录历史、服务器指标 |
| [admin_session.swagger.json](admin_session.swagger.json) | 审计日志 |
| [admin_task.swagger.json](admin_task.swagger.json) | 计划的后台任务 |

### 媒体与转码
| 规范 | 说明 |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | 删除媒体条目、刷新元数据、生成 trickplay |
| [metadata.swagger.json](metadata.swagger.json) | 元数据提供方、搜索、覆盖 |
| [transcoding.swagger.json](transcoding.swagger.json) | 转码配置、硬件检测、进行中的任务 |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | 转码缓存管理 |
| [mkv.swagger.json](mkv.swagger.json) | MKV 容器工具（轨道、章节、mux/demux） |

### 用户与策略
| 规范 | 说明 |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | 按用户的媒体库访问控制 |
| [parental.swagger.json](parental.swagger.json) | 家长控制与 PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | 全局播放设置 |

## 认证

所有接口都需要有效的管理员 JWT 令牌。
