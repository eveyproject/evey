---
source_sha: f8a4c4c0cff61bcab8e0c2da5453fe2689339488b1c66d260a2907b56b1187cb
---
# 配置

两个地方，一条规则。**服务器启动之前就必须存在的东西** - 密钥、容器装配、反向代理相关信息 -
放在 compose 文件里。**运维者之后再调整的东西** - 日志级别、语言、转码上限、硬件加速、文件
监控、扩展沙箱和配额 - 放在**管理 → 服务器配置**中，保存在数据库里，容器重建后依然保留，而且
大多无需重启即可生效。

在环境变量里设置这类参数，只会作为首次启动的初始值。所以本页并不列出二进制接受的所有 `EVEY_*`：
其余的都是首次启动的初始值和应急出口。如果需要完整参考，请看 `internal/config/config.go`。

## 1. 你必须自己设置的

对默认安装来说：什么都不用。媒体路径和镜像标签就写在 compose 文件里，没有 `.env`。而仓库自带的
`docker-compose.yml` 则从 `.env` 读取 `MEDIA_DIR`：

```env
MEDIA_DIR=/path/to/your/media
```

无论哪种方式，Postgres 和 Meilisearch 都内置在容器中运行，并在首次启动时自己生成密钥，保存在
`./evey-data/keys` 下（`postgres.password`、`meili.key`，权限 0600）。服务器通过
`EVEY_DB_PASSWORD_FILE` / `MEILI_MASTER_KEY_FILE`（已写入镜像）读取它们，这也是
`docker exec evey-server evey admin ...` 不需要在命令行上提供密码的原因。只有当你自带数据库或
搜索引擎时，才需要显式设置 `EVEY_DB_PASSWORD` / `MEILI_MASTER_KEY` - 显式的值总是优先于文件。

元数据开箱即用：内置了一个 TMDB 密钥。自己的密钥（在 themoviedb.org 免费申请）会给你独立的
配额；在**管理 → 服务器配置 → API 密钥**中填写，或用 `EVEY_TMDB_API_KEY` 作为初始值。服务器
发布之后，唯一需要添加的变量是 `EVEY_SERVER_BASE_URL` - 见第 2 节。

## 2. 已发布的实例，只需两个变量

在反向代理之后、启用硬件转码时，整个环境只有两个变量。人们常往 compose 文件里写的其他东西，
要么已经在镜像里，要么就是默认值：

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # your reverse proxy, /32 if you know its address
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # hardware transcoding
    group_add:
      - "991"                                 # the group owning /dev/dri/renderD128 on YOUR host
```

从旧配置里**不**值得照搬的行：

| 行 | 为什么不需要 |
| --- | --- |
| `EVEY_LOG_DIR`、`EVEY_TRANSCODING_CACHE_DIR`、`EVEY_TRICKPLAY_CACHE_DIR` | 镜像已经把它们设为 `/data/logs`、`/cache/transcode`、`/data/trickplay`。重复设置不会改变任何东西。 |
| `EVEY_FORWARDED_PROTO` | 已弃用 - 由 `EVEY_SERVER_BASE_URL` 的协议推导。设置它只会记录一条弃用警告。 |
| `EVEY_KEYFRAME_BACKFILL=on` | 除 `off` 以外的任何值都会启用它，不设置也一样。 |
| `LIBVA_DRIVER_NAME` | libva 根据 GPU 的 PCI id 选择驱动。只有当你的显卡需要 libva 不会选的驱动时才强制指定（用 `docker exec evey-server vainfo` 验证）。 |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` 会选择主机支持的最强约束。只有当你需要*封闭失败*的保证时才显式设置 - `landlock` 会拒绝运行它无法约束的扩展，而 `auto` 会带警告地降级。 |
| 为 `/dev/dri/card0` 加的 `group_add` | Evey 只会打开 `renderD128`。用 `ls -ln /dev/dri` 查看，只添加那一个组。 |

## 3. 管理面板替代不了的两项设置

