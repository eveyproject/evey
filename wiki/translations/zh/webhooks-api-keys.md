---
source_sha: f8b9121424daaf442c366f75a6973b21de77d280d45f2cb7e7dadab38b7328be
reviewed: false
---
# Webhook 与 API 密钥

不编写插件就把 Evey 连接到其他工具的两种方式：Webhook 把 Evey 的事件发送到你的 URL，API 密钥让你的脚本调用 Evey。

## Webhook

**Admin → Webhooks → 创建 Webhook**：一个名称、一个 URL、要发送的事件，以及一个可选的密钥。**测试** 会发送一个 ping 并报告响应。

<figure class="evey-shot" markdown="span">
  ![Admin → Webhooks 中的一个 Webhook](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>一个发送每次播放开始和结束的 Webhook（界面为英文）。</figcaption>
</figure>

**事件。** Webhook 可以订阅所有事件，或者一份选定的列表。这些事件就是服务器本身的事件，例如：

| 领域 | 事件 |
| --- | --- |
| 媒体库 | `library.scan.started`、`library.scan.completed`、`media.item.added`、`media.item.updated`、`media.item.removed` |
| 播放 | `playback.started`、`playback.stopped`、`media.marked.watched`、`transcode.started`、`transcode.failed` |
| 账户 | `user.created`、`user.login.succeeded`、`user.login.failed`、`user.profile.created` |
| 请求 | `media.request.created`、`media.request.answered` |
| 服务器 | `backup.created`、`backup.failed`、`plugin.installed`、`plugin.error`、`server.config.changed` |

**投递。** 每个事件都是一个 JSON `POST`。随附两个请求头：

- `X-Evey-Delivery`：一个在同一次投递的多次重试之间保持不变的标识符，这样你的接收端可以忽略重复的请求；
- `X-Evey-Signature`：`sha256=` 后跟用 Webhook 的密钥计算出的请求体 HMAC-SHA256。用同一个密钥验证它，就能知道请求来自你的服务器。没有密钥时，签名什么也证明不了。

失败的投递会在 1、5 和 30 秒后重试。每次尝试都会记录在 Webhook 的投递日志中。

**只允许公网地址。** Webhook URL 必须是 `http` 或 `https`，并指向一个公网地址。私有和本地地址（`192.168.x.x`、`10.x.x.x`、`localhost`、解析到这类地址的容器名称）会被拒绝，在保存 Webhook 时以及服务器连接时都会检查。这可以防止服务器被用来访问你的内部网络；位于本地网络中的接收端需要一个公网地址或一个中继。

## API 密钥

API 密钥让脚本无需密码即可调用服务器 API。密钥由管理员在 **Admin → API 密钥** 中创建，并属于该管理员。

- **密钥只显示一次**，在创建时。请在那时保存它；之后无法再次显示，只能撤销。
- **它以其所有者的身份行事**，在其权限范围内拥有管理员权限。
- **权限范围** 限制密钥能做什么：

| 权限范围 | 允许 |
| --- | --- |
| `media:read` | 读取媒体元数据 |
| `media:write` | 修改媒体并启动扫描 |
| `playback:read` | 读取播放状态和历史 |
| `playback:write` | 控制播放和进度 |
| `users:read` | 读取用户 |
| `users:write` | 修改用户 |
| `admin:read` | 读取管理活动和统计 |
| `admin:write` | 修改管理设置 |

**没有任何权限范围的密钥拥有完全访问权限**，与其所有者相同：这是为现有密钥保留的旧行为。请选择脚本需要的权限范围。

密钥放在 `Authorization` 请求头中，带或不带 `Bearer` 都可以：

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

密钥被撤销或其所有者被禁用时，它会立即停止工作。端点本身列在[服务器 API](api/README.md) 中。
