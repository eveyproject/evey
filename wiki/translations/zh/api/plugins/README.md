---
source_sha: a960630483d1b2a05980e34cb7d0eacb371e9de2acf7fcd4af72e9609a09a8cf
reviewed: true
---
# 插件 API

和插件有关的端点：安装和管理插件（管理员）、Webhook（管理员），以及插件界面为每个用户保存的偏好。
编写插件是另一套约定，通过本地套接字上的 gRPC 进行：请看[编写插件](../../plugins/dev-setup.md)和
[插件 API 参考](../../plugins/api-reference.md)。

| 规范 | 说明 |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | 安装、启用、禁用、卸载插件；配置、日志、资源限制、运行时；预览和应用更新；`IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | 创建、列出、修改和删除 Webhook；投递历史 |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | 插件界面按用户保存的偏好（`prefs` 权限） |

上传插件包是一个 multipart 请求 `POST /api/v1/admin/plugins/upload`
（字段 `plugin`、可选的 `sig`、`accept_permission_changes`）；`POST
/api/v1/admin/plugins/preview` 只做分析，不安装。两者都是管理员路由。

## 身份验证

插件管理和 Webhook 需要管理员身份，和[管理 API](../admin/README.md) 一样。`plugin_prefs` 是唯一为插件令牌设计的服务：
宿主页面为一个插件和一个用户签发这个令牌，而这个服务只返回属于这一对组合的数据（[服务器 API](../README.md#authentication)）。

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