这两项在**管理 → 远程访问**里都有一个长得很像的设置，但两者都不能让环境变量变得多余。这一点
经常让人困惑，所以值得讲清楚。

区别在于各自存放在哪里。远程访问那一块（启用/禁用、外部 URL、LAN 地址段、远程码率上限）**保存
在数据库中** - 它的环境变量只作为首次启动的初始值，之后就被忽略。`EVEY_SERVER_BASE_URL` 和
`EVEY_TRUSTED_PROXIES` 正好相反：每次启动都从环境变量读取，**面板里的任何东西都不会覆盖它们**。
所以在这两种情况下，面板里看起来重复的值，其实是另一项恰好也提到了地址的设置。

**"外部 URL"替代不了 `EVEY_SERVER_BASE_URL`。**面板里的值用于服务器*生成*的 URL。但服务器是否
信任 `X-Forwarded-Proto` - 也就是它是否认为自己通过 HTTPS 提供服务 - 是**在启动时、只根据环境
变量**决定的。去掉这个变量，它就会退回到 `http://localhost:9091`：`Secure` cookie、重定向和
所有绝对链接都会降级为 `http://`，分享/下载 URL 会指向 localhost。面板里的值读取得太晚，帮不上忙。

**"LAN 网络"替代不了 `EVEY_TRUSTED_PROXIES`。**它们是两个阶段，而不是同一件事的两个名字：

1. `EVEY_TRUSTED_PROXIES` 决定**信任谁的 `X-Forwarded-For`**，从而决定客户端得到哪个地址。
2. 然后 LAN 网络再把**这个**地址归类为本地还是远程。

第二步作用于第一步的结果。没有第一步，所有客户端都带着代理的地址到来，归类结果全都一样 - 远程
访问关卡正是这样整整四个月都没起作用（见 `internal/config/remote_access_resolve.go`）。回环
地址始终受信任且无法移除；你的条目是*追加*上去的。

## 4. 文件所有者（PUID/PGID）

**默认：自动。**未设置 `PUID`/`PGID` 时，入口脚本会在启动时查看 `/media*` 挂载的所有者，并以
该用户身份运行服务器，这样*写入*媒体库的功能（跳转索引修复会原地重建文件）开箱即用 - 不用
`chown`，不用配置。启动日志会告诉你选了谁：

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

属于**不同用户**的媒体挂载会尽力处理：服务器采用拥有最多挂载的那个用户，并把其他挂载的所属组
加为附加组，这样只要那些挂载上的文件对组可写，修复就能进行（启动警告会列出哪些挂载可能仍会
拒绝写入）。只有当所有媒体挂载都属于 root 或保留的 uid `1001` 时，自动检测才会完全放弃（带警告
退回 `1000:1000`）- 请把媒体 chown 给普通用户，或者显式设置 `PUID`/`PGID`：

```env title=".env"
PUID=1026   # run the server as this user id  (host: `id -u <media-owner>`)
PGID=100    # ...and this group id
```

显式的值总是优先于检测结果。

工作原理：容器只在重新映射内部用户（以及重新获得 Evey 自己的状态目录 `/data`、`/cache` 的
所有权 - **绝不**碰你的媒体挂载）所需的那一小段时间里以 root 启动，然后放弃特权。通过 compose 的
`group_add:` 授予的附加组（例如主机上用于 `/dev/dri` 硬件转码的 render 组）在放弃特权后依然保留。
服务器进程本身从不以 root 运行。

完全不用 root 的替代方案：如果连这短暂的 root 你也无法接受，就改为在 compose 文件的服务上设置
`user: "UID:GID"`。入口脚本会检测到非 root 启动，从而跳过重新映射（包括媒体所有者检测）- 这时
保持 `./evey-data` 对该 UID 可写就是你自己的事了。

注意：
- `PUID=1001` 会被拒绝：那是内部的 `evey-plugin` 用户，刻意分开，以免扩展与服务器共用身份。
- 媒体默认以读写方式挂载，以便修复可以直接进行；如果你想要只读的媒体库，就在媒体卷后加上 `:ro`
  （此时修复会以明确的错误失败）。

