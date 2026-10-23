---
source_sha: 7be5d2c69f31e5a809a830f40949f2c4242950d173dfc5bf681d26c5be44e87f
reviewed: true
---
# 权限与插槽 { #permissions-and-slots }

写给插件作者：清单里可以声明什么，安装程序会检查什么，以及插件运行时要遵守哪些规则。
下面的目录列出了可供选择的项；服务器如何隔离插件、这种隔离无法阻止什么，请看
[插件沙箱与威胁模型](../SECURITY.md)。

## 安装时检查什么 { #what-is-checked-at-install }

出现以下情况时，插件会在安装时被拒绝：

- 缺少 `kind`，或者它既不是 `plugin` 也不是 `theme`；
- `kind: plugin` 缺少 `language` 或 `entrypoint`；或者 `kind: theme` 声明了它们，或声明了 `permissions` 或 `capabilities`；
- 某个权限或能力是未知的；
- 某个前端模块指向了[插槽列表](#slot-whitelist)之外的插槽；
- 某条 `external_fetch` 条目违反了[外部请求规则](#external-fetch-rules)，或者声明了 `network` 却一个条目都没有；
- ZIP 包违反了[解压限制](../SECURITY.md#zip-extraction)；
- 静态分析发现了严重的模式，或者安装程序按严重处理的警告（不安全的反序列化、代码混淆）。
  编译好的二进制文件、内嵌的压缩包和原生代码都会被拒绝，除非清单声明了 `native-binary`。

分析之前会先去掉注释，所以被注释掉的行不会被报告。
各个类别列在[插件沙箱与威胁模型](../SECURITY.md#static-analysis)里。

安装之前，管理员会看到插件声明的权限和能力，以及分析发现了什么；插槽、主机和签名会在更新时以及安全仪表板里显示。插件声明的任何内容都不会在管理员不知情的情况下被授予。

## 插件在什么条件下运行 { #what-the-plugin-runs-under }

无论清单里声明了什么：

- 插件只看得到它自己的目录，看不到服务器的数据；
- 它没有直接的网络访问：带 `network` 权限的插件，只能通过宿主的 `Fetch` 调用访问它 `external_fetch` 列表里的主机，不能访问其他任何主机；
- 没有 `subprocess` 权限时，它不能启动子进程（线程可以正常使用）；
- 它在资源限制下运行，管理员可以调整这些限制：默认 256 MiB 内存、64 个进程和线程、它的目录 100 MiB 磁盘空间，以及较低的 CPU 优先级。

开发期间签名是可选的。怎样给插件签名，以及服务器可以强制执行的签名策略，请看[编写插件](dev-setup.md#7-sign-your-plugin)。

## 权限目录 { #permissions-catalog }

权限写在 `permissions` 下；任何不在表里的值都会导致安装被拒绝。接收 Webhook（`webhook.receiver`）和提供元数据
（`metadata.provider`）是能力，写在 `capabilities` 下：请看[插件 API 参考](api-reference.md#capabilities)。

| 权限 | 授予什么 |
|---|---|
| `network` | 后端可以调用 `Fetch`。必须同时提供一个非空的 `external_fetch` 列表；没有的话安装会被拒绝。进程本身仍然没有任何套接字。 |
| `subprocess` | 后端可以创建子进程。没有这个权限时，启动器的内核过滤器会拒绝 `fork`、`vfork` 以及不带 `CLONE_THREAD` 的 `clone`；对于解释型插件，seccomp 垫片还会拒绝 `execve`。无论哪种情况，线程都能正常工作。管理员会看到一条专门的警告。 |
| `native-binary` | 包里可以带 `.so`、`.dll`、`.exe`、`.wasm`、`.jar`、内嵌的压缩包，或分析无法读取的原生代码。管理员会看到这些不透明的代码，并且必须接受。`go` 插件必须声明它。 |
| `filesystem` | 声明有意访问插件目录之外的文件系统。它会被记录下来并展示给管理员；但无论声明了什么，沙箱都不会给予插件目录之外的任何文件访问。 |
| `media.read` | 宿主上的 `GetMediaItem` 和 `ListMediaItems`：完整的媒体记录。 |
| `media.write` | 为写入元数据的调用预留，目前还没有提供。 |
| `api.library` | 和 `media.read` 相同的媒体读取调用。 |
| `api.user` | 为 `GetUserPreferences` 预留，目前还没有提供。 |
| `api.metadata` | 为 `UpdateMetadata` 预留，目前还没有提供。 |
| `notification.sender` | 为 `SendNotification` 预留，目前还没有提供。 |
| `public` | 插件界面可以读取媒体条目的一个安全投影（id、标题、类型、年份、简介、海报），永远读不到路径或流。请看 [`evey.media.*`](api-reference.md#eveymedia-permission-public)。 |
| `prefs` | 插件界面可以在插件自己的命名空间里读写当前用户的偏好。请看 [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs)。 |
| `hardware.gpu` | 进程会拿到宿主的 DRM 渲染节点（真实的 `renderD1xx`，绝不会是 card 节点）。这会暴露内核驱动的攻击面，而且显存的边界和转码器共用：GPU 一旦挂起，转码也可能停滞。没有这个权限时，GPU 节点完全不在 Landlock 允许列表里；有了它，启动器会添加一条只针对这一个文件的 `IOCTL_DEV` 规则（需要 Landlock ABI 5 或更高；在更旧的内核上，仅有读写权限就足以提交命令）。插件安装后，安全仪表板会显示一条严重警告。只把它授予那些您愿意在沙箱外运行的插件。 |

## 外部请求规则 { #external-fetch-rules }

写在 `external_fetch: [...]` 里，安装时检查（`validateExternalFetch`）。要么全部通过，要么全部不通过：只要有一条无效，整个安装就会被拒绝。

每一条是一个**主机名**。协议前缀和末尾的路径在解析时会被丢弃，只保留主机名。主机必须完全匹配，`*.example.com` 这样的通配符在安装时会被拒绝，端口后缀会被忽略。

| 条目 | 是否有效 | 原因 |
|---|---|---|
| `api.tmdb.org` | 是 | 精确的主机名，标准写法 |
| `https://api.tmdb.org` | 是 | 协议和路径在解析时被丢弃，保留主机名 |
| `*.tmdb.org` | 否 | 通配符在安装时会被拒绝；请逐个列出主机 |
| `*` | 否 | 单独的通配符会被拒绝 |
| `192.168.1.1`、`::1` | 否 | 不接受 IP 地址，IPv4 和 IPv6 都不行 |

运行时，宿主的代理还会加上自己的限制，清单无法放宽这些限制：只允许 `http` 和 `https`，只允许 `GET` 和 `HEAD`；
目标主机要和您的列表匹配（不区分大小写，条目里的 `:port` 后缀会被忽略）；DNS 解析之后，私有或保留地址一律拒绝；
响应体最大 8 MiB，一次请求最长 20 秒。重定向会再次按列表和地址规则检查。

同一份主机列表，加上 `https://` 前缀，也就是插件框架的 `connect-src`：小组件可以直接调用这些主机，但只能走 `https`。

## 插槽白名单 { #slot-whitelist }

插件只能在明确允许的插槽里注入界面。关键的路径有意不在其中。

| 插槽 | 需要登录 | 说明 |
|---|---|---|
| `home.section` | 是 | 首页的主要内容区 |
| `home.hero` | 是 | 首页顶部的大横幅区 |
| `library.sidebar` | 是 | 媒体库视图的侧边栏 |
| `media.detail.sidebar` | 是 | 媒体详情页的侧边栏 |
| `player.overlay` | 是 | 播放器上的浮层界面 |
| `settings.user.appearance` | 是 | 用户设置里的外观部分 |
| `sidebar.bottom` | 是 | 导航栏的底部区域 |
| `admin.nav` | 是 | 管理面板里的一个导航入口，打开插件自己的沙箱页面；它不会注入到已有的管理界面里 |

**永远不允许：** `admin.*`（`admin.nav` 除外）、`login.*`、`setup.*`、`profile.security.*`。
这些路径事关身份验证或权限，在那里注入内容可能被用来提升权限或钓鱼。

`admin.nav` 的内容和其他插槽一样在沙箱中运行：它在一个不带 `allow-same-origin` 的 `<iframe sandbox="allow-scripts">` 里渲染，
所在页面的策略是 `default-src 'none'`。所以它只是显示在管理区域里的第三方内容，并不是授予插件的管理能力。

桥接层限制了框架能向页面提出的请求：`evey.toast` 每秒最多 3 条消息，每条 200 个字符，不能含 HTML；
`evey.navigate` 只能导航到 `/home`、`/library`、`/media`、`/player`、`/settings` 和 `/search`；`evey.resize` 被限制在 50 到 800 px 之间。
框架本身不持有令牌：宿主为 `public` 和 `prefs` 调用签发一个限定于（插件，用户）、有效期五分钟的令牌，
而服务器默认拒绝这个令牌没有列出的所有方法。

## 更新流程 { #update-workflow }

1. 管理员上传新的 ZIP 包。`PreviewPluginUpdate` 会运行静态分析和签名检查，并返回与已安装版本相比，
   权限、能力、插槽和 `external_fetch` 有哪些不同。
2. 如果有不同，管理员必须先确认，更新才会应用（`requires_ack = true`；通过上传路由时为 `accept_permission_changes=true`）。
3. 包的哈希值在预览时记下，在应用时核对：被分析的 ZIP，就是被安装的那个 ZIP。
4. 签名密钥要检查连续性：用与已安装版本不同的密钥签名、或不再签名的更新会被拒绝，直到管理员接受新的发布者。在 `EVEY_PLUGINS_COSIGN_POLICY=enforce` 下，即使预览通过了，只要应用这一步的签名不受信任或无效，更新就会被阻止。

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
comments stripped before analysis: docs/SECURITY.md (Static analysis) ; internal/adapters/driven/pluginrunner/analyzer_normalize.go
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
