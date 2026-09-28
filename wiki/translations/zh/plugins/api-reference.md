---
source_sha: 2d10a959b8d809ae197a485f1214dbb6d86d37c8658c1c4d4c12d0222f6593f8
---
# 扩展 API 参考

本文档面向编写 Evey 扩展的外部开发者。扩展所受的安全约束见 [`security-model.md`](security-model.md)。

## 清单 schema

每个扩展 ZIP 的根目录都必须包含一个 `plugin.yaml`。

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # required for plugins; the full command line the interpreter runs -
                              # the first token is the interpreter, looked up on PATH; the
                              # remaining tokens are the script/args, path-confined to the ZIP.
                              # "language" is only used to check the runtime is available and to
                              # copy SDK stubs - it does NOT launch the entrypoint.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - api.example.com          # bare hostname only - no scheme, no wildcard, no literal IP

capabilities:
  - metadata.provider        # see Capabilities section

# Optional: declare a sub-directory whose contents are served as static assets.
# Required when frontend modules are declared.
web_root: public/

# Optional: frontend modules injected into the host UI.
frontend:
  - slot: home.section       # see Slot whitelist in security-model.md
    src: public/front.js     # path relative to ZIP root; must be under web_root

config_schema:               # optional; JSON Schema object describing plugin config keys
  api_key:
    type: string
    secret: true             # or `type: password`: never sent back to any client, admin included
    description: "TMDB API key"
