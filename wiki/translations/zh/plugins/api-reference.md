---
source_sha: bb7b744c4f03cee0ce9a6cdfdd4b96cd6e99f6d71eee21c50f601fc4595ae619
reviewed: false
---
# 插件 API 参考

本文档面向编写 Evey 插件的外部开发者。关于施加在插件上的安全约束，见 [`security-model.md`](security-model.md)。

## 清单架构

每个插件 ZIP 的根目录都必须包含一个 `plugin.yaml`。

```yaml
name: my-plugin              # 必填；可读的名称
version: 1.2.3               # 必填；semver
kind: plugin                 # 必填；必须为 "plugin"（纯 CSS/JS 主题用 "theme"）
language: nodejs             # 插件必填；nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # 插件必填；解释器运行的完整命令行：
                              # 第一个词是解释器，在 PATH 中查找；
                              # 其余的词是脚本及其参数，路径限制在 ZIP 内。
                              # "language" 只用于检查运行时是否可用以及
                              # 复制 SDK 存根：它不会启动 entrypoint。

description: "..."           # 可选
author: "..."                # 可选
min_evey_version: "1.0.0"   # 可选；semver 格式的最低宿主版本

permissions:
  - network                  # 见权限一节
  - media.read

external_fetch:              # 声明了 "network" 权限时必填
  - api.themoviedb.org       # 见 security-model.md 中的外部访问规则
  - api.example.com          # 只能是纯主机名：不带协议、不用通配符、不用字面 IP

capabilities:
  - metadata.provider        # 见能力一节

# 可选：声明一个子目录，其内容作为静态资源提供。
# 声明了前端模块时必填。
web_root: public/

# 可选：注入宿主界面的前端模块。
frontend:
  - slot: home.section       # 见 security-model.md 中的插槽列表
    src: public/front.js     # 相对于 ZIP 根目录的路径；必须位于 web_root 之下

config_schema:               # 可选；描述插件配置键的 JSON Schema 对象
  api_key:
    type: string
    secret: true             # 或 `type: password`：永远不会发回给任何客户端，包括管理员
    description: "TMDB API key"
```

标记为 `secret: true`（或 `type: password`）的字段会照常保存并交给插件，但每条读取路由（公开的路由和管理配置面板）都会返回掩码后的值。当掩码原样提交回来时，管理面板会保留已保存的值，所以保存其他字段永远不会清除机密。

**安装时执行的验证规则：**

- `kind: plugin` 需要 `language` 和 `entrypoint`。
- `kind: theme` 不得声明 `language`、`entrypoint` 或 `permissions`。
- `frontend` 非空时必须有 `web_root`。
- `frontend[].src` 必须位于 `web_root` 之下。
- `frontend[].slot` 必须在服务器端的插槽白名单中。
- `network` 权限需要一个非空的 `external_fetch` 列表。
- 所有 `external_fetch` 条目都必须通过主机白名单验证器。
- 所有能力和权限都必须是服务器已知的。

## 后端 IPC

插件进程和 Evey 宿主通过两个 Unix 域 socket 以 gRPC 通信，它们的路径在启动时通过环境变量传入：

```
EVEY_PLUGIN_ID=<plugin_id>
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/plugin.sock   # 你的进程在这里监听并提供 PluginService
EVEY_HOST_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock       # 你的进程连接这里以调用 HostService
```

宿主根据内核报告的 PID（`SO_PEERCRED`）在 socket 上验证插件进程：不会为进程本身签发任何令牌。

一个单独的、短期的 JWT（5 分钟，范围限定为 `plugin_id` + `user_id`）会按需通过 `IssuePluginToken` RPC 签发：插件的*前端 iframe* 获取一个，用于验证它自己的 `public`/`prefs` 范围的调用。它与上面的后端 socket 无关，也从不在启动时交给插件进程。

