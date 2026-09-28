---
source_sha: 7d46731b1dfd44908d653fde2e55780a150fa9681de75fa6518859def0fe4dbd
---
# 扩展 API

用于管理扩展和 webhook 集成的接口。

## 规范

| 规范 | 说明 |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | 安装、启用、停用、卸载扩展。扩展的配置与日志。扩展运行时。 |
| [webhook.swagger.json](webhook.swagger.json) | webhook 的增删改查，投递历史 |

## 扩展开发

扩展通过 gRPC 与 Evey 服务器通信。完整的开发指南见[扩展文档](../../plugins.md)，内容包括：
- 扩展的生命周期（安装、启用、配置、停用、卸载）
- 可用的事件钩子
- 配置 schema
- 安全沙箱（seccomp、进程隔离）
