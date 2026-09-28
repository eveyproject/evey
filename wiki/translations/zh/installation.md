---
source_sha: c781817bb39af138ca65b5a030c42b0e6e199a7cd2613b2415b3dcba8e7427be
---
# 安装

Evey 以**单个容器**运行:镜像在同一个端口上提供 API 和网页界面,并在内部运行 Postgres 与
Meilisearch,以非 root 的服务器用户身份运行,密钥在首次启动时生成。第 1 节的 compose 文件
就是完整的安装。它不需要 `.env`,不需要 `--privileged`,也不需要任何内存或 CPU 设置。

> Git 仓库根目录下的 `docker-compose.yml` **不是**这个文件。那是运维布局(第 7 节):
> 独立服务形式的 Postgres、自带的 Caddy,以及一个缺了就无法启动的 `.env`。请复制下面的代码块,
> 而不是仓库里的文件。

不使用 Docker 运行 Evey,请参阅 [deployment-bare-metal.md](deployment-bare-metal.md)。

## 前提条件

- Docker + Docker Compose v2(`docker compose`,而不是旧的 `docker-compose`)。
- 在持久磁盘上存放数据的位置(下面的 compose 在工作目录中使用 bind mount)。
- **推荐:**一个用于 HTTPS 的反向代理 - Evey 只提供明文 HTTP。参阅
  [reverse-proxy.md](reverse-proxy.md)。

## 1. compose 文件

将其保存为 `docker-compose.yml`,并在最后一行 volume 中填入你的媒体路径。密钥会在首次启动时
写入 `./evey-data/keys`;无需自己生成任何东西。

```yaml
services:
  evey:
    image: gravityz/evey:latest        # pin a tag here (e.g. 0.2.0-beta.1, sha-1a2b3c4)
    container_name: evey-server
    ports:
      - "9091:9091"
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091   # e-mail links, OIDC redirects, cast/device URLs
    volumes:
      - ./evey-data:/data              # database, JWT key, generated secrets, search index, logs, backups
      - evey-cache:/cache              # transcode + trickplay cache (safe to prune)
      # Read-write so the seek-index repair rebuilds files in place; append
      # `:ro` for a read-only library and accept that those repairs fail.
      - /path/to/your/media:/media
    # The embedded database is stopped cleanly AFTER the server on `docker
    # stop`; a large catalogue needs more than Docker's 10s default.
    stop_grace_period: 60s
    restart: unless-stopped

volumes:
  evey-cache:
```

让环境变量保持**最少** - compose 文件(或存放密钥的 `.env`)中只放这些:你自带的密钥、外部
数据库的连接信息、存储路径,以及反向代理相关设置。几乎其他一切(服务器名称、并发转码数、
速率限制、日志级别、扫描/监视间隔、备份计划、API 密钥)都**可以在管理 → 服务器配置中实时修改**,
不应该塞进 compose。
`PUID`/`PGID`:默认不设置 → 入口脚本会沿用你 `/media` 挂载的所有者,使原地修复能够工作;
用 `id -u`/`id -g` 覆盖。容器仅为应用该映射而以 root 启动,随后放弃权限 - 服务器本身从不以
root 运行。

**在 NAS 上(Synology Container Manager、QNAP Container Station):**

- 在首次启动**之前**创建数据文件夹(在 compose 旁边执行 `mkdir evey-data`,或通过 File
  Station):与 Docker CLI 不同,这些界面会拒绝文件夹不存在的 bind mount。其他什么都不用预先
  创建 - 入口脚本会自己创建 `plugins/`、`keys/`、`postgres/`。
- 将 `PUID`/`PGID` 设为拥有媒体的 DSM 用户(通过 SSH 执行 `id <user>`;在 Synology 上通常在
  `1026` 左右,组为 `100` = `users`),而不要依赖自动检测:DSM 共享文件夹常常属于 `root`,
  而入口脚本绝不会沿用 root。
- 带 Quick Sync 的 Intel NAS:加上 `docker-compose.gpu.yml` 中的 `devices:` 和
  `group_add:` 行(render 组的 GID 由 `getent group render` 或
  `stat -c %g /dev/dri/renderD128` 给出)。
- 不需要 `.env` - 仓库根目录下需要 `.env` 的 `docker-compose.yml` 是运维布局,不是这个。

## 2. 首次启动

```bash
docker compose up -d
docker compose logs -f evey
```

首次启动时,服务器会:

1. **自动生成 JWT 签名密钥**,位于 `EVEY_JWT_PRIVATE_KEY_PATH`(`/data/jwt.key`,在
   `./evey-data` 内)。你不需要手动创建它。请妥善保护 `evey-data` - 丢失密钥会让所有人登出。
2. **自动运行数据库迁移**(内置,启动时应用;没有单独的迁移步骤)。参阅
   [runbooks/migration-runbook.md](runbooks/migration-runbook.md)。

然后打开 `http://<你的-ip>:9091`。只要还没有管理员,你就会进入**设置向导**(`/setup`):创建
第一个管理员账户。之后登录,并**添加一个媒体库**(管理 → 媒体库),指向 `/media` 下的某个路径
(你的 `MEDIA_DIR`,以读写方式挂载)。第一次扫描会自动开始。向导、媒体库、邀请他人以及公开
地址前要检查什么,都在[第一步](guide/first-steps.md)中。

