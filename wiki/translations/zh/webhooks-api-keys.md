---
source_sha: 09be615f048b42a5c91430ee4558b2bc906d1e19a1f0c51eb2577ffa9d579533
reviewed: true
---
# Webhook 与 API 密钥

不写插件，也有两种办法把 Evey 和其他工具连起来：Webhook 把服务器的事件发送到您指定的网址，
API 密钥让脚本以您的身份调用服务器。两者都由管理员在管理面板里创建；
本页写给负责设置它们的人，也写给编写接收端或脚本的人。

## 把事件发送到一个网址

**管理 → Webhooks → 创建 Webhook**：填写**名称**、**URL**、要发送的**事件**，以及一个可选的**密钥**，用于签名。
**测试**会发送一个 ping，并显示对方的回应。

<figure class="evey-shot" markdown="span">
  ![管理 → Webhooks 中的一个 Webhook](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>一个在每次播放开始和结束时发送通知的 Webhook（界面为英文）。</figcaption>
</figure>

**事件。** Webhook 可以订阅**所有事件**，也可以只订阅您挑选的一部分。表单里列出了服务器发布的全部事件，其中包括：

| 领域 | 事件 |
| --- | --- |
| 媒体库 | `library.scan.started`、`library.scan.completed`、`media.item.added`、`media.item.updated`、`media.item.removed`、`metadata.refreshed` |
| 播放 | `playback.started`、`playback.stopped`、`media.marked.watched`、`transcode.started`、`transcode.failed` |
| 账户 | `user.created`、`user.login.succeeded`、`user.login.failed`、`user.profile.created`、`user.apikey.created` |
| 请求 | `media.request.created`、`media.request.answered` |
| 服务器 | `backup.created`、`backup.failed`、`plugin.installed`、`plugin.error`、`server.config.changed`、`transcoding.derived_cache_full`（字幕存储的剩余空间低于预留余量） |

调用 `EmitEvent` 的插件会发布 `plugin.event.<type>`，订阅了**所有事件**的 Webhook 也会收到它。

**投递。** 每个事件都是一个 JSON 格式的 `POST` 请求，带有两个请求头：

- `X-Evey-Delivery`：一个标识符，同一次投递的多次重试都使用同一个值，这样您的接收端就能忽略重复的请求；
- `X-Evey-Signature`：`sha256=` 后面跟着请求体的 HMAC-SHA256，用 Webhook 的密钥计算。
  用同一个密钥验证它，就能确认请求来自您的服务器。没有设置密钥时，这个签名什么也证明不了。

每次尝试最多等待 30 秒的回应。投递失败后，会在 1 秒、5 秒和 30 秒后重试。每次尝试都会被记录；
这份历史要通过 API 读取（[插件 API](api/plugins/README.md) 里的 `ListWebhookDeliveries`），在面板里看不到。

!!! warning "只允许公网地址"
    Webhook 的 URL 必须是 `http` 或 `https`，并且指向一个公网地址。
    `localhost`、私有地址和链路本地地址（`192.168.x.x`、`10.x.x.x`、`172.16.x.x`……），
    以及解析到这类地址的域名，都会被拒绝：保存 Webhook 时检查一次，服务器发起连接时再检查一次。
    这样可以防止有人借服务器之手访问您的内部网络；如果接收端在您的本地网络里，它需要一个公网地址或一个中继。

## 从脚本调用服务器

API 密钥让脚本不用密码就能调用服务器 API。密钥由管理员在**管理 → API 密钥 → 新建 API 密钥**里创建，归这位管理员所有。

- **密钥只显示一次**，就在创建的时候。请当场保存好；之后无法再显示，只能用**撤销**作废。
- **它以所有者的身份行事**，在自己的权限范围内拥有管理员权限。
- **权限范围**限制密钥能做的事：

| 权限范围 | 允许 |
| --- | --- |
| `media:read` | 读取媒体元数据 |
| `media:write` | 修改媒体，启动扫描 |
| `playback:read` | 读取播放状态和历史 |
| `playback:write` | 控制播放和进度 |
| `users:read` | 读取用户 |
| `users:write` | 修改用户 |
| `admin:read` | 读取管理活动和统计数据 |
| `admin:write` | 修改管理设置 |

不带任何权限范围创建的密钥（**全部（旧版）**）可以完整访问所有非管理类方法，和它的所有者一样；
但管理类方法总是要求对应的权限范围，所以旧版密钥到不了这些方法。请只勾选脚本需要的权限范围。

密钥放在 `Authorization` 请求头里，带不带 `Bearer` 都可以：

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

密钥一旦被撤销，或者它的所有者被停用，就会立即失效。端点本身列在[服务器 API](api/README.md) 里，那里也介绍了其他身份验证方式。

<!-- sources
form labels: i18n admin.webhooks.form.* (name, url, secret, events, allEvents), admin.webhooks.createWebhook, common.test, admin.webhooks.testPingSent
event list: frontend/packages/client/src/lib/webhookEvents.ts - WEBHOOK_EVENT_TYPES ; EventName() in internal/domain ; plugin events: internal/adapters/driving/pluginhost/host_server.go - pluginEmittedEvent.EventName
delivery, headers, timeout, retries, signature: internal/app/webhook/service.go - handleAny, attemptDeliver, sign, defaultRetryDelays, deliveryTimeout ; internal/domain/webhook/model.go - MatchesEvent ; history: proto/evey/v1/webhook.proto - ListWebhookDeliveries
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL, isPrivateIP ; internal/pkg/ssrf/dialer.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey (Bearer optional, evey_ prefix), authorize (admin methods always enforce the scope) ; internal/pkg/auth/rbac.go (scopes) ; rbac.go - CreateAPIKey RoleAdmin ; i18n auth.apiKeys.* (newKeyTitle, scopesAll, saveWarning, revoke)
-->
