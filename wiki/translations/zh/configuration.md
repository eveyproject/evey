---
source_sha: 822f2e4e87b78c5d8f5b5509badc1a7c70b7d5fe0c6a2b84d574662a66b8ba95
reviewed: true
---
# 配置参考 { #configuration-reference }

本页说明：Evey 服务器的每项设置位于哪里，一项设置同时出现在管理面板和环境变量里时以哪个值为准，
以及什么情况需要重启。变量本身按主题列在下面链接的十个页面里；本页面向要修改 compose 文件的管理员，
或者想知道为什么某个变量好像不起作用的人。

## 两个地方，一条规则 { #two-places-one-rule }

**服务器启动之前就必须具备的内容**，例如密钥、存储路径、公网地址、反向代理的情况，作为环境变量写在 compose 文件里。
**管理员之后再调整的内容**，例如语言、转码上限、硬件加速、计划任务、插件限制，放在管理面板里，
保存在数据库中，容器更新后依然保留，而且大多不用重启就能生效。

如果一项设置在两个地方都有，优先顺序永远一样：

1. **面板里保存的值优先。** 每次启动时，服务器先读取环境变量，然后对每个字段，
   如果数据库里存有值，就用它替换。变量永远不会覆盖已保存的值。
2. **没有保存任何值时，使用变量。** 面板把它显示为当前的默认值；**重置为默认值**会删除已保存的值，
   从下次启动起，变量重新生效。
3. 两者都没有设置时，使用**内置的默认值**。

密钥是例外：面板从不显示它保存的密码或密钥；而像 `EVEY_TMDB_API_KEY` 这样的变量，
第一次使用时会被复制到数据库里，这样面板就能显示正在使用自定义密钥。

## 什么需要重启 { #what-needs-a-restart }

- 修改**环境变量**，总是需要重新创建容器（`docker compose up -d`）：进程只读取一次环境变量。
- 在**面板**里修改，大多数设置会实时生效（服务器名称、日志级别、速率限制、硬件加速、SMTP、单点登录、
  远程访问、插件限制；保留期限在下一次每日清理时生效）。
- 面板会把**启动时读取**的设置标上*重启后生效*：媒体库文件监视和扫描间隔、转码缓存预算、
  转码页面的**高级限制**卡片、片头跳过提供商，以及**配置条目**里已知的运维开关（其他键的行不显示该提示）。保存会记录您的选择，重启才会生效。

各个主题页面里的每张表格都有一列**面板**：写着字段的位置（从**管理**开始），以及它是实时生效还是重启后生效；
如果变量在面板里没有对应的设置，就写“无”。

| 页面 | 内容 |
| --- | --- |
| [服务器与网络](environment/server.md) | 公网地址、端口、反向代理、远程访问、请求限制、容器用户 |
| [数据库与搜索](environment/database-search.md) | 内置或外部的 Postgres 和 Meilisearch |
| [账户与登录](environment/accounts.md) | 会话有效期、签名密钥、单点登录 |
| [媒体库与元数据](environment/libraries.md) | 扫描、元数据密钥、片头检测 |
| [转码与硬件](environment/transcoding.md) | 显卡、容量、画质上限、缓存、校准片段 |
| [播放分发与下载](environment/playback-delivery.md) | 视频流打包开关、预先准备的下载、播放器的跳过按钮 |
| [后台工作](environment/background.md) | 后台的各项处理及其节奏 |
| [插件与沙箱](environment/plugins.md) | 沙箱模式、单独的用户、资源限制、签名 |
| [邮件和通知](environment/notifications.md) | SMTP、浏览器推送密钥 |
| [备份、日志和监控](environment/logs-monitoring.md) | 备份、日志、历史记录保留、指标和追踪 |

## 值是怎样读取的 { #how-values-are-read }

- 布尔值接受 `1`、`t`、`true`、`0`、`f`、`false`。时长必须带单位：`30s`、`5m`、`2h`（单独写 `30` 会被忽略）。
  列表用逗号分隔。
