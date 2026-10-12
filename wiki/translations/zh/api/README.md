---
source_sha: 5d6e1c496b373ae579e3de32d6359d6c1c9712b93ef8c17806d17afca066d8f8
reviewed: true
---
# 服务器 API

服务器的 HTTP API 用 OpenAPI（Swagger 2.0）文件来描述，每个服务一个文件。
本页告诉您哪个文件夹里有什么，以及调用是怎样验证身份的；真正的参考是这些文件本身。

| 部分 | 内容 |
|---------|-------------|
| [公开 API](public/README.md) | 已登录用户能做的事：浏览、播放、收藏、提交请求。 |
| [管理 API](admin/README.md) | 服务器管理：配置、转码、备份、元数据、用户。 |
| [插件 API](plugins/README.md) | 插件管理、Webhook，以及插件界面为每个用户保存的偏好。 |

每个 `.swagger.json` 文件都可以导入任何 OpenAPI 工具（Swagger UI、Postman、Insomnia）。
HTTP 路由是根据 `proto/evey/v1/` 里的 gRPC 定义生成的，所以文件里的方法名就是它背后的 gRPC 方法。

## 身份验证

能到达 API 的凭据有三种，都放在 `Authorization` 请求头里：

| 凭据 | 谁使用 | 怎样使用 |
|---|---|---|
| 会话令牌 | 通过应用登录的人 | `Bearer <token>`，登录时获得，由应用负责续期。 |
| API 密钥 | 脚本，代表创建它的管理员 | `Bearer evey_...`（`Bearer` 前缀可以省略）。有权限范围，可以撤销：[Webhook 与 API 密钥](../webhooks-api-keys.md)。 |
| 插件令牌 | 插件的界面，经由宿主页面 | 为一个插件和一个用户签发，有效期五分钟，只带有该插件获准的 `public` 和 `prefs` 权限范围；其他所有方法都会拒绝它。插件的框架页面从不持有这个令牌：调用由宿主页面代为转发。 |

无论带有什么权限范围，插件令牌都永远到不了任何管理方法
（[ADR-0003](../adr/0003-plugin-scoped-tokens.md)，
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md)）。

## 重新生成

写给贡献者：这些文件是根据 protobuf 定义重新生成的。

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