## 5. 变量参考

### 服务器
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_DEMO=false                        # public demo instance only (demo.eveyproject.org): locks admin writes, no mail/push/webhooks
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### 数据库
```env
# Embedded (default when EVEY_DB_HOST is unset): Postgres 17 inside the container.
EVEY_EMBEDDED_POSTGRES=            # true | false; unset = embedded iff EVEY_DB_HOST is unset
EVEY_POSTGRES_DATA_DIR=/data/postgres   # cluster location inside the container (under ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # unset = generated on first boot into /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # read when EVEY_DB_PASSWORD is unset (image default)

# External: use these INSTEAD of the block above, not alongside it.
# Setting the host turns the embedded database off.
EVEY_DB_HOST=postgres              # the repository compose's sibling service is `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # or disable | require | verify-full
```

内置的 Postgres 只监听容器的回环地址，并要求通过 TCP 提供密码；生成的密码就是服务器（以及
`evey admin`）使用的那个。它的数据目录必须位于真正的文件系统上（ext4、btrfs、xfs……），不能
放在网络共享上。两种布局下，迁移都会在启动时自动执行。

### 认证
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### 媒体库
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

媒体库扫描探测器在进程内读取 `.mkv`/`.webm` 和 `.mp4`/`.m4v` 的元数据（mkvgo，只读文件头 -
在重新扫描/修复/NAS 上快约 35 倍），无需任何配置：它始终开启。ffprobe 仍然是权威，负责处理
快速路径无法逐字段对应的任何文件（HDR/Dolby Vision、未映射的编码……）。见
[adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md)。

### 转码
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

硬件设置见 [transcoding.md](transcoding.md)。

**Chromecast 上的杜比音频**（`EVEY_CAST_WASM_AUDIO_ENABLED`，默认 `false`）。Evey 可以把原始的
AC-3/E-AC-3 音轨交给投屏接收端，让它在电视上用 WebAssembly 解码，从而免去转码。它**对投屏
接收端是关闭的**，因为一台运维者的低端设备在走这条路径时两次在电影播到一半时被杀掉 - 而且它的
解码器在那里测出来只有 3.13 倍实时速度，根本没带来好处。关闭时，投屏拿到的是带 AAC 音轨的普通
混流 master：在所有设备上从第一秒起就有声音。浏览器**不受**影响 - 它们保留 wasm 解码器，那里
已经验证过。设为 `true` 可为投屏重新启用。

**优先使用管理面板。**并发数、硬件加速、AV1、缓存预算以及高级限制（HLS 最大并发会话数、软件
和软件 HDR 的高度上限、一次只编码一档、每档一个硬件编码器）都在**管理 → 转码**中。在那里保存的
值重启后依然保留，并优先于环境变量；面板会注明哪些要重启后才生效。

**后台工作器开关**以行的形式设置在**管理 → 服务器配置**中（面板会建议键名，并为每个键显示对应的
环境变量和期望值）。它们都在启动时读取，因此重启后生效：

| 键 | 值 |
| --- | --- |
| `transcoding.keyframe_backfill` | 设为 `off` 关闭关键帧索引回填（默认开启） |
| `transcoding.keyframe_backfill_batch` | 每轮条目数，> 0 |
| `transcoding.keyframe_backfill_pause_ms` | 条目之间的毫秒数，>= 0 |
| `transcoding.keyframe_backfill_idle_sec` | 空闲轮次之间的秒数，> 0 |
| `transcoding.level_backfill` | 设为 `off` 关闭 H.264/HEVC level 回填（默认开启） |
| `transcoding.level_backfill_batch` | 每轮条目数，> 0 |
| `transcoding.level_backfill_pause_ms` | 条目之间的毫秒数，>= 0 |
| `transcoding.level_backfill_idle_sec` | 空闲轮次之间的秒数，> 0 |
| `transcoding.pause_bg_on_playback` | 设为 `off` 让后台工作在播放时继续运行（默认开启：它会让路） |
| `transcoding.snapshot_fastpath` | 设为 `off` 强制使用精确的 ffmpeg 提取器，而不走 mkvgo 的关键帧快速路径 |

每一项在环境变量里也有对应（`EVEY_KEYFRAME_BACKFILL`、`EVEY_LEVEL_BACKFILL_BATCH`……），但面板
里的行优先。

**后台 CPU 预算**（管理 → 服务器配置 → 分配的算力）限制所有后台任务加起来能占用的处理器，实时
生效（无需重启）：

| 键 | 值 |
| --- | --- |
| `background.cpu_percent` | 所有后台任务共同使用的 CPU 容量份额：`25`（默认，温和）或 `50`（加速） |
| `background.cpu_percent.<task>` | 某项繁重任务自己的份额，`1`..`80`，替代该任务的全局份额：`library_scan`、`trickplay_generation`、`intro_detection`、`complexity_classifier` |

份额从不叠加：后台运行的所有东西加起来，都不超过设置的最高份额。份额会换算成整数个核心（8 核的
25 % = 2），至少一个。每个后台 ffmpeg 启动前都会在这个预算里预留线程；媒体库扫描和片头检测据此
确定并行度，trickplay 生成据此确定解码线程数。播放时仍会暂停所有后台分析
（`transcoding.pause_bg_on_playback`）。`EVEY_LIBRARY_SCAN_WORKERS` 和
`EVEY_INTRO_DETECTION_MAX_PARALLEL` 可以手动设定这两个池的大小，并为它们绕过预算。旧的键
`trickplay.adaptive_cpu`（`false` = 50 %）和 `intro_detection.use_all_cores`（`true` = 片头
检测用 80 %）在设置新键之前仍然有效。

**转码缓存预算**（管理 → 服务器配置 → 服务器设置，`transcoding.cache_max_gb`，默认 `10`，重启后生效）限制
整个转码缓存在磁盘上的大小：播放分段、快速启动的 remux 和兼容下载加在一起，归到一个数字下。
它刻意采用绝对大小：缓存位于 `evey-cache` 卷上，与运维者在那里运行的其他东西并存，卷的其余
部分做什么由运维者决定。由一个清理器负责执行，最久未使用的先清，只保留观众正在读取的内容。无论
预算多少，它还会在缓存卷上保留 2 GiB 空闲空间，并清掉 7 天无人取走的下载。拷贝（remux）会话
从不保存整部电影：它只在观众周围保留一个分段窗口，窗口之外的内容会重新从源文件定位读取。服务
大多数 MKV 和 MP4 源的零存储 mkvgo 路径则完全不碰缓存。

### 扩展（沙箱与限制）
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - run plugins as a separate user to wall off secrets (auto = on only when the effective sandbox resolves to "basic", i.e. neither Landlock nor bubblewrap); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0-19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

按场景的指南：[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。完整模型：[SECURITY.md](SECURITY.md)。

### 邮件（SMTP）
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

见 [notifications.md](notifications.md)。

### SSO（OIDC）
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

见 [oidc.md](oidc.md)。

### 备份
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

见 [backups.md](backups.md)。

### 反向代理
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # added to the always-trusted loopback, not a replacement
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

HTTPS 行为（Secure cookie、HSTS、信任 `X-Forwarded-Proto`）跟随 `EVEY_SERVER_BASE_URL` 的
协议：`https://` 的基础 URL 会启用它。`EVEY_FORWARDED_PROTO=true|false` 已弃用，仅作为覆盖项
保留。

