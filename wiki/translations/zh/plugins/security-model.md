---
source_sha: f4ecae392bc0b50e2c1ef8c70f50db7121732b5c6e7c194891cf416bffb92680
reviewed: false
---
# 权限与插槽

本页面向插件作者：清单可以声明什么、安装插件时检查什么，以及运行时适用于它的规则。服务器如何限制插件，以及这种限制无法阻止什么，在[安全模型与插件沙箱](../SECURITY.md)中说明。

## 安装时检查什么

出现以下情况时，插件会在安装时被拒绝：

- 缺少 `kind`，或者它既不是 `plugin` 也不是 `theme`；
- `kind: plugin` 缺少 `language` 或 `entrypoint`，或者 `kind: theme` 声明了它们或声明了 `permissions`；
- 某个权限或能力是未知的；
- 某个前端模块指向[插槽列表](#slot-whitelist)之外的插槽；
- 某个 `external_fetch` 条目违反了[外部访问规则](#external-fetch-rules)；
- ZIP 超出解压限制：超过 5,000 个文件、单个文件超过 100 MB、解压后超过 500 MB、压缩比超过 100:1、路径超出压缩包，或者包含符号链接；
- 静态分析发现严重模式（shell 执行、混淆、动态导入、沙箱逃逸、权限变更、挖矿……）。编译后的程序和内嵌的压缩包会被拒绝，除非清单声明了 `native-binary`。

分析也会读取注释：注释中的一行 `# pip install foo` 会像真实代码一样被报告。发布前请删除这样的行。完整的类别列表见[安全模型与插件沙箱](../SECURITY.md#static-analysis)。

安装之前，管理员会看到声明的权限、插槽和主机，以及分析的发现。

## 插件在什么条件下运行

无论清单声明了什么：

- 插件只能看到自己的目录，看不到服务器的数据；
- 它没有直接的网络访问：拥有 `network` 权限的插件通过宿主的 `Fetch` 调用访问其 `external_fetch` 列表中的主机；
- 没有 `subprocess` 时它不能启动子进程（线程可以工作）；
- 它在管理员可以调整的资源限制下运行：默认 256 MiB 内存、64 个进程和线程、其目录 100 MiB 磁盘空间，以及较低的 CPU 优先级。

开发期间签名是可选的。如何为插件签名，以及服务器可以强制执行什么签名策略：[编写插件](dev-setup.md)。

## 权限目录

权限在 `permissions` 下声明；任何其他值都会导致安装被拒绝。接收 Webhook（`webhook.receiver`）和提供元数据（`metadata.provider`）是能力，在 `capabilities` 下声明：见[插件 API 参考](api-reference.md#capabilities)。

| 权限 | 授予的内容 |
|---|---|
| `network` | 插件可以发起出站网络连接。**必须**与一个非空的 `external_fetch` 列表一起使用：没有它安装会被拒绝。 |
| `subprocess` | 插件可以启动子进程。没有它时有两层限制：LD_PRELOAD seccomp 垫片为解释型插件阻止 `execve`/`execveat`，启动器的 `-deny-fork` 在内核中阻止创建进程本身（不带 `CLONE_THREAD` 的 `fork`/`vfork`/`clone`）。两种情况下线程都能工作。（启动器的内核 seccomp 不拒绝 `execve`，因为它必须执行插件，但任何子进程都会继承内核过滤器 + Landlock 域。） |
| `native-binary` | 插件可以附带 `.so`/`.dll`/`.exe`/`.wasm`/`.jar` 或内嵌的压缩包。管理员会在安装预览中看到这些不透明的代码，并且必须接受它们。 |
| `filesystem` | 声明的访问插件自身目录之外文件系统的意图。会被记录并展示给管理员；无论声明什么，沙箱都不会授予插件目录之外的文件访问。 |
| `media.read` | 插件可以调用 `MediaRead` RPC：读取媒体文件的元数据和流访问。 |
| `media.write` | 插件可以调用 `MediaWrite` RPC：创建和更新媒体记录。 |
| `api.library` | 插件可以调用媒体库管理 RPC：列出和搜索媒体库、触发扫描。 |
| `api.user` | 插件可以调用用户范围的 RPC（读取自己的个人资料、偏好）。 |
| `api.metadata` | 插件可以调用元数据 RPC：搜索和写入增强数据。 |
| `notification.sender` | 插件可以发布通知事件（网页推送、电子邮件、Webhook）。 |
| `public` | 插件的界面可以读取媒体的安全投影（id、标题、类型、年份、简介、海报），永远不包含路径或流。见[插件 API 参考](api-reference.md#public)。 |
| `prefs` | 插件的界面可以在插件自己的命名空间中读写当前用户的偏好。见[插件 API 参考](api-reference.md#prefs)。 |
| `hardware.gpu` | **风险最高的权限：仅限管理员，官方插件级别。** 授予主机的 DRM 渲染节点（`-render-dev`，真实的 `renderD1xx`，从不是 card 节点，从不是 `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`）：这是内核驱动中的一个攻击面，以及一个与转码器**共享**的 GPU 内存边界（在 AMD、Apple 和 Qualcomm GPU 上可能存在跨进程内存泄漏；挂起的 GPU 可能阻塞转码）。没有这个权限时，GPU 设备节点会被完全排除在 Landlock 设备列表之外（无论文件权限如何都会被拒绝）。有了它，启动器只为那一个文件添加一条明确的 Landlock `IOCTL_DEV` 规则（ABI v5 及以上）；在更旧的 Landlock ABI 上，Landlock 根本不管控 ioctl，所以仅凭读写权限就已经可以提交 GPU 命令。安装器会在预览中显示醒目的 CRITICAL 警告；只授予可信的官方插件。 |

## 外部访问规则

在清单的 `external_fetch: [...]` 中声明，并在安装时由 `validateExternalFetch`（`internal/adapters/driven/pluginrunner/manifest.go`）检查。全有或全无：一个无效条目会拒绝整个安装。

条目是一个**纯主机名**。协议前缀会被容忍并丢弃：只保留主机名。

| 条目 | 有效 | 原因 |
|---|---|---|
| `api.tmdb.org` | 是 | 精确的主机名，正常形式 |
| `https://api.tmdb.org` | 是 | 协议被解析并丢弃，保留主机名 |
| `*.tmdb.org` | 否 | 通配符会被拒绝：列出你需要的每个子域名 |
| `*` | 否 | 同样的规则 |
| `192.168.1.1`、`::1` | 否 | 字面 IP 会被拒绝，两种地址族都是 |

运行时，宿主的代理会加上它自己的限制，清单无法放宽这些限制：只允许 `http` 和 `https`，只允许 `GET` 和 `HEAD`，目标主机以不区分大小写的方式与你的列表比较，DNS 解析后的私有或保留地址会被拒绝；重定向会按这两条规则重新检查。

> 代理的匹配逻辑也能理解 `*.example.com` 这种形式，但安装器会拒绝这样的条目，所以它永远不会被用到。请写明确的主机名。

## 插槽列表

插件只能在明确允许的插槽中注入界面。关键路径被有意排除在外。

| 插槽 | 需要登录 | 说明 |
|---|---|---|
| `home.section` | 是 | 页面主要内容区域 |
| `home.hero` | 是 | 主横幅区域 |
| `library.sidebar` | 是 | 媒体库视图的侧边栏 |
| `media.detail.sidebar` | 是 | 媒体详情的侧边栏 |
| `player.overlay` | 是 | 播放器中的叠加界面 |
| `settings.user.appearance` | 是 | 用户设置中的外观区块 |
| `sidebar.bottom` | 是 | 导航栏的底部区域 |
| `admin.nav` | 是 | 管理面板中的一个导航项，打开插件自己的沙箱化页面；不会向现有的管理界面注入任何东西 |

**永远不允许：** `admin.*`（`admin.nav` 除外）、`login.*`、`setup.*`、`profile.security.*`：这些路径对认证和权限至关重要，在那里注入插件是权限提升或钓鱼的途径。

`admin.nav` 的内容也像其他任何插槽一样被隔离：它在一个 `<iframe sandbox="allow-scripts">`（没有 `allow-same-origin`）中、以 CSP `default-src 'none'` 绘制，所以它是在管理区中显示的第三方内容，而不是授予插件的管理员能力。

来源：`internal/domain/plugin/slots.go`。

前端模块运行在一个无法访问宿主页面的隔离 `<iframe>` 中。桥接限制了它们能做的事：`evey.toast` 每秒最多 3 条消息、200 个字符、不能包含 HTML；`evey.navigate` 只接受 `/home`、`/library`、`/media`、`/player`、`/settings` 和 `/search`；`evey.resize` 被限制在 50-800 px。

## 更新流程

1. 管理员上传新的 ZIP → `PreviewPluginUpdate` 运行静态分析和 cosign 验证，并返回与已安装版本相比在权限、插槽和 `external_fetch` 上的差异。
2. 如果差异不为空（新权限、新插槽、更改的外部主机），管理员必须在应用更新之前明确接受（`requires_ack = true`）。
3. 源文件的校验和在预览时记录，并在应用时检查（防止 TOCTOU：被分析的 ZIP 就是被安装的 ZIP）。
4. 如果 cosign 策略为 `enforce`，应用时签名错误会阻止更新，即使预览已经通过。