上面的 compose 文件有意以读写方式挂载它:seek 索引修复会原地重写文件,而 `:ro` 挂载会让这些
修复失败。只有在你愿意放弃它们时才加上 `:ro`。

## 3. 常见调整

**变体,每一种都是在同一个文件中改几行:**

- *第二个媒体文件夹*:在 `volumes:` 下添加 `- /other/path:/media2`(`/media3`,……依此
  类推)。以读写方式挂载,见上面的说明。
- *放在你已经在运行的反向代理后面*(Nginx Proxy Manager、Traefik……位于某个 Docker 网络上):
  去掉 `ports:`,加入代理的网络,并告诉服务器它的公开 URL 以及要信任哪个邻居:
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
  networks:
    default:
      name: proxy-net        # the network your proxy is on
      external: true
  ```
  然后让代理指向 `evey-server:9091`。各代理的设置:[reverse-proxy.md](reverse-proxy.md)。
- *还没有代理,但想要 HTTPS*:从 [reverse-proxy.md](reverse-proxy.md) 添加自带的 Caddy
  服务(自动获取 Let's Encrypt 证书)。
- *Postgres 或 Meilisearch 作为独立服务*:第 7 节。

**HTTPS 与反向代理。**Evey 在 9091 端口提供明文 HTTP。如果还没有代理,
[reverse-proxy.md](reverse-proxy.md) 中自带的 Caddy 服务会为你获取 Let's Encrypt 证书
(80/443 端口需能到达主机)。使用你自己的代理(Nginx、Traefik、Nginx Proxy Manager)时:将
`EVEY_SERVER_BASE_URL` 设为你公开的 `https://` URL(正是这个 scheme 启用 Secure cookie 和
HSTS),并通过 `EVEY_TRUSTED_PROXIES` 信任该代理。流媒体端点需要关闭缓冲且不设超时 - 现成的
配置已经处理好了。完整设置:[reverse-proxy.md](reverse-proxy.md)。