gRPC 服务定义位于 `proto/plugin/v1/plugin_api.proto`。各语言的 SDK 存根由该 proto 生成，可以包含在插件 ZIP 中，或者随插件一起分发。有几种语言的存根是预先生成的（`sdk/<lang>/`）；要面向未附带的语言，见 [dev-setup.md → 为其他语言生成存根](dev-setup.md#generating-stubs-for-another-language)。

宿主在 RPC 层面执行权限：调用你的清单没有声明的 RPC 会得到 `PermissionDenied`。

### 数据存储

每个插件都有自己的持久化键值存储，命名空间属于该插件：**不需要任何权限，插件拥有自己的数据**。用它保存游标、缓存、上次同步的时间戳，以及插件需要在重启之间记住的任何东西。值是不透明的字符串（如果需要结构，请自己编码 JSON）。

| `HostService` RPC | 用途 |
|---|---|
| `SetData(key, value)` | 写入或更新一个值。 |
| `GetData(key) → {value, found}` | 读取一个值。 |
| `DeleteData(key)` | 删除一个键。 |
| `ListDataKeys() → {keys}` | 列出插件的键。 |

插件 id 取自经过验证的 socket：插件只能读写它**自己的**命名空间，永远无法访问另一个插件的。卸载插件时，它的整个存储会被自动清除（`ON DELETE CASCADE`），所以删除后不会留下过时的数据。

这个存储对插件是全局的。要**按用户**保存数据（例如当前观看用户的偏好），见下面的 `prefs` 权限：这个接口从插件的 iframe 访问，那里存在用户上下文。

## 前端 API

当插件声明了 `frontend` 模块时，iframe `srcdoc` 的 HTML 会包含一个桥接脚本，向模块的 JavaScript 暴露 `window.evey`。

前端模块运行在一个具有不透明来源的沙箱化框架中：它无法访问宿主页面的 DOM、Cookie 或存储，只能通过 `window.evey` 访问 Evey。它自己的网络访问被框架的 `connect-src` 限制为其 `external_fetch` 列表中的主机，且只能通过 `https://`；没有这个列表的插件无法打开任何连接。

### `evey.navigate(path)`

把宿主应用导航到 `path`。路径必须以 `/home`、`/library`、`/media`、`/player`、`/settings` 或 `/search` 开头；其他任何路径（管理区、登录、设置向导、绝对 URL 或协议相对 URL）都会被悄悄丢弃。

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

在宿主界面中显示一条提示通知。

- `level`：`"success"` | `"error"` | `"info"`
- `message`：纯文本，最多 200 个字符，HTML 会被去掉
- 速率限制为每秒 3 条

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

请求宿主把 iframe 插槽调整为 `height` 像素。限制在 50-800 之间。`0` 会折叠插槽：没有内容可显示的插件不会在页面上占用空间。

```js
evey.resize(300);
evey.resize(0); // 没有内容可显示：插槽消失
```

### `evey.onInit(callback)`

注册一个回调，在宿主向插件发送初始化上下文时触发一次。如果在调用 `onInit` 之前宿主已经发送了 `init`，回调会立即使用缓存的数据被调用。

```js
evey.onInit(function(data) {
  // data.context         - 插件配置值（来自 config_schema）
  // data.locale          - 当前语言字符串，例如 "en-US"
  // data.theme           - 当前主题 id
  // data.surface         - "web"、"mobile" 或 "tv"：小部件所在的位置
  // data.suggestedHeight - 仅 home.section：宿主在那里推荐的高度（px）
  // data.viewportHeight  - 仅 tv：面板高度，单位 dp
});
```

宿主最多等待 5 秒，等插件发出 `ready` 信号（由桥接脚本自动完成）。如果超时，iframe 会被卸载并报告错误。

#### 高度

`home.section` 小部件无需做任何事就会以 `data.suggestedHeight` 打开：宿主知道它把你放在了什么界面上（桌面页面、手机、电视面板），并推荐首页小部件在那里应有的高度，这样同一个首页上的所有小部件大小一致。只有在你更清楚的时候才调用 `evey.resize()`；宿主永远不会在你不知情的情况下调整或缩放你的内容。

#### 在电视上

电视应用在一个 WebView 中运行你的小部件，一些在浏览器中成立的事情在那里不再成立：

- **不要根据你自己的视口确定大小。** 在 WebView 中，`window.innerHeight` 就是宿主刚刚分配给你的插槽：这是循环依赖。如果你要调整自己的大小，请依据 `data.viewportHeight`（面板）或 `data.suggestedHeight`，永远不要依据 `innerHeight`。
- **没有焦点，没有指针。** 小部件永远不会获得方向键焦点，也没有悬停。任何交互内容都应该放在网页和移动端界面上。
- **让文字远离边缘。** 电视会有过扫描：在每一边都留出大约 5% 的宽度和高度。
- **在真实设备上放大测试 WebGL。** 着色器运行在设备的 GPU 和驱动上，而不是你电脑的。一个在 `fract()` 之前把输入乘以几百的格点哈希，在浏览器中看起来正常，在 Mali GPU 上却画出了平面多边形而不是云；请让噪声坐标保持有界（`mod()`），并对有界的值做哈希。`tests/plugins/weather-v2` 是上述所有要点的参考小部件。

## 插槽

| 插槽 | 出现的位置 |
|---|---|
| `home.section` | 首页上的内容区块 |
| `home.hero` | 首页上的主横幅区域 |
| `library.sidebar` | 媒体库视图中的侧边栏面板 |
| `media.detail.sidebar` | 媒体详情页上的侧边栏面板 |
| `player.overlay` | 视频播放器上的叠加层 |
| `settings.user.appearance` | 用户设置中的外观子区块 |
| `sidebar.bottom` | 主导航侧边栏的底部区域 |
| `admin.nav` | 管理面板中的一个导航项，打开插件自己的沙箱化页面 |

`admin.nav` 是**唯一**的管理区插槽。它不会向现有的管理界面注入任何东西：它添加一个菜单项，在一个专用的管理路由上用沙箱化的 iframe 打开你的插件自己的页面。请求它会出现在安装同意的差异中。

`admin.*`、`login.*`、`setup.*`、`profile.security.*` 下的所有其他插槽在安装时都会被拒绝。

## 能力

能力声明插件承担什么服务器端角色。它们影响宿主把哪些 RPC 转给插件，以及如何在服务注册表中登记插件。

### `metadata.provider`

插件可以为媒体条目提供丰富的元数据。没有专门的元数据增强 RPC：宿主调用的 `PluginService` 只暴露 `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities`（`proto/plugin/v1/plugin_api.proto`）。`metadata.provider` 插件通过 `OnEvent` 接收相关事件，并通过 `GetCapabilities` 报告其能力；它不实现单独的增强方法。

所需权限：`api.metadata`。

### `media.read`

插件可以读取媒体记录和流。对应清单中的 `media.read` 权限。

### `media.write`

插件可以创建或更新媒体记录。对应 `media.write` 权限。

### `notification.sender`

插件可以发布通知。宿主通过插件转发外发的通知事件。对应 `notification.sender` 权限。

### `webhook.receiver`

插件注册为一个 Webhook 端点。宿主通过 gRPC 把匹配的入站 HTTP POST 请求转发给插件。除了能力声明之外，不需要额外的权限。

## 权限

`public` 和 `prefs` 是**权限**，而不是能力：它们不改变插件承担的服务器端角色，而是授予插件的 iframe 额外的读写范围。在清单的 `permissions` 下声明它们。

### `public`

授予插件的 iframe 对非敏感服务器数据的只读访问：`GetPublicMediaItem` / `ListPublicMediaItems`，它们返回一个**安全投影**（id、标题、类型、年份、简介、海报 URL），永远不会包含文件路径、大小、提供商 id 或流。服务器标识（名称）本来就是公开的，不需要权限。在 `permissions` 中声明 `public` 来请求它。

### `prefs`

授予插件的 iframe 通过 `PluginUserDataService`（`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`）读写**当前用户**在插件自己命名空间中的偏好。插件和用户由 iframe 令牌绑定，所以插件只能访问（它自己的插件、当前用户）的数据：永远无法访问另一个插件的、另一个用户的，或者身份/安全相关的字段。这些记录会随插件或用户一起级联删除。在 `permissions` 中声明 `prefs`。

> **永远无法请求的东西。** `admin.*`、`auth.*`（登录/登出/会话）、`system.*`，以及通用的 `user.write` / `library.write` 都被硬性锁定：即使清单试图声明它们，它们也会从任何令牌中被去掉，并在 API 边界被拒绝。范围受限的插件令牌在服务器 API 上**默认拒绝**：它只能访问上面授予的权限明确解锁的方法。

## 示例

`examples/plugins/` 包含采用当前清单格式的可用插件：Node.js 和 Python 的起步模板（`nodejs-example`、`python-example`）、一个元数据提供者（`metadata-example`）、一个 Webhook 接收器（`webhook-receiver`）、一个首页小部件（`weather`）、插件配置和机密（`config-demo`）、使用 `public` / `prefs` 权限和 `admin.nav` 插槽的数据存储（`data-demo`），以及两个主题。[dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) 介绍如何复制其中一个。