```

标记为 `secret: true`（或 `type: password`）的字段会照常保存并交给扩展，但所有读取接口 -
无论是公开接口还是管理后台的配置面板 - 返回的都是掩码。当掩码原样提交回来时，管理面板会保留
已保存的值，所以保存其他字段永远不会抹掉一个密钥。

**安装时执行的校验规则：**

- `kind: plugin` 需要 `language` 和 `entrypoint`。
- `kind: theme` 不得声明 `language`、`entrypoint` 或 `permissions`。
- `frontend` 非空时必须提供 `web_root`。
- `frontend[].src` 必须位于 `web_root` 之下。
- `frontend[].slot` 必须在服务器端的插槽白名单中。
- `network` 权限要求 `external_fetch` 列表非空。
- 所有 `external_fetch` 条目都必须通过主机白名单校验。
- 所有能力和权限都必须是服务器已知的。

## 后端 IPC

扩展进程通过 Unix 域套接字上的 gRPC 与 Evey 宿主通信。套接字路径在启动时通过环境变量 `EVEY_PLUGIN_SOCK` 传入。

宿主在 `cmd.Start` 之后立即通过 `SO_PEERCRED` 校验扩展进程的 PID 来完成认证 - 不会给进程本身签发任何令牌。

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

另有一个独立的短期 JWT（5 分钟，限定于 `plugin_id` + `user_id`），通过 `IssuePluginToken` RPC 按需签发 - 扩展的*前端 iframe* 获取它来认证自己在 `public`/`prefs` 范围内的调用。它与上面的后端套接字毫无关系，启动时也从不交给扩展进程。

gRPC 服务定义位于 `proto/plugin/v1/plugin_api.proto`。各语言的 SDK 桩代码由这个 proto 生成，可以放进扩展 ZIP，也可以随附提供。有几种语言的桩代码已预先生成（`sdk/<lang>/`）；若要面向未内置的语言，请参见 [dev-setup.md → 为其他语言生成桩代码](dev-setup.md#generating-stubs-for-another-language)。

宿主在 RPC 层面执行权限检查 - 调用你的清单未声明的 RPC，结果是 `PermissionDenied`。

### 数据存储

每个扩展都有自己持久化的键值存储，位于扩展自己的命名空间 - **不需要任何权限，扩展拥有自己的
数据**。可以用它保存游标、缓存、上次同步时间，以及扩展需要在重启之间记住的任何东西。值是不透明
的字符串（需要结构时请自行编码为 JSON）。

| `HostService` RPC | 用途 |
|---|---|
| `SetData(key, value)` | 写入或覆盖一个值。 |
| `GetData(key) → {value, found}` | 读取一个值。 |
| `DeleteData(key)` | 删除一个键。 |
| `ListDataKeys() → {keys}` | 列出扩展的所有键。 |

扩展 id 取自已认证的套接字 - 扩展只能读写**自己的**命名空间，永远碰不到别的扩展。扩展卸载时，
它的整个存储会被自动清除（`ON DELETE CASCADE`），因此卸载后不会残留任何过期数据。

这个存储是扩展级别的全局存储。若要**按用户**保存数据（例如当前观看用户的某项偏好），请看下面的
`prefs` 权限 - 那个接口要从扩展的 iframe 访问，那里才有用户上下文。

## 前端 API

当扩展声明了 `frontend` 模块时，iframe `srcdoc` 的 HTML 中会包含一个桥接脚本，把 `window.evey` 暴露给模块的 JavaScript。

前端模块无法直接访问宿主页面的 DOM、`window`、`fetch`、`localStorage` 或任何其他浏览器全局对象，只能使用通过 `window.evey` 明确提供的内容。

### `evey.navigate(path)`

让宿主应用跳转到 `path`。路径必须是白名单内的路由；对 `/admin/*`、`/auth/*`、`/setup/*` 和 `/profile/security/*` 的请求会被静默丢弃。

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

在宿主界面中显示一条 toast 通知。

- `level`：`"success"` | `"error"` | `"info"`
- `message`：纯文本，最多 200 个字符，HTML 会被去掉
- 每秒最多 3 条

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

请求宿主把 iframe 插槽调整为 `height` 像素。限制在 50-800 之间。`0` 会把插槽收起：没有内容可显示的扩展不会在页面上占位置。

```js
evey.resize(300);
evey.resize(0); // nothing to say: the slot disappears
```

### `evey.onInit(callback)`

注册一个回调，在宿主把初始化上下文发给扩展时触发一次。如果在调用 `onInit` 之前宿主已经发送了 `init`，回调会立即用缓存的数据执行。

```js
evey.onInit(function(data) {
  // data.context         - plugin config values (from config_schema)
  // data.locale          - active locale string, e.g. "en-US"
  // data.theme           - active theme id
  // data.surface         - "web", "mobile" or "tv": where the widget landed
  // data.suggestedHeight - home.section only: the height (px) the host recommends there
  // data.viewportHeight  - tv only: the panel height in dp
});
```

宿主最多等待 5 秒，等扩展发出 `ready` 信号（桥接脚本会自动完成）。超时后 iframe 会被卸载，并报告一个错误。

#### 高度

`home.section` 小组件无需任何操作就会以 `data.suggestedHeight` 打开：宿主知道把你放在了哪个界面上 - 桌面页面、手机、电视面板 - 并推荐那里主页小组件应有的高度，让主页上的每个小组件大小一致。只有在你更清楚时才调用 `evey.resize()`；宿主从不在背后改变或缩放你的内容。

#### 在电视上

电视应用在 WebView 中运行你的小组件，浏览器里成立的一些事在那里就不成立了：

- **不要根据自己的视口来定尺寸。** 在 WebView 里，`window.innerHeight` 就是宿主刚分给你的插槽 - 这是循环论证。如果要自己调整尺寸，请依据 `data.viewportHeight`（面板高度）或 `data.suggestedHeight`，永远不要依据 `innerHeight`。
- **没有焦点，没有指针。** 小组件永远拿不到方向键的焦点，也没有悬停。所有可交互的内容都属于网页和手机界面。
- **文字远离边缘。** 电视会过扫描：每一边都留出大约 5% 的宽度和高度。
- **在真机上、放大后测试 WebGL。** 着色器运行在机顶盒的 GPU 和驱动上，而不是你桌面电脑的。一个在 `fract()` 之前把输入乘以几百的格点哈希，在浏览器里看着没问题，在 Mali GPU 上却画出了平面多边形而不是云；请把噪声坐标限定在有界范围内（`mod()`），并只对有界的值做哈希。`tests/plugins/weather-v2` 是上述所有要点的参考小组件。

## 插槽

| 插槽 | 出现位置 |
|---|---|
| `home.section` | 主页上的内容区块 |
| `home.hero` | 主页上的主横幅区域 |
| `library.sidebar` | 媒体库视图中的侧边面板 |
| `media.detail.sidebar` | 媒体详情页上的侧边面板 |
| `player.overlay` | 视频播放器上的覆盖层 |
| `settings.user.appearance` | 用户设置中的外观子区块 |
| `sidebar.bottom` | 主导航侧边栏的底部区域 |
| `admin.nav` | 管理面板中的一个菜单项，打开扩展自己的隔离页面 |

`admin.nav` 是管理区**唯一**的插槽。它不会往现有的管理页面里注入界面 - 只是添加一个菜单项，
在一个专用的管理路由上、用隔离的 iframe 打开你扩展自己的页面。申请它会出现在安装时的授权差异里。

`admin.*`、`login.*`、`setup.*`、`profile.security.*` 下的其他所有插槽都会在安装时被拒绝。

## 能力

能力声明扩展在服务器端承担什么角色。它们决定宿主把哪些 RPC 路由给扩展，以及如何在服务注册表中登记扩展。

### `metadata.provider`

扩展可以为媒体条目提供增强的元数据。没有专门的增强 RPC - 宿主调用的 `PluginService` 只暴露 `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities`（`proto/plugin/v1/plugin_api.proto`）。`metadata.provider` 扩展通过 `OnEvent` 接收相关事件，并通过 `GetCapabilities` 报告自己的能力；它不需要实现单独的增强方法。

所需权限：`api.metadata`。

### `media.read`

扩展可以读取媒体记录和流。对应清单中的 `media.read` 权限。

### `media.write`

扩展可以创建或更新媒体记录。对应 `media.write` 权限。

### `notification.sender`

扩展可以发布通知。宿主会把外发的通知事件经由扩展发送。对应 `notification.sender` 权限。

### `webhook.receiver`

扩展注册为一个 webhook 端点。宿主通过 gRPC 把匹配的入站 HTTP POST 请求转发给扩展。除了声明该能力外，不需要额外权限。

## 权限

`public` 和 `prefs` 是**权限**，不是能力 - 它们不改变扩展在服务器端承担的角色，而是给扩展的
iframe 额外的读写范围。请在清单的 `permissions` 下声明它们。

### `public`

授予扩展 iframe 对非敏感服务器数据的只读访问：`GetPublicMediaItem` / `ListPublicMediaItems`，
它们返回一个**安全投影**（id、标题、类型、年份、简介、海报 URL）- 永远不包括文件路径、大小、
提供方 id 或流。服务器的品牌信息（名称）本来就是公开的，不需要权限。在 `permissions` 中声明
`public` 即可申请。

### `prefs`

授予扩展 iframe 在扩展自己的命名空间内读写**当前用户**偏好的权限，通过 `PluginUserDataService`
（`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`）。扩展和用户由 iframe 令牌绑定，所以扩展
只能触碰（它自己这个扩展、当前这个用户）的数据 - 永远碰不到其他扩展的、其他用户的，也碰不到身份
或安全字段。这些记录会随扩展或用户的删除而级联删除。在 `permissions` 中声明 `prefs`。

> **你永远无法申请的。** `admin.*`、`auth.*`（登录/登出/会话）、`system.*`，以及通用的
> `user.write` / `library.write` 都被硬性锁死：它们会从任何令牌中被剥离，即使清单试图声明，也会
> 在 API 边界被拒绝。带范围的扩展令牌在服务器 API 上是**默认拒绝**的 - 它只能访问上面授予的权限
> 明确开放的方法。

## 示例

`examples/plugins/` 中是采用当前清单格式、可以运行的扩展：Node.js 和 Python 的起步模板（`nodejs-example`、`python-example`）、一个元数据来源（`metadata-example`）、一个 webhook 接收器（`webhook-receiver`）、一个主页小组件（`weather`）、扩展的配置与密钥（`config-demo`）、带 `public` / `prefs` 权限和 `admin.nav` 插槽的数据存储（`data-demo`），以及两个主题。[dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) 介绍了如何复制其中一个。