- 服务器无法解析的值，会**被忽略，并在日志里给出警告**
  （`environment variable ignored, value could not be parsed`），然后使用默认值。修改 compose 文件后，
  请看一看日志的前几行。
- `EVEY_DB_PASSWORD` 和 `MEILI_MASTER_KEY` 也可以来自一个文件，这个文件由同名加 `_FILE` 后缀的变量指定；
  明确给出的值优先于文件。

## 密钥 { #secrets }

内置的 Postgres 和 Meilisearch 在第一次启动时生成自己的密钥，放在 `./evey-data/keys` 里
（`postgres.password`、`meili.key`，权限 0600）。服务器通过镜像里设置好的 `EVEY_DB_PASSWORD_FILE` 和
`MEILI_MASTER_KEY_FILE` 读取它们，这也是 `evey admin` 命令不需要密码的原因。
只有使用外部数据库或搜索引擎时，才需要设置 `EVEY_DB_PASSWORD` 或 `MEILI_MASTER_KEY`。

Evey 内置了一个 TMDB 密钥。`EVEY_TMDB_API_KEY` 可以提供您自己的密钥作为初始值；
在**管理 → 服务器配置 → API 密钥**里设置的密钥优先于它。

## 不需要的变量 { #variables-you-do-not-need }

这些行常常是从旧的配置里带过来的，下面说明为什么它们不起任何作用：

| 行 | 为什么不需要 |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | 镜像已经把它们设为 `/data/logs`、`/cache/transcode`、`/data/trickplay`。重复设置不会改变任何东西。 |
| `EVEY_FORWARDED_PROTO` | 已弃用，由 `EVEY_SERVER_BASE_URL` 的协议推断。设置它只会记录一条“已弃用”的警告。 |
| `EVEY_KEYFRAME_BACKFILL=on` | 除了 `off` 以外的任何值都会开启它，不设置也一样。 |
| `LIBVA_DRIVER_NAME` | libva 根据 GPU 的 PCI id 选择驱动。只有当您的显卡需要一个 libva 不会自动选择的驱动时，才需要强制指定（用 `docker exec evey-server vainfo` 确认）。 |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` 会选择您的主机支持的最强隔离方式。只有当您想要“无法隔离就拒绝”（*fail-closed*）的保证时，才需要明确设置：`landlock` 会拒绝运行它无法隔离的插件，而 `auto` 会降级并给出警告。 |
| 为 `/dev/dri/card0` 添加的 `group_add` | Evey 只会打开渲染节点（`/dev/dri/renderD*`，主机上有哪个就是哪个），从不打开 `card*`。用 `ls -ln /dev/dri` 查看，只添加渲染节点所属的组。 |

## 文件所有者（PUID 和 PGID） { #file-ownership-puid-and-pgid }

**默认：自动。** 没有设置 `PUID`/`PGID` 时，入口脚本会在启动时查看您的 `/media*` 挂载目录归谁所有，
并以那个用户运行服务器。这样，那些会*写入*您媒体库的功能（修复跳转索引时会就地重建文件）无需额外设置即可使用：
不用 `chown`，也不用任何配置。启动日志会显示选择了哪个用户：

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

只有真正挂载的 `/media*` 才算数：镜像自带的空 `/media`（每个 Debian 镜像都有一个）会被忽略，所以放在别处（比如 `/data` 下）的媒体库启动时不会有任何所有权警告。

如果媒体挂载目录属于**不同的用户**，服务器会尽力处理：它采用拥有挂载目录最多的那个用户，
并把其他挂载目录的组加为附加组，这样，只要那些挂载目录里的文件允许组写入，修复就能进行
（启动时的警告会列出哪些挂载目录仍可能拒绝写入）。只有当所有媒体挂载目录都属于 root，
或者在启用插件时属于保留的 uid `1001` 时，自动检测才会完全不生效（改用 `1000:1000`，并给出警告）。
这时，请把您的媒体 chown 给一个普通用户，或者在 compose 文件的 `environment:` 下明确设置 `PUID`/`PGID`：

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      PUID: "1026"   # 以这个用户 id 运行服务器（在主机上查看：`id -u <media-owner>`）
      PGID: "100"    # ……以及这个组 id
```

