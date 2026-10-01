---
source_sha: 4346505ac20853c385d4b6641af12c128cdd868479ccef57a064e5b9e0901fce
reviewed: false
---
# 配置参考

两个地方，一条规则。**服务器启动之前必须存在的东西**（机密、存储、公网地址、反向代理信息）放在 compose 文件中。**管理员之后调整的东西**（语言、转码限制、硬件加速、计划、插件限制）放在管理面板中，保存在数据库里，在容器更新后依然保留，并且大多无需重启即可生效。对于两处都有的设置，环境变量只为首次启动提供初始值：以面板中保存的值为准。

按主题划分的变量：

| 页面 | 涵盖 |
| --- | --- |
| [服务器与网络](environment/server.md) | 公网地址、端口、反向代理、远程访问、请求限制、容器用户 |
| [数据库与搜索](environment/database-search.md) | 内置或外部的 Postgres 和 Meilisearch |
| [账户与登录](environment/accounts.md) | 会话有效期、签名密钥、单点登录 |
| [媒体库与元数据](environment/libraries.md) | 扫描、元数据密钥、片头检测 |
| [转码与硬件](environment/transcoding.md) | 显卡、容量、画质限制、缓存、校准片段 |
| [播放分发与下载](environment/playback-delivery.md) | 流封装开关、预先准备的下载、播放器跳转按钮 |
| [后台任务](environment/background.md) | 后台处理及其节奏 |
| [插件与沙箱](environment/plugins.md) | 沙箱模式、单独用户、资源限制、签名 |
| [电子邮件与通知](environment/notifications.md) | SMTP、浏览器推送密钥 |
| [备份、日志与监控](environment/logs-monitoring.md) | 备份、日志、历史保留、指标与追踪 |

## 机密

内置的 Postgres 和 Meilisearch 在首次启动时生成它们的机密，存放在 `./evey-data/keys`（`postgres.password`、`meili.key`，权限 0600）。服务器通过镜像中设置的 `EVEY_DB_PASSWORD_FILE` 和 `MEILI_MASTER_KEY_FILE` 读取它们，这也是 `evey admin` 命令不需要密码的原因。只有在使用外部数据库或搜索引擎时才设置 `EVEY_DB_PASSWORD` 或 `MEILI_MASTER_KEY`；显式的值优先于文件。

内置了一个 TMDB 密钥。`EVEY_TMDB_API_KEY` 提供你自己的密钥的初始值；在 **Admin → 服务器配置 → API 密钥** 中设置的密钥优先于它。

## 不需要的变量

常常从旧配置中带过来、但不会改变任何东西的行，以及原因：


| 行 | 为什么不需要 |
| --- | --- |
| `EVEY_LOG_DIR`、`EVEY_TRANSCODING_CACHE_DIR`、`EVEY_TRICKPLAY_CACHE_DIR` | 镜像已经把它们设置为 `/data/logs`、`/cache/transcode`、`/data/trickplay`。重复设置不会改变任何东西。 |
| `EVEY_FORWARDED_PROTO` | 已弃用：由 `EVEY_SERVER_BASE_URL` 的协议推导。设置它只会记录一条弃用警告。 |
| `EVEY_KEYFRAME_BACKFILL=on` | 除 `off` 以外的任何值（包括不设置）都会启用它。 |
| `LIBVA_DRIVER_NAME` | libva 根据 GPU 的 PCI id 选择驱动。只有当你的显卡需要 libva 不会选择的驱动时才强制设置（用 `docker exec evey-server vainfo` 验证）。 |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` 会选择你的主机支持的最强隔离。只有在你想要*失败即拒绝*的保证时才显式设置：`landlock` 会拒绝运行它无法隔离的插件，而 `auto` 会降级并给出警告。 |
| 针对 `/dev/dri/card0` 的 `group_add` | Evey 只会打开 `renderD128`。用 `ls -ln /dev/dri` 检查，只添加那个组。 |


## 文件所有权（PUID 和 PGID）

**默认：自动。** 未设置 `PUID`/`PGID` 时，入口脚本会在启动时查看你的 `/media*` 挂载的所有者，并以该用户运行服务器，这样会*写入*你的媒体库的功能（跳转索引修复会就地重建文件）开箱即用：无需 `chown`，无需配置。启动日志会告诉你选择了什么：

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

由**不同用户**拥有的媒体挂载会尽力处理：服务器采用拥有最多挂载的用户，并把其他挂载的组加入为附加组，这样当这些挂载上的文件对组可写时，修复就能工作（启动警告会列出哪些挂载仍可能拒绝写入）。只有当所有媒体挂载都属于 root 或保留的 uid `1001` 时，自动检测才会完全让步（退回到 `1000:1000` 并给出警告）：把你的媒体的所有者改为普通用户，或者显式设置 `PUID`/`PGID`：

```env title=".env"
PUID=1026   # 以这个用户 id 运行服务器  （主机上：`id -u <media-owner>`）
PGID=100    # ……以及这个组 id
```

显式的值总是优先于检测结果。

工作原理：容器以 root 启动，时间只够重新映射其内部用户（并重新设置 Evey 自己的状态目录 `/data`、`/cache` 的所有权；**永远不会**动你的媒体挂载），然后放弃特权。通过 compose 的 `group_add:` 授予的附加组（例如主机上用于 `/dev/dri` 硬件转码的 render 组）在降权后会被保留。服务器进程本身从不以 root 运行。

完全不用 root 的替代方案：如果连这短暂的 root 你都无法接受，就在 compose 文件的服务上设置 `user: "UID:GID"`。入口脚本会检测到非 root 启动并跳过重新映射（包括媒体所有者检测）：此时保持 `./evey-data` 对该 UID 可写就由你自己负责。

注意：
- `PUID=1001` 会被拒绝：它是内部的 `evey-plugin` 用户，单独保留是为了让插件永远无法共享服务器的身份。
- 媒体默认以读写方式挂载，以便修复能够直接工作；如果你更想要只读的媒体库，在媒体卷后加上 `:ro`（修复随后会以明确的错误失败）。

## 服务器不读取的变量

有些变量出现在 compose 文件中，但服务器并不读取它们：

- `EVEY_HTTP_PORT`、`EVEY_DOMAIN`、`MEDIA_DIR`：由仓库自己的 `docker-compose.yml` 使用（发布的端口、自带 Caddy 的域名、媒体路径），而不是由 Evey 使用。`EVEY_VERSION` 在那里是镜像标签；只有当二进制文件不带版本号时，服务器才会在 `/healthz` 上显示它。
- `EVEY_WEB_DIR`（`/srv`，网页界面文件所在的位置），以及 `EVEY_DB_PASSWORD_FILE`、`MEILI_MASTER_KEY_FILE`、`EVEY_IMAGE_REPO`、`EVEY_IMAGE_RUNTIMES`：与镜像的布局相对应。不支持修改它们。
- `EVEY_SUPERVISED`、`EVEY_COMPANION_PIDS`、`EVEY_RESTART_EXIT_CODE`、`EVEY_LAUNCHER_BIN`、`EVEY_PLUGIN_LAUNCHER`：由入口脚本和插件启动器为自己使用而设置。
