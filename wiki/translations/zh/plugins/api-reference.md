---
source_sha: 5e96cbd1483c320ab2ab021a2d894797d1cd5b0958ba8303a940db6026044ccb
reviewed: true
---
# 插件 API 参考 { #plugin-api-reference }

编写插件时要遵守的约定：`plugin.yaml` 的各个字段、后端和服务器之间互相调用的两个 gRPC 服务，
以及界面模块在自己的框架里拿到的 `window.evey` 桥接接口。一步步使用它们的教程见[编写插件](dev-setup.md)；
每个权限允许做什么、安装程序会拒绝什么，见[权限与插槽](security-model.md)。

## 清单 { #manifest }

每个 ZIP 包的根目录里都有一个 `plugin.yaml`（或 `plugin.yml`），最大 1 MB。

```yaml title="plugin.yaml"
name: my-plugin              # 必填；唯一，kebab-case 格式
version: 1.2.3               # 必填；semver，X.Y.Z 或 X.Y.Z-label
kind: plugin                 # 必填；"plugin" 或 "theme"
language: nodejs             # 插件必填；镜像里的一个运行时
entrypoint: node index.js    # 插件必填；服务器执行的命令行

description: "..."           # 可选
author: "..."                # 可选
min_evey_version: "1.0.0"    # 可选；会被接受但被忽略

permissions:                 # 后端和界面可以调用什么
  - network
  - media.read
external_fetch:              # 有 "network" 时必填；只写单纯的主机名
  - api.themoviedb.org
capabilities:                # 插件声称自己承担的角色
  - metadata.provider

web_root: public/            # 声明了 frontend 时必填；作为静态资源提供
frontend:
  - slot: home.section       # 允许的插槽之一
    src: public/front.js     # 必须位于 web_root 之下

config_schema:               # 插件卡片上“配置”面板里的字段
  api_key:
    type: password           # 永远不会发回给任何客户端
    label: API key           # 不会显示：面板用键名作为字段标签
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| 字段 | 规则 |
|---|---|
| `name`、`version`、`kind` | 必填。`version` 必须符合 `X.Y.Z` 或 `X.Y.Z-label`；更新包的版本必须高于已安装的版本。 |
| `language` | `kind: plugin` 必填：`python`、`nodejs`、`php`、`ruby`、`lua`、`bash`、`sh` 或 `go`。它决定检查哪个运行时、把哪些存根复制到入口旁边；它本身不启动任何东西。 |
| `entrypoint` | `kind: plugin` 必填。第一个词是解释器（声明了 `native-binary` 时是二进制文件），其余是它的参数；进程在插件的目录里运行。 |
| `permissions` | 每个值都必须在[权限目录](security-model.md#permissions-catalog)里。`network` 要求 `external_fetch` 不为空。 |
| `external_fetch` | 主机名。协议前缀和末尾的路径在解析时会被丢弃；主机必须完全匹配，`*.example.com` 这样的通配符在安装时会被拒绝，端口后缀会被忽略；IP 地址会导致安装被拒绝（[规则](security-model.md#external-fetch-rules)）。 |
| `capabilities` | `metadata.provider`、`media.read`、`media.write`、`webhook.receiver` 或 `notification.sender`；其他值都会被拒绝。主题不能声明任何能力。 |
| `web_root` | `frontend` 不为空时必填；必须是相对路径，不能含 `..`，不能含反斜杠。写 `.` 会把整个插件目录都提供出去，只会记录一条警告。 |
| `frontend[]` | `slot` 必须在[插槽列表](#slots)里；`src` 必须位于 `web_root` 之下。 |
| `config_schema` | 字段名到 `{type, label, description, required, default, min, max, options}` 的映射；`label` 不会显示，面板用键名作为字段标签。类型：`string`、`text`、`password`、`boolean`、`integer`、`number`、`select`（配合 `options: [{value, label}]`）。 |

主题（`kind: theme`）不能声明 `language`、`entrypoint`、`permissions` 或 `capabilities`；它的格式见[主题](../themes.md)。

**机密字段。** `type: password` 的字段，或者带有 `secret: true` 的字段，会照常保存并交给后端，
但从任何读取路由返回时都是遮盖过的，管理面板里的**配置**面板也一样。保存面板时，如果遮盖值没有改动，
就保留已保存的值，所以修改别的字段永远不会把机密清空。

## 后端：两个服务 { #backend-the-two-services }

后端进程和服务器通过两个 Unix 套接字用 gRPC 通信，套接字的路径通过环境变量传入：

```
EVEY_PLUGIN_ID=<plugin id>
EVEY_PLUGIN_SOCK=<path>   # 您的进程在这里监听，提供 PluginService
EVEY_HOST_SOCK=<path>     # 您的进程连接这里，调用 HostService
```

服务器根据内核报告的 PID（`SO_PEERCRED`）在套接字上验证进程的身份；不会交给进程任何令牌。
每个插件每分钟最多调用 600 次。约定以 `proto/plugin/v1/plugin_api.proto` 为准；插件启动时，
自带语言的存根会被复制到入口旁边（[准备工作](dev-setup.md#1-prerequisites)）。

### PluginService：服务器调用什么 { #pluginservice-what-the-server-calls }

| 方法 | 何时调用 |
|---|---|
| `Initialize(plugin_id, config, host_version)` | 进程绑定好自己的套接字之后，调用一次。`config` 带着管理员设置的 `config_schema` 的值。回答 `success: false` 并附上 `error`，就表示拒绝启动。 |
| `HealthCheck()` | 定期调用，用来区分正常运行的进程和无响应的进程。只要还能提供服务，就回答 `healthy: true`。 |
| `Shutdown(reason)` | 插件被禁用、更新、卸载，或服务器停止时。回答之后请尽快退出。 |
| `OnEvent(event_type, payload, source)` | 约定里有定义，但服务器目前不向插件投递任何事件：实现它并回答 `handled: true` 即可，目前它不会被调用。 |
| `GetCapabilities()` | 约定里有定义；服务器目前不调用它。 |

### HostService：插件能调用什么 { #hostservice-what-the-plugin-can-call }

如果清单没有声明某个调用需要的权限，调用会被拒绝，返回 `PermissionDenied`。标为*未提供*的方法会返回 `Unimplemented`；
它们属于为以后版本准备的约定，列在这里是为了不让存根误导您。

| 方法 | 权限 | 说明 |
|---|---|---|
| `Log(level, message, fields)` | 无 | 写入插件的日志，显示在插件卡片的**日志**下。 |
| `GetConfig(key)` / `SetConfig(key, value)` | 无 | 插件的配置：**配置**面板里的值，加上插件自己写入的键。最多 256 个键，键最长 256 字节，值最大 16 KiB。 |
| `GetData`、`SetData`、`DeleteData`、`ListDataKeys` | 无 | 下面介绍的[数据存储](#data-store)。 |
| `EmitEvent(event_type, payload)` | 无 | 以 `plugin.event.<event_type>` 的名字发布到服务器的事件总线上，Webhook 可以订阅它。 |
| `GetMediaItem(id)`、`ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` 或 `media.read` | 完整的媒体记录，包括 `file_path`、`provider_ids` 和 `genres`。 |
| `Fetch(url, method, headers)` | `network` | 访问沙箱外部的唯一途径：`http` 或 `https`，`GET` 或 `HEAD`，目标是 `external_fetch` 里的主机；私有地址在解析后和重定向时都会被拒绝。响应体最大 8 MiB（`truncated` 会注明是否被截断），整个请求最长 20 秒。服务器不会附加自己的任何凭据。 |
| `SearchMedia`、`GetMediaStreams`、`GetChapters`、`GetSubtitles` | `api.library` 或 `media.read` | *未提供。* |
| `UpdateMetadata` | `api.metadata` 或 `media.write` | *未提供。* |
| `GetUserPreferences` | `api.user` | *未提供。* |
| `SendNotification` | `notification.sender` | *未提供。* |

### 数据存储 { #data-store }

每个插件都有一个持久的键值存储，不需要声明任何权限：游标、缓存、上次同步的时间，任何需要在重启之后记住的东西都可以放进去。
值是不透明的字符串；JSON 请自己编码。插件 id 取自经过验证的套接字，所以插件只能读写自己的命名空间；
插件被删除时，整个存储也随之删除。

| 方法 | 用途 |
|---|---|
| `SetData(key, value)` | 创建或替换一个值。 |
| `GetData(key)` | 返回 `value` 和 `found`。 |
| `DeleteData(key)` | 删除一个键。 |
| `ListDataKeys()` | 列出插件的所有键。 |

这个存储对整个插件是共用的。**每个用户各自**的数据（比如正在看小组件的那个人的偏好）放在界面这一侧，通过下面的 `prefs` 权限保存。

## 前端 API { #frontend-api }

界面模块运行在一个来源不透明的框架里，策略是 `default-src 'none'`：无法访问宿主页面、宿主的 cookie 和存储，
不能用表单，不能弹出窗口，`connect-src` 也只限于 `external_fetch` 里的主机，且只能使用 `https`。
其他一切都通过 `window.evey` 进行，它在模块运行之前注入。同一个模块无需修改，就能在网页应用以及移动和电视应用里运行。

### `evey.onInit(callback)` { #eveyoninitcallback }

用宿主发来的上下文调用一次；如果回调是在上下文到达之后才注册的，会立即被调用。

```js
evey.onInit(function (data) {
  // data.config          - 插件的配置值（config_schema）
  // data.context         - 宿主上下文
  // data.locale          - 当前语言区域，例如 "en-US"
  // data.theme           - "dark" 或 "light"
  // data.surface         - "web"、"mobile" 或 "tv"
  // data.suggestedHeight - 仅 home.section：宿主建议的高度（px）
  // data.viewportHeight  - 仅 tv：面板的高度，单位 dp
});
```

### `evey.navigate(path)` { #eveynavigatepath }

让宿主应用跳转到 `path`。只有 `/home`、`/library`、`/media`、`/player`、`/settings` 和 `/search` 开头的路径会被执行；
其他路径（管理区域、登录页、绝对 URL）都会被直接丢弃，不给任何提示。

### `evey.toast(level, message)` { #eveytoastlevel-message }

在宿主界面里显示一条提示消息。`level` 取 `"success"`、`"error"` 或 `"info"`；`message` 是纯文本，超过 200 个字符的部分会被截掉；
每秒最多显示 3 条。

### `evey.resize(height)` { #eveyresizeheight }

请求把插槽的高度设为 `height` 像素，限制在 50 到 800 之间。`0` 会把插槽收起来：没有内容可显示的小组件不占任何空间。
`home.section` 小组件什么都不用做，打开时就是 `data.suggestedHeight` 的高度：宿主知道把您放在了哪个界面上，
会推荐首页小组件在那里应有的高度，这样每个小组件的大小都一致。只有在您确实需要不同的高度时，才需要调整。

### `evey.prefs.*`（权限 `prefs`） { #eveyprefs-permission-prefs }

当前用户在插件自己命名空间里的偏好，通过宿主为框架持有的令牌，绑定到（这个插件，这个用户）。
每个调用都返回一个 promise；缺少权限时 promise 会被拒绝。

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*`（权限 `public`） { #eveymedia-permission-public }

媒体条目的一个安全投影：id、标题、类型、年份、简介、海报 URL。永远不会有文件路径、文件大小、提供方 id 或流。

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });
const data  = await evey.media.poster(id);  // 海报，以 data: URL 的形式
```

`poster` 返回 `data:` URL，是因为框架的策略禁止直接从服务器加载图片；可以把它用作 `<img>` 的来源，或者当作纹理。

### `evey.assets.read(path)` { #eveyassetsreadpath }

按相对路径读取插件自己 `web_root` 里的一个文件，返回 `data:` URL：后端在运行时写出的文件，
就能在框架里直接使用，不用操心跨域问题。只能读到插件自己的资源。

### 在电视上 { #on-a-television }

电视应用在 WebView 里运行模块，一些在浏览器里成立的事情，在那里就不成立了：

- **不要按自己的视口来定尺寸。** 在 WebView 里，`window.innerHeight` 就是宿主刚刚分给您的插槽高度。
  请按 `data.viewportHeight` 或 `data.suggestedHeight` 来定尺寸。
- **没有焦点，也没有指针。** 小组件永远不会获得方向键的焦点，也没有悬停效果；任何需要交互的东西都应放在网页和移动端。
- **文字不要贴着边缘。** 电视会过扫描：每一边都留出大约 5% 的宽度和高度。
- **在真实的电视盒上测试 WebGL。** 着色器运行在电视盒自己的 GPU 和驱动上，而不是您电脑上的；噪声坐标要保持在有限范围内。
  `tests/plugins/weather-v2` 是以上所有要点的参考小组件。

## 插槽 { #slots }

| 插槽 | 出现在哪里 |
|---|---|
| `home.section` | 首页上的一个内容区块 |
| `home.hero` | 首页顶部的大横幅区 |
| `library.sidebar` | 媒体库视图里的一个侧边栏面板 |
| `media.detail.sidebar` | 媒体详情页上的一个侧边栏面板 |
| `player.overlay` | 视频播放器上的一层浮层 |
| `settings.user.appearance` | 用户设置里的外观部分 |
| `sidebar.bottom` | 主导航侧边栏的底部 |
| `admin.nav` | 管理菜单里的一个入口，打开插件自己的页面 |

`admin.nav` 是管理区域里唯一的插槽。它不会往已有的管理界面里注入任何东西：它只是添加一个菜单入口，
在同样的沙箱框架里、在一条仅限管理员的路由上打开插件的页面。申请这个插槽会显示在需要管理员同意的差异里。
其他所有 `admin.*`、`login.*`、`setup.*` 和 `profile.security.*` 插槽，安装时都会被拒绝。

## 能力 { #capabilities }

能力声明的是插件自称承担的角色：`metadata.provider`、`media.read`、`media.write`、`webhook.receiver` 或 `notification.sender`。
安装程序会检查名称，管理员会在预览里看到这份列表，以及更新时新增的项，**安全仪表板**也会记录它。
服务器目前不会根据能力转发任何东西：`metadata.provider` 收不到任何补全元数据的调用，`webhook.receiver` 收不到任何传入的请求。
插件实际能做什么，由它的**权限**决定。

## 界面的权限 { #permissions-of-the-interface }

`public` 和 `prefs` 和其他权限一样写在 `permissions` 下。它们不授予后端进程任何东西；它们解锁的是框架里的 `evey.media.*` 和 `evey.prefs.*`。
框架本身从不持有令牌：宿主签发一个限定于（插件，用户）、有效期五分钟的令牌，并代为转发调用；
服务器默认拒绝这个令牌的权限范围没有列出的所有方法
（[ADR-0012](https://github.com/eveyproject/evey/blob/master/docs/adr/0012-plugin-token-deny-by-default.md)）。

!!! note "插件永远不能申请的权限"
    `admin.*`、`auth.*`、`system.*`、`user.write` 和 `library.write` 被完全禁止：
    写了它们的清单会被拒绝，令牌永远不可能带有它们，而且无论令牌带有什么权限范围，管理方法都会拒绝插件令牌。

## 示例 { #examples }

`examples/plugins/` 里有按当前清单格式编写、可以直接运行的插件：Node.js 和 Python 的入门示例（`nodejs-example`、`python-example`）、
一个元数据提供方（`metadata-example`）、一个 Webhook 接收端（`webhook-receiver`）、一个带构建步骤的首页小组件（`weather`）、
一个演示数据存储、`public` 和 `prefs` 权限以及 `admin.nav` 插槽的示例（`data-demo`）、一个展示所有 `config_schema` 字段类型的示例，带有一个 `admin.nav` 页面（`config-demo`），
以及两个主题（`theme-dark-blue`、`open-workshop`）。`make plugin-zips` 会把它们全部打包。

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go ; interpreters: runner_start.go - knownInterpreters
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; frontend/packages/client/src/web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; frontend/packages/client/src/native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