明确设置的值永远优先于自动检测。

工作原理：容器以 root 身份启动，仅用于重新映射它内部的用户
（并重新设置 Evey 自己的状态目录 `/data`、`/cache` 的所有者，**绝不**改动您的媒体挂载目录），然后放弃特权。
通过 compose 的 `group_add:` 授予的附加组（例如为了 `/dev/dri` 硬件转码而加入的主机 render 组）在放弃特权后仍然保留。
服务器进程本身从不以 root 身份运行。

完全不用 root 的做法：如果连这短暂的 root 您也不能接受，可以改为在 compose 文件里给服务设置 `user: "UID:GID"`。
入口脚本会发现自己不是以 root 启动，于是跳过重新映射（也跳过检测媒体的所有者）。
这时，需要由您自己保证 `./evey-data` 对这个 UID 可写。

!!! note
    启用插件时，`PUID=1001` 会被拒绝：它是内部的 `evey-plugin` 用户，单独留出来，是为了让插件永远不会和服务器使用同一个身份。
    媒体默认以读写方式挂载，这样修复可以直接进行；如果您希望媒体库只读，请在媒体卷后面加上 `:ro`
    （这时修复会失败，并给出明确的错误）。

## 服务器不读取的变量 { #variables-the-server-does-not-read }

有些变量出现在 compose 文件里，但服务器并不读取：

- `EVEY_HTTP_PORT`、`EVEY_DOMAIN`、`MEDIA_DIR`：由代码仓库自带的 `docker-compose.yml` 使用
  （对外发布的端口、随附的 Caddy 使用的域名、媒体路径），不是由 Evey 使用。`EVEY_HTTP_PORT` 只是主机端口：容器始终监听 9091，compose 映射为 `${EVEY_HTTP_PORT:-9091}:9091`。`EVEY_VERSION` 在那里是镜像标签；
  只有当服务器的程序本身不带版本号时，服务器才会在 `/healthz` 上显示它。
- `EVEY_WEB_DIR`（`/srv`，网页界面文件所在的位置），以及 `EVEY_DB_PASSWORD_FILE`、`MEILI_MASTER_KEY_FILE`、
  `EVEY_IMAGE_REPO`、`EVEY_IMAGE_RUNTIMES`：与镜像的目录结构相对应。不支持修改它们。
- `EVEY_SUPERVISED`、`EVEY_COMPANION_PIDS`、`EVEY_RESTART_EXIT_CODE`、`EVEY_LAUNCHER_BIN`、`EVEY_PLUGIN_LAUNCHER`：
  由入口脚本和插件启动器设置，供它们自己使用。
- `EVEY_BANNER=off` 只是隐藏日志里的启动横幅；`NO_COLOR` 去掉横幅的颜色。

<!-- sources
precedence: cmd/evey/main.go - loadOverride (DB row wins when non-empty) ; cmd/evey/wire_helpers.go - resolveFlag, resolveAPIKeyWithBuiltin (env seeded into server_config)
reset to default: internal/adapters/driving/http/settings_handler.go - reset / resetSideEffect / requiresRestart
parsing: internal/config/config.go - envBool, envDuration, envStrSlice, warnBadEnv, envSecret (_FILE)
restart: internal/adapters/driving/http/settings_handler.go (requiresRestart) ; i18n admin.transcoding.advancedRestartHint ; admin.serverConfig.appliedOnRestart ; admin.serverConfig.introProvidersRestart
variables: internal/config/config.go ; docker/entrypoint.sh ; Dockerfile ; docker-compose.yml ; cmd/evey/banner.go
-->