见 [reverse-proxy.md](reverse-proxy.md)。

### 片头检测
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # every player auto-skips detected intros (web/mobile read the flag in the segments response; cast receivers get intro segments gated by it)
```

`api.tidb_key`、`intro_providers.enabled`、`intro_providers.tidb_submit_enabled`、
`intro_providers.skip_preview_enabled` 和 `intro_providers.auto_skip_enabled` 也可以通过
`/admin/server-config` 配置（优先数据库，环境变量作为后备）。切换 `intro_providers.enabled`
后请重启服务器；跳过预告和自动跳过片头这两个开关实时生效（无需重启）。

见 [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md)。

### 日志
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # the image's default; the compose file maps it to ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

`EVEY_LOG_DIR` 下每天一个的 `evey-YYYY-MM-DD.log` 文件才是权威日志。`docker logs evey-server`
显示的是同一日志流，但 Docker 的 json-file 驱动会轮转它，轮转之后这个命令看起来可能比容器早几个
小时就停了。两者不一致时，请相信文件 - 或者以 root 身份查看原始的
`/var/lib/docker/containers/<id>/*-json.log*` 文件。在镜像之外（不用容器的安装），默认是相对于
工作目录的 `logs` 目录，所以请把 `EVEY_LOG_DIR` 指向一个可写的绝对路径。

### 远程访问（谁能从你的网络外部连接）
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # unset = auto (see below); true/false to decide yourself
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # the public address, if it differs from EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # CIDRs treated as LAN
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = no cap for remote (WAN) sessions
```

> ⚠️ 这道关卡保护的是 **HTTP** 监听端口（`EVEY_SERVER_PORT` + 1）。`EVEY_SERVER_PORT` 上的
> 原生 gRPC 监听本身是内部传输通道，不受关卡保护，所以它**只绑定在回环地址**
> （`EVEY_GRPC_BIND`，默认 `127.0.0.1`）：它唯一的客户端是同一进程内的 HTTP 网关。设置
> `EVEY_GRPC_BIND=0.0.0.0` 可以有意暴露它 - 启动时会警告远程访问关卡覆盖不到它，用防火墙保护它
> 就成了你的事。

**关卡。**远程访问关闭时，IP 不在 `EVEY_REMOTE_ACCESS_LAN_NETWORKS` 中的客户端，在每个路由上都会
被 `403 {"code":"remote_access_disabled"}` 拒绝。这是一项部署上的选择（"这个实例只给我自己的
网络用"），而不是安全层 - 保护你媒体库的是认证。

**默认自动。**不设置 `EVEY_REMOTE_ACCESS_ENABLED` 时，服务器在启动时决定：如果
`EVEY_REMOTE_ACCESS_EXTERNAL_URL` 或 `EVEY_SERVER_BASE_URL` 是可公开访问的地址，远程访问就**开启**
（你发布的服务器本来就是要被访问的）；否则它只在局域网内可用。这个决定会在启动时记录，
`evey admin doctor` 也会报告它。

设置该变量 - 或使用**管理 → 远程访问** - 可以明确决定；明确的选择永远不会被推断覆盖。如果你
刻意让一台已发布的服务器只在局域网内可用，启动日志会带一条警告说明这一点，因为否则你收到的症状
会是"你手机上能用，我手机上不行"。

> 在反向代理之后，只有当代理转发客户端的真实 IP、并且 `EVEY_TRUSTED_PROXIES` 覆盖了该代理时，
> 这才有效。否则每个客户端看起来都是代理自己的地址 - 见 [reverse-proxy.md](reverse-proxy.md)。

设置了 `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` 后，客户端 IP 被归为 WAN 的播放会话会被限制在这个
码率；该上限只会**降低**客户端/个人资料的上限。有两条安全规则需要了解：

- RFC1918/私有的客户端 IP **永远**不会被限速，哪怕你忘了把它的地址段写进
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS` - 上限只落在明确的公网地址上。
- 如果你使用不常见的本地地址段（CGNAT `100.64.0.0/10`、某个 VPN/Tailscale 子网），又**不**想
  限制这些观众，请把它们明确写进 `EVEY_REMOTE_ACCESS_LAN_NETWORKS`。在反向代理之后，请确保它
  转发了客户端的真实 IP（`X-Forwarded-For`），否则每个会话看起来都来自代理的地址。