## 4. 更新 Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # until "healthy" (a migration on a large catalogue takes minutes)
```

只需要这些。新镜像在启动时应用待执行的迁移,并在此之前导出一份数据库 dump
(`./evey-data/backups/pre-migration-*`);容器的健康检查一旦开始提供服务就变绿;`up -d`
只重启 `evey` 服务 - 旁边的数据库或代理永远不会被触碰。

**回退。**把上一个标签写回 `image:` 行(或者写进 `EVEY_VERSION`,如果你的 compose 读取它),
然后重新执行 `docker compose up -d`。如果新版本迁移了 schema,先恢复它在迁移前导出的 dump -
`docker exec evey-server evey admin rollback` 会连同 dump 路径打印出确切的步骤。

**确认正在运行什么。**`docker exec evey-server evey version` 打印正在运行的二进制的版本和
commit(`/healthz` 也会报告),因此“这是哪份代码?”从不依赖于某个标签。

> 从仓库 checkout 运行的运维者有一个做同样事情的脚本:`scripts/evey-update.sh`(默认 dry-run;
> 状态、更新前 dump、pull、等待健康、commit 校验、回退命令行)和 `scripts/evey-rollback.sh`。
> 它们是仓库自身部署的便利工具,安装并不需要。

**固定你运行的版本。**在 `image:` 行使用发布标签(`0.2.0-beta.1`)或不可变的
`sha-<short>` 标签;像 `beta-0.0.1` 这样的分支标签随每次推送移动,所以相隔一天的两次 pull
可能运行不同的代码,而“回到 `beta-0.0.1`”也不会把你带到任何地方。脚本会打印它从哪个标签启动,
方便你固定它。

**镜像标签**(`gravityz/evey`):

- 发布标签 `vX.Y.Z` 会发布 `X.Y.Z`、`X.Y`、`X` 和 `latest`。`latest` 始终指最新的
  semver **发布版**。
- 分支构建发布分支名(例如 `beta-0.0.1`)和 `sha-<short>`。
- **运行时变体** - 同一个应用,附带不同的扩展运行时:
  - 无后缀(默认 / “lean”)- 仅 Python 3。
  - `-nodejs` - 为 JS 扩展加入 Node。
  - `-full` - 另外加上 PHP + Ruby + Lua。
  只有当你的扩展需要这些运行时时才选 `-nodejs` / `-full`;否则默认的最小。例如设置
  `EVEY_VERSION=latest-nodejs`。
- **GPU 变体** - 发布的标签包含所有加速器家族(Intel 从 Gen9 到现在、AMD),仅 Intel 的
  OpenCL 编译器就约 900 MB。自己构建的镜像可以只保留主机所属的家族:
  `docker build --build-arg GPU=intel-legacy .`(Intel Gen9 到 Gen11,UHD 6xx)、
  `GPU=intel`(Gen12 及更新)、`GPU=amd` 或 `GPU=none`。镜像标签 `org.evey.gpu` 记录了
  选择。NVIDIA 不需要变体:没有任何镜像内置 NVIDIA 库,NVIDIA Container Toolkit 在运行时
  注入主机驱动(`--gpus all` 或 compose 中的设备预留),所以 NVENC 在每个变体下都能工作,
  包括 `none`。在同时有 Intel 核显和 NVIDIA 显卡的机器上,校准的 `auto` 模式会先选 VAAPI;
  要使用显卡,请在管理后台的转码面板中把硬件模式固定为 `nvenc`。

## 5. 备份与恢复

开启定时数据库备份(`EVEY_BACKUP_INTERVAL_HOURS`、`EVEY_BACKUP_DIR` → `/data/backups`,
保存在 `evey-data` 内),或从管理面板触发一次备份。备份涵盖所有用户数据,不包括你的媒体文件。
详情:[backups.md](backups.md)。

对于被锁在外面 / 管理员丢失 / 恢复的情况,镜像内带有 `evey admin` CLI(`reset-password`、
`rotate-jwt-key`、`restore-backup`):[runbooks/recovery.md](runbooks/recovery.md)。
迁移安全:[runbooks/migration-runbook.md](runbooks/migration-runbook.md)。

## 6. 故障排查

出问题时的首要检查:

```bash
docker compose ps                       # is every service Up / healthy?
docker compose logs --tail=200 evey     # server logs (migration errors, config validation)
```

- **evey 无法启动 / 配置错误。**使用外部数据库时,如果 `EVEY_DB_PASSWORD` 仍是默认值 `evey`,
  服务器会拒绝启动;使用外部 Meilisearch 时,如果 `MEILI_MASTER_KEY` 为空或是占位符(短的
  非默认密钥只会产生警告)。阅读日志中的校验信息并修正 `.env`。
- **内嵌数据库拒绝启动。**日志顶部的 `evey:` 行会说明原因:另一个大版本的集群(见上面的升级
  一节)、不是 Postgres 集群的数据目录,或者 Postgres 自己带 `postgres:` 前缀的错误(磁盘已满、
  `./evey-data/postgres` 的权限、网络共享文件系统)。
- **重启循环,日志中出现 `/cache/...` 上的 `permission denied`。**`evey-cache` 卷属于与服务器
  运行 uid(`PUID`,或沿用的媒体所有者)不同的 uid。2026-09-17 之前的镜像只在 `./evey-data`
  的所有者也变化时才改变它的所有者,因此预先以 `PUID` 创建的数据文件夹(上面的 NAS 情形)会把
  缓存落下。请更新镜像;在旧镜像上执行一次
  `docker run --rm -v evey_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`(卷名为
  `<project>_evey-cache`)。
- **无法连接外部数据库。**确认 Postgres 健康且密码一致:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **搜索失效 / 为空。**检查 Meilisearch 的健康状况,然后重建索引 - 在管理后台的搜索设置中,
  或通过 `docker exec evey-server evey admin reindex-search`。索引由 Postgres 重建,所以删除
  `meili-data` 是安全的。
- **健康端点:**`curl -sf http://localhost:9091/healthz && echo OK`。

