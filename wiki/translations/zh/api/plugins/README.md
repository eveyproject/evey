---
source_sha: eea62b3dd7b2be9c176a9b8431d6ee58738ecf22c64c0f154bb1b1211878f293
reviewed: false
---
# 插件 API

用于管理插件和 Webhook 集成的端点。

## 规范

| 规范 | 说明 |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | 安装、启用、禁用、卸载插件。插件配置和日志。插件运行时。 |
| [webhook.swagger.json](webhook.swagger.json) | Webhook 的增删改查、投递历史 |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | 插件界面按用户保存的偏好（`prefs` 权限） |

## 插件开发

插件通过 gRPC 与 Evey 服务器通信。完整的开发指南见[插件文档](../../plugins.md)，包括：
- 插件生命周期（安装、启用、配置、禁用、卸载）
- 可用的事件挂钩
- 配置架构
- 安全沙箱（seccomp、进程隔离）
