---
source_sha: c5a6e328a581a7ee91f06bf5ab358435d8699a0c280a7161b22a0879f70c4d5a
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
language: nodejs             # 插件必填；决定复制哪些存根
entrypoint: node index.js    # 插件必填；服务器执行的命令行

display_name: My Plugin      # 可选；一个字符串，或一个映射 {en: ..., fr: ...}
description: "..."           # 可选
author: "..."                # 可选
min_evey_version: "1.0.0"    # 可选；高于服务器版本时，拒绝安装或更新

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
| `display_name` | 给人看的名称；`name` 仍是更新时用来对应插件的标识。只写一个字符串时算作英文，适用于所有语言；映射以语言代码为键（`en`、`fr`、`pt-BR`）。每个值 1 到 40 个字符，前后不能有空格，也不能含控制字符。目前由主题选择器显示。 |
| `language` | `kind: plugin` 必填，可以是任意值。它决定把哪些存根复制到入口旁边（即 `sdk/<language>/` 里的）。对于通过 shebang 启动的脚本（`entrypoint: main.py`），`python`、`nodejs`、`php`、`ruby`、`lua`、`bash` 和 `sh` 还要求镜像里有对应的解释器。它本身不启动任何东西。 |
| `entrypoint` | `kind: plugin` 必填。第一个词是插件包里的文件（二进制文件，声明了 `native-binary` 时；或脚本），或者是镜像里能在其 `PATH` 中找到的命令（`node`、`python3`、`java`、`dotnet`），其余是它的参数；命令找不到会让安装被拒绝。进程在插件的目录里运行。 |
| `permissions` | 每个值都必须在[权限目录](security-model.md#permissions-catalog)里。`network` 要求 `external_fetch` 不为空。 |
| `external_fetch` | 主机名。协议前缀和末尾的路径在解析时会被丢弃；主机必须完全匹配，`*.example.com` 这样的通配符在安装时会被拒绝；不带端口的条目只允许 80 和 443 端口，`host:port` 只允许该端口；IP 地址会导致安装被拒绝（[规则](security-model.md#external-fetch-rules)）。 |
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
| `OnEvent(event_type, payload, source)` | 仅 `metadata.request`，发给具有 `metadata.provider` 能力的插件（见下文[元数据请求](#metadata-requests)）。其他插件不会收到任何事件：回答 `handled: false`。 |
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
| `UpdateMetadata(media_item_id, fields, provider_ids)` | `api.metadata` 或 `media.write`，以及 `metadata.provider` 能力 | 主动为一部电影或剧集提供您的文本，与[元数据回答](#metadata-requests)相同。`fields` 包含 `locale`（必填）以及 `title`、`overview`、`tagline` 中的任意几项；该文本替换您为这种语言提供过的内容，只含 `locale` 的 `fields` 会将其删除。`provider_ids` 会被拒绝：作品由 TMDB 识别。一个插件能存储的内容有上限：每个作品 16 种语言，总共 200 000 条文本和 128 MiB。超出后调用返回 `RESOURCE_EXHAUSTED`；替换或删除您已经提供的文本仍然可以。 |
| `GetUserPreferences` | `api.user` | *未提供。* |
| `SendNotification` | `notification.sender` | *未提供。* |

### 元数据请求 { #metadata-requests }

每当服务器从 TMDB 识别或刷新一部电影或剧集时，它会向每个已启用的 `metadata.provider` 插件逐个发送 `event_type` 为 `metadata.request` 的 `OnEvent`，每个插件最多等待 15 秒。内容如下：

| 字段 | 内容 |
|---|---|
| `media_item_id` | 作品的 ID，供之后调用 `UpdateMetadata` 使用。 |
| `type` | `movie` 或 `series`。 |
| `title`、`original_title` | 服务器语言下的 TMDB 标题，以及原始标题。 |
| `year` | 未知时不出现。 |
| `provider_ids` | `tmdb`、`imdb` 以及该作品已知的其他 ID。 |
| `language` | TMDB 文本的语言，例如 `fr`。 |
| `locales` | 服务器显示的所有语言。 |

回答 `handled: true`，并附上如下形式的 `result`：

```json
{
  "locale": "fr",
  "title": "...",
  "overview": "...",
  "tagline": "...",
  "translations": {
    "de": { "overview": "..." }
  }
}
```

顶层字段使用 `locale` 指定的语言；没有 `locale` 时使用请求中的 `language`。`translations` 用于添加其他语言。您不覆盖的字段请省略。语言标签会被截为语言部分（`fr-FR` 视为 `fr`），超过 8 KiB 的字段会被忽略。一个回答会替换您之前为该作品提供的全部内容；`handled: false` 或不含文本的回答表示您不认识该作品，并会清除您为该作品提供的内容。出错或超时则保留您之前提供的内容。

管理员选择您的文本替换 `title`、`overview` 和 `tagline` 中的哪些字段，并可以一次性就整个媒体库向您查询（[元数据插件](../plugins.md#metadata-plugins)）。季和单集不会发送。

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

媒体条目的一个安全投影：id、标题、类型、年份、简介、海报 URL。永远不会有提供方 id 或流；除非插件还持有 `media.files`（见下文），也不会有文件路径和文件大小。

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });  // { items, total }
const data  = await evey.media.poster(id);  // 海报，以 data: URL 的形式
```

`list` 返回一页，最多 500 个条目，所有类型混在一起（剧集、季、单集……），除非用 `type` 加以限定。
需要整个媒体库的插件，应指定类型并逐页读取，直到拿到 `total` 个条目：

```js
const page = await evey.media.list({ type: "MEDIA_ITEM_TYPE_MOVIE", page: 2, pageSize: 500 });
```

有 `media.files` 权限，并且只有当正在查看的人是管理员时，每个条目才会额外带上 `filePath`、`fileSize`
（一个字符串，和 API 里所有 64 位数字一样）和 `quality`：`width`、`height`、`videoCodec`、`hdrType`（SDR 时为空）、
`bitDepth`、`bitrate`（每秒比特数）、`durationMs`、`container`、`audioCodec` 和 `audioChannels`，取自第一条视频轨和默认音轨。对其他任何人，令牌在签发时就不带这个权限，两个字段都是空的：
展示给所有人的模块不能依赖它们，它们该出现在 `admin.nav` 插槽里。

`poster` 返回 `data:` URL，是因为框架的策略禁止直接从服务器加载图片；可以把它用作 `<img>` 的来源，或者当作纹理。

### `evey.media.gaps()`（权限 `media.gaps`） { #eveymediagaps-permission-mediagaps }

媒体库缺了什么，由 Evey 计算：已有剧集中已播出但没有的单集，以及已开始收藏的系列电影中已上映但没有的影片。
一部剧或一个系列包含什么，由 Evey 自己按节奏向 TMDB 查询（保存的结果一周后更新）；插件从不调用 TMDB，只读取结果。

```js
const { series, sagas } = await evey.media.gaps();
// series: [{ seriesId, title, year, fromCatalog, seasons: [{ season, whole,
//   episodes: [{ episode, title, airDate }] }] }]
// sagas: [{ collectionId, name, owned, missing: [{ title, releaseDate, posterUrl }] }]
```

只计算已经发布的内容：没有日期或日期晚于今天的单集或电影还不算缺失，特别篇（第 0 季）不计入。
TMDB 不认识的剧集按其自身编号中的空缺列出（`fromCatalog` 为 false）：低于已有最高集号的缺失单集、低于已有最高季号的缺失整季（`whole`，不带单集列表）。
仅限管理员会话，与 `media.files` 相同。

### `evey.media.trash(ids)`（权限 `media.trash`，仅限插槽 `admin.nav`） { #eveymediatrashids }

请求 Evey 把媒体文件移到其媒体库的回收站。插件只是请求：Evey 会显示自己的确认窗口，内容来自 Evey 自己对每个条目的记录
（标题、年份、路径、大小、画质），绝不采用插件发来的内容；管理员在那里点确认之前，什么都不会移动。

```js
const res = await evey.media.trash([id1, id2]);
// { trashed: [{ id, trashedPath }], failed: [{ id, message }] }
```

确认后，每个文件连同字幕和 `.nfo` 会移到其媒体库根目录下的 `.evey-trash`，保持相同的相对路径，该条目随之从 Evey 中移除。
磁盘上不会删除任何内容：由管理员清空该文件夹，移回原处的文件会在下次扫描时回来。只适用于电影或单集：剧集是一个文件夹。
管理员拒绝时 Promise 以 `cancelled` 拒绝；在浏览器中，除了拥有 `media.trash` 的插件的 `admin.nav` 页面之外，
任何地方都会以 `method not permitted` 拒绝（手机和电视应用暂不提供）。单个文件被拒（只读挂载、文件已不在）会出现在 `failed` 中。

`evey.media.restore(paths)` 按同样的规则撤销：根据 `trashedPath` 值，Evey 把每个文件连同字幕移回原处并扫描媒体库，
标题随之回来，作为按文件名重新识别的新条目（旧条目的播放进度不会恢复）。结果为
`{ restored: [{ trashedPath, restoredPath }], failed: [...] }`；Evey 拒绝媒体库回收站之外的任何路径，以及已被重新占用的位置。
无需确认：它不会销毁任何内容。

### `evey.assets.read(path)` { #eveyassetsreadpath }

按相对路径读取插件自己 `web_root` 里的一个文件，返回 `data:` URL：后端在运行时写出的文件，
就能在框架里直接使用，不用操心跨域问题。只能读到插件自己的资源。

### `evey.config.save(values)`（仅限插槽 `admin.plugin.config`） { #eveyconfigsavevalues }

写入插件的服务器配置，也就是管理员按 `config_schema` 设置的值。给出的键会合并进已保存的配置，
其余的键保持原样，服务器会按模式校验合并后的结果，和校验生成的表单完全一样。值都是字符串，和表单里一样。
Promise 会以 `{ values }` 兑现，即实际保存的配置（密钥字段会被遮蔽）；某个值被拒绝时，
它会带着服务器的消息被拒绝。在 `admin.plugin.config` 之外的任何地方，这个调用都会被拒绝。

```js
await evey.config.save({ city: "Brest", lat: "48.39", lon: "-4.49" });
```

当前的值会在 `onInit` 里以 `config` 的形式传入，和每个插槽一样。

### 在电视上 { #on-a-television }

电视应用在 WebView 里运行模块，一些在浏览器里成立的事情，在那里就不成立了：

- **不要按自己的视口来定尺寸。** 在 WebView 里，`window.innerHeight` 就是宿主刚刚分给您的插槽高度。
  请按 `data.viewportHeight` 或 `data.suggestedHeight` 来定尺寸。
- **没有焦点，也没有指针。** 小组件永远不会获得方向键的焦点，也没有悬停效果；任何需要交互的东西都应放在网页和移动端。
- **文字不要贴着边缘。** 电视会过扫描：每一边都留出大约 5% 的宽度和高度。
- **在真实的电视盒上测试 WebGL。** 着色器运行在电视盒自己的 GPU 和驱动上，而不是您电脑上的；噪声坐标要保持在有限范围内。
  [eveyproject/evey-plugin-weather](https://github.com/eveyproject/evey-plugin-weather) 是以上所有要点的参考小组件。

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
| `admin.plugin.config` | 插件自己的配置面板，位于管理员插件界面中它自己的卡片里 |

`admin.nav` 和 `admin.plugin.config` 是管理区域里仅有的两个插槽。两者都不会往已有的管理界面里注入任何东西：
`admin.nav` 添加一个菜单入口，在同样的沙箱框架里、在一条仅限管理员的路由上打开插件的页面；
`admin.plugin.config` 取代由 `config_schema` 生成的表单，显示在插件自己的卡片里，用于普通字段不太好选的值（比如在重名城市中选一个）。
申请其中任何一个都会显示在需要管理员同意的差异里。
其他所有 `admin.*`、`login.*`、`setup.*` 和 `profile.security.*` 插槽，安装时都会被拒绝。

## 能力 { #capabilities }

能力声明的是插件自称承担的角色：`metadata.provider`、`media.read`、`media.write`、`webhook.receiver` 或 `notification.sender`。
安装程序会检查名称，管理员会在预览里看到这份列表，以及更新时新增的项，**安全仪表板**也会记录它。
服务器会根据其中一种能力转发：`metadata.provider` 会收到[元数据请求](#metadata-requests)，并可以调用 `UpdateMetadata`。其他能力目前不转发任何东西；`webhook.receiver` 收不到任何传入的请求。
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
一个元数据提供方（`metadata-example`）、一个 Webhook 接收端（`webhook-receiver`）、
一个演示数据存储、`public` 和 `prefs` 权限以及 `admin.nav` 插槽的示例（`data-demo`），以及一个展示所有 `config_schema` 字段类型的示例，带有一个 `admin.nav` 页面（`config-demo`）。
`make plugin-zips` 会把它们全部打包。项目自己的插件和主题（天气小组件、CShell、Dark Blue 等）各自位于 [eveyproject](https://github.com/eveyproject) 下的独立仓库中。

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - CheckEntrypoint ; launchers: runner_start.go - resolveCommand ; launcher_resolve.go - lookupLauncher
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