### 哪些卷可以安全删除

| 卷 | 内容 | 可安全删除? |
|--------|----------|-----------------|
| `evey-data` | 数据库(`postgres/`)、JWT 密钥 + 生成的密钥(`keys/`)、日志、备份、扩展、搜索索引(`meili/`) | 否 - 丢失数据、JWT 密钥 + 扩展。仅 `meili/`:是,由 `evey admin reindex-search` 重建 |
| `postgres-data` | 数据库(仅外部布局) | 否 - 丢失所有数据 |
| `evey-cache` | 转码 + trickplay 缓存 | 是 - 按需重建 |

## 7. 运维布局:外部 Postgres 和 Meilisearch

**内嵌数据库的行为。**集群位于 `./evey-data/postgres`(Postgres 17,首次启动时 `initdb`,
归服务器用户所有)。它只在容器的回环地址上监听,并在 TCP 上要求生成的密码,因此以其他 uid 降权
运行的扩展无法打开它。`docker stop` 在服务器关闭自己的连接后才到达它,作为一次有序的快速关闭。
数据目录必须位于真实的文件系统上(ext4、btrfs、xfs……)- 不能在 CIFS/NFS 共享上。备份
(`evey admin`、管理面板、定时 `pg_dump`)的工作方式与外部数据库完全相同。打开 SQL shell:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### 外部 Postgres

希望自己掌控数据库生命周期的运维者(托管 Postgres、固定的大版本、独立的卷、按自己的节奏
dump+restore)会把它作为旁边的服务运行:添加该服务并让服务器指向它 - **设置
`EVEY_DB_HOST` 本身就会关闭内嵌数据库**,这就是为什么内嵌数据库出现之前的每个安装都能原样继续
运行:

```yaml
services:
  evey:
    # ... as above, plus:
    environment:
      EVEY_DB_HOST: postgres
      EVEY_DB_SSLMODE: disable          # internal Docker network; the bundled Postgres has no TLS
    depends_on:
      postgres: { condition: service_healthy }

  postgres:
    image: postgres:16-alpine
    container_name: evey-postgres
    environment:
      POSTGRES_DB: ${EVEY_DB_NAME:-evey}
      POSTGRES_USER: ${EVEY_DB_USER:-evey}
      POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
    volumes:
      - ./postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped
```

```env
EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
```

**外部 Meilisearch,同样的思路。**独立的搜索容器拥有自己的生命周期和卷(并让你只需
`docker compose restart meilisearch` 就能重置索引)。关闭内嵌的那个,让服务器指向它,并共享
密钥 - 显式的 `MEILI_MASTER_KEY` 总是优先于生成的 `./evey-data/keys/meili.key`:

