---
source_sha: 28799abda1b5a9531b81bdebfe211e4bbd56a063012a9faec636c9dc4128e730
reviewed: true
---
# 插件

这一页写给管理员：怎样安装插件或主题，安装时您接受了什么，以及服务器允许它碰到您主机上的哪些东西。
所有操作都在**管理 → 插件**里进行；写给插件作者的页面列在[本页末尾](#writing-a-plugin)。

插件为 Evey 加上它本身不做的事：首页上的一个栏目、播放器上的一层叠加内容、一个元数据来源、一个主题。
每个插件都是一个单独的进程，通过本机的套接字和服务器通信，被关在沙箱里，并且只能使用它的清单（manifest）里声明的权限。
如果插件有界面，界面在页面里一个隔离的框架中运行。

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![没有插件的首页](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>没有插件（界面为英文）。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![装了天气插件的首页](images/home.webp){ loading=lazy }
  <figcaption>装了示例天气插件：最上面多了一个栏目，在它自己的框架里绘制（界面为英文）。</figcaption>
</figure>

</div>

## 安装插件

1. 在**管理 → 插件**里，打开**安装插件**，选择**插件文件 (.zip)**。如果作者提供了签名，
   把它加到**签名文件 (.zip.sig，可选)**里。**从 URL 安装**选项卡则是通过链接安装；
   在 URL 后面加上 `#sha256:<hash>`，下载的文件就会被核对。
2. **预览并安装**会分析这个安装包，但不安装它。预览显示插件请求的**权限**和**功能**、
   它想出现在界面的哪些位置（插槽）、它会联系哪些主机、**静态分析**的结果，
   以及在服务器检查签名时，安装包是否**已签名**。
3. 请把这份清单当作合同来读：插件能做的事，只有这些。确认后，插件就会启动。

<figure class="evey-shot" markdown="span">
  ![演示服务器上的管理 → 插件](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>管理 → 插件</strong>：可用的运行时、资源限制，以及每个插件各一张卡片（界面为英文）。</figcaption>
</figure>

如果分析在安装包里发现了严重的问题，不管您怎么回答，安装都会被拒绝。警告会显示出来，由您决定。
插件使用的语言必须在服务器的**可用运行时**里：默认镜像只带 Python，`-nodejs` 镜像加上了 Node.js，
`-full` 镜像再加上 PHP、Ruby 和 Lua。

!!! tip "签名的插件"
    插件作者可以用 [cosign](https://docs.sigstore.dev/cosign/installation/) 给 ZIP 签名，
    而没有签名或签名有问题的插件会怎样，由您决定。要验证签名，请把作者的公钥交给服务器，并选择一种策略：

    ```sh
    EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn（默认）| enforce
    ```

    设为 `warn` 时，签名验证失败会显示在预览和**安全仪表板**里；设为 `enforce` 时，安装或更新会被拒绝。
    没有公钥就什么都不验证，而且服务器不需要 `cosign` 程序。插件作者请看
    [编写插件](plugins/dev-setup.md#7-sign-your-plugin)。

## 更新插件

在插件的卡片上，**从源更新…**接收新的 ZIP（一个路径或一个 URL），并显示同样的预览。
更新的版本号必须比已安装的高。如果更新请求了新的权限、功能、插槽或主机，预览会显示**此更新请求新权限**，
并列出差别：在您选择**批准并安装**之前，什么都不会生效。被分析的安装包就是被安装的安装包：
真正应用更新时，会再核对一次它的哈希值。

## 管理已安装的插件

每张卡片都显示插件的状态（**已启用**或**已禁用**），并提供：

| 操作 | 作用 |
| --- | --- |
| **已启用** / **已禁用** | 启动或停止插件的进程。被禁用的插件保留它的文件和配置。 |
| **配置** | 插件声明的设置（一个 API 密钥、一个城市、一个单位……）。机密字段以遮盖的方式显示，永远不会再发回浏览器。 |
| **日志** | 插件通过主机写下的最后几行。 |
| **卸载** | 删除插件的进程、文件和它保存的数据。 |

页面顶部的**资源限制**为每个插件进程设定上限。面板里的值就是默认值；
如果面板从来没有保存过，就由环境变量来设定它们。

| 字段 | 默认值 | 环境变量 |
| --- | --- | --- |
| **最大内存 (MiB)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **最大进程数** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **最大磁盘 (MiB)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **CPU nice (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **沙箱模式** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

内存、进程和磁盘的限制在监控程序下一次巡查时生效，超出限制的插件会被停止；
CPU 优先级和沙箱模式对之后启动的插件生效。`0` 表示不限制内存、进程或磁盘。

**安全仪表板**按插件保存分析发现的问题、签名结果和最近一次错误。它的**主机沙箱**一行显示这台机器上实际生效的沙箱模式：
如果显示 `basic`，说明主机无法按路径隔离文件系统，[插件沙箱设置](plugins/sandbox-deployment.md)说明了要改什么。

插件默认是开启的。`EVEY_PLUGINS_ENABLED=false` 会关闭整个插件子系统。

## 插件在您的主机上能做什么

不管它的清单怎么写，一个插件：

- 只看得到它自己的目录，看不到服务器的数据，也看不到媒体文件；
- 不能直接上网：有 `network` 权限的插件，也只能经过服务器、用 `GET` 和 `HEAD` 访问它声明过的主机；
- 不能启动子进程，除非它声明了 `subprocess`；
- 受上面那些资源限制的约束；如果主机允许，还会以一个单独的用户运行。

它的界面只能出现在清单声明的插槽里：首页的一个栏目或顶部的大图区域、媒体库或详情页的一个侧边面板、
播放器上的一层叠加内容、外观设置、导航侧栏的底部，或者管理菜单里的一个条目（点开是插件自己的页面）。
插件永远不能在登录、初始设置、安全相关的页面或现有的管理页面里绘制任何东西。在它的框架里，
它碰不到页面、您的 Cookie 或您的会话：它从服务器读取的内容，都要经过一个短时有效的令牌，
这个令牌只限于它可以申请的两种权限（`public`：媒体标题和海报的安全视图；`prefs`：当前用户在它自己命名空间里的偏好设置）。

插件可以申请的权限列在[权限与插槽](plugins/security-model.md)里。沙箱怎样工作，以及它仍然无法防止什么，
见[插件沙箱与威胁模型](SECURITY.md)。

!!! warning "服务器检查插件和谁通信，而不检查它说了什么"
    一旦某个主机出现在插件的列表里，服务器就无法检查插件发给它的内容。安装第三方插件之前，
    请像对待任何软件一样先审查它，并把它的主机列表看作可能收到您媒体库标题的各方的名单。

## 主题

主题是一个 `kind: theme` 的安装包，没有进程：一个样式表，还可以带一个被关在隐藏框架里的脚本。
它也是通过同一个**安装插件**对话框安装的；样式表在安装时会被清理。之后，每个用户在**设置 → 外观 → 主题**里选择它。
主题可以包含什么，见[主题](themes.md)。

## 编写插件

| 页面 | 内容 |
| --- | --- |
| [编写插件](plugins/dev-setup.md) | 一步一步做出第一个插件，安装调试循环、签名、发布。从这里开始。 |
| [插件 API 参考](plugins/api-reference.md) | 清单、后端调用和数据存储、界面的 `evey.*` 桥接接口。 |
| [权限与插槽](plugins/security-model.md) | 清单可以声明什么，安装时检查什么，访问外部的规则，各个插槽。 |
| [主题](themes.md) | 用 CSS 和一个沙箱里的脚本重新设计界面的样式。 |
| [Webhook 与 API 密钥](webhooks-api-keys.md) | 不写插件，也能把 Evey 和其他工具集成起来。 |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall ; secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