```yaml
services:
  evey:
    # ... plus:
    environment:
      EVEY_EMBEDDED_MEILI: "false"
      EVEY_MEILI_URL: http://meilisearch:7700
    depends_on:
      meilisearch: { condition: service_healthy }

  meilisearch:
    image: getmeili/meilisearch:v1.12
    container_name: evey-meilisearch
    environment:
      MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - ./meili-data:/meili_data
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
      interval: 5s
      retries: 5
    restart: unless-stopped
```

```env
MEILI_MASTER_KEY=<run: openssl rand -hex 16>
```

如果可以,给这个容器一个非 root 的 `user:` 和一个 `mem_limit` - 官方镜像以 root 运行,它的
常驻内存(内存映射的索引)看起来很大,但大部分是可回收的页缓存。索引由 Postgres 重建
(`evey admin reindex-search`),所以 `meili-data` 无论如何都可以丢弃。

只有这些环境变量属于基础设施/密钥,**留在 compose/.env 中** - 它们永远不能从管理后台修改:
`EVEY_DB_PASSWORD`、`MEILI_MASTER_KEY`、`EVEY_DB_HOST`、`EVEY_DB_USER`、`EVEY_DB_NAME`、
`EVEY_DB_SSLMODE`、`EVEY_EMBEDDED_POSTGRES`、`EVEY_EMBEDDED_MEILI`、`EVEY_MEILI_URL`、
`*_DIR` 路径、`EVEY_JWT_PRIVATE_KEY_PATH`、`EVEY_SERVER_BASE_URL`、
`EVEY_TRUSTED_PROXIES`,以及扩展总开关 `EVEY_PLUGINS_ENABLED`。
(`EVEY_PLUGINS_SANDBOX_MODE`、`EVEY_PLUGINS_UID_SEPARATION` 和资源限制可以在这里设为初始
默认值,但可在运行时于管理 → 插件中覆盖。)完整参考:[configuration.md](configuration.md)。

### 将现有安装迁移到内嵌数据库

把 Postgres 作为旁边服务运行的安装(内嵌数据库之前每个版本的布局)可以迁移到单个容器:数据只差
一份 dump,`./evey-data` 中的其余一切(JWT 密钥、扩展、备份、trickplay)都原样沿用。内嵌数据库
会在服务器运行之前,把在 `./evey-data/postgres-import.sql.gz` 找到的 dump **导入一次,导入到它
首次启动时创建的集群中** - 这样服务器自己的迁移会在其上应用,就像任何一次更新之后一样。

```bash
# 1. dump the current database (plain SQL, gzipped - what the import expects)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. write the new compose (keep a copy of the old one): the single-service
#    file from section 1, with your variations (extra media folders, the
#    proxy block) and whatever you had in `environment:` (hardware
#    transcoding: `devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME).

# 3. first boot: initdb, import, migrations, then the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. search index: an external Meilisearch's index is not carried over, rebuild it
docker exec evey-server evey admin reindex-search
```

**不再**需要的:`EVEY_DB_PASSWORD`、`MEILI_MASTER_KEY` 以及 `EVEY_DB_HOST` /
`EVEY_MEILI_URL` 的接线(密钥会自动生成)、`postgres` 和 `meilisearch` 服务,以及由 `.env`
预置的 API 密钥(它们已经在导出的 `server_config` 中)。在确认服务器正常之前保留
`./postgres-data` 和 `./meili-data`;需要回退时,旧的 compose 仍然能运行它们。留在已初始化
集群旁边的 dump 会被报告并忽略,绝不会合并;导入失败会丢弃新集群,以便下次启动时从 dump 重试。

### Postgres 大版本升级(内嵌数据库)

镜像固定了内嵌 Postgres 的大版本(目前为 17)。大版本更新的镜像会**拒绝打开**旧集群,并给出
一条指向本节的消息 - 它绝不会静默运行一个它读不了的数据目录。跨越的方法是用旧镜像 dump,
再用新镜像 restore:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# set the new EVEY_VERSION in .env, then:
docker compose up -d                              # initialises an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

偏好 `pg_upgrade` 或多年固定一个大版本的运维者使用
[外部 Postgres 布局](#7-operator-layout-external-postgres-and-meilisearch)。

## 8. ARM 主机(x86-64 和 ARM64)

稳定版(`vX.Y.Z` 标签)为 `linux/amd64` 和 `linux/arm64` 发布一个**多架构**镜像;
`docker pull`/`compose up` 会自动选择正确的那个,无需任何参数。预发布标签
(`-alpha`/`-beta`)和所有分支构建**仅限 amd64** - 在 ARM 主机(Raspberry Pi、ARM NAS、
Ampere VPS)上,请在 `EVEY_VERSION` 中固定一个稳定的 `vX.Y.Z`。

在 ARM 上有两点值得了解:

- **ARM 上的硬件转码:仅限 Rockchip,且仅限校准所证明的部分。**Evey 接入了四个加速后端 -
  Intel QSV、VAAPI、NVENC,以及 Rockchip **RKMPP**(RK3588 / RK3576 / RK3568 系列 SoC,通过
  arm64 jellyfin-ffmpeg 的 `h264_rkmpp` / `hevc_rkmpp` 编码器、`drm_prime` 解码、RGA 缩放,
  以及在存在 Mali ICD 时的 OpenCL 色调映射)。QSV 和 Intel VAAPI 驱动仅限 x86(有意不装进
  arm64 镜像);NVENC 属于 NVIDIA。要使用 RKMPP,请把 MPP 服务节点和 DRM 渲染节点映射进容器
  (`/dev/mpp_service`、`/dev/dri`,以及内核提供时的 `/dev/rga`);Evey 会检测它,运行一次真实
  的编码探测,然后在你的板子上逐个形态测量并证明后才提供 - 不会凭 SoC 名称做任何假设,未被证明
  的形态留在 CPU 上。项目尚未在真实硬件上验证 RKMPP:请把它当作尽力而为,并在**管理 → 转码 →
  编码校准**中查看哪些已被证明。Raspberry Pi 的 **V4L2 M2M** 编码器**没有**接入:在 Pi 或
  Synology 类 ARM(Realtek)NAS 上没有硬件编码路径,所以 ARM = **直接播放**(客户端原样播放
  文件,几乎零开销)加 CPU 上的**软件转码**。能力是校准出来的,不是猜的:较弱的核心保留 720p
  软件下限并假定一次只做一个编码;你仍然可以调低 `transcoding.max_video_sessions`(管理 →
  转码)并依靠直接播放。
- **在 ARM 上 HDR 色调映射同样走快速路径。**自带的 arm64 jellyfin-ffmpeg 带有 SIMD
  `tonemapx` 滤镜(NEON),就像 amd64 构建带有它的 AVX2 版本 - 因此 HDR→SDR 色调映射是硬件
  SIMD 的,而不是缓慢的 zscale 链。(Evey 在运行时探测 `tonemapx`,只有在某个构建缺少它时才
  回退到 zscale。)

如果你为未发布的架构自行构建镜像,`docker buildx build --platform linux/arm64` 可直接使用 -
Dockerfile 将 x86 专属部分放在 `TARGETARCH` 判断之后。

## 9. 扩展沙箱

扩展是在独立进程中运行的第三方代码。官方镜像包含了约束它们所需的一切 - 不需要 `--privileged`,
也不需要额外的 Docker 安全参数 - 而 `EVEY_PLUGINS_SANDBOX_MODE=auto`(默认)会选择主机允许的
最强后端,在所有现代内核上都是 Landlock。服务器本身从不以 root 运行。逐层细节(seccomp、uid
隔离、资源限制)在 [SECURITY.md](SECURITY.md) 中;本页只说明要检查什么。

首次启动后,在**管理 → 插件 → 安全仪表板**(主机沙箱卡片)中检查实际生效的沙箱:在现代内核上
`auto` 应显示 `landlock`;`basic` 表示主机无法隔离扩展的文件系统访问 - 参阅下面的部署指南。

适合你机器的设置(旧 NAS、无容器、该开/该关什么、用平实语言说明的风险):[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。完整的威胁模型和各层细节:[plugins/security-model.md](plugins/security-model.md)。
