---
source_sha: c89d50a53938f9f7a84056f270adf0482f2fbed4e8bd8d475e943e7ea09cc583
reviewed: false
---
# 高级部署

[单容器安装](../installation.md)适合大多数服务器。本页介绍管理员可能更喜欢的布局：把数据库或搜索引擎作为单独的服务、ARM 主机、镜像变体，以及在特殊主机上的插件沙箱。要完全不使用 Docker 运行 Evey，见[不使用容器](../deployment-bare-metal.md)。

## 内置数据库

数据库集群位于 `./evey-data/postgres`（Postgres 17，在首次启动时创建，归服务器用户所有）。它只在容器的环回地址上监听，并要求使用生成的密码，所以以其他用户运行的插件无法打开它。`docker stop` 会在服务器关闭连接后干净地关闭它。

数据目录必须位于本地文件系统（ext4、btrfs、xfs……）上，而不是 SMB/CIFS 或 NFS 共享上。

打开 SQL shell：

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres 主版本升级

镜像固定了内置 Postgres 的主版本（17）。带有更新主版本的镜像会拒绝打开旧集群，并给出指向这里的消息，而不是运行一个它无法读取的数据目录。迁移的办法是用旧镜像转储，再用新镜像恢复：

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # 在确认恢复成功之前保留它
# 在 compose 文件中写入新的镜像标签，然后：
docker compose up -d                              # 用新的主版本创建一个空集群
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## 外部 Postgres

要自己管理数据库的生命周期（托管的 Postgres、固定的主版本、你自己的转储策略），把它作为单独的服务运行，并让服务器指向它。**设置 `EVEY_DB_HOST` 就会关闭内置数据库。**

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... 与单容器文件相同，另外加上：
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # 内部网络，不用 TLS
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

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>   # 使用默认值 'evey' 时服务器拒绝启动
    ```

## 外部 Meilisearch

搜索引擎也是同样的原则：关闭内置的，让服务器指向你的实例，并共享密钥。显式的 `MEILI_MASTER_KEY` 优先于生成的 `./evey-data/keys/meili.key`。

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... 另外加上：
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

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

索引是从数据库重建的（`evey admin reindex-search`），所以 `meili-data` 随时都可以丢弃。

## 迁移到内置数据库

把 Postgres 作为单独服务运行的安装可以迁移到单容器。内置数据库会在首次启动时，在它创建的集群中导入一次位于 `./evey-data/postgres-import.sql.gz` 的转储，然后才运行服务器；服务器的迁移随后在其上应用。

```bash
# 1. 转储当前数据库（纯 SQL，gzip 压缩）
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. 用单容器的 compose 文件替换原文件，保留你的媒体文件夹、
#    反向代理设置和硬件相关的行。

# 3. 首次启动：创建集群、导入、迁移、启动服务器
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. 外部 Meilisearch 的索引不会被带过来：重建它
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`、`MEILI_MASTER_KEY` 以及 `EVEY_DB_HOST` / `EVEY_MEILI_URL` 设置都不再需要。在检查服务器之前保留 `./postgres-data`。留在现有集群旁边的转储会被报告并忽略，从不合并；导入失败会丢弃新集群，以便下次启动时重试。

## ARM 主机

从版本标签构建的镜像（`0.2.0`，包括 `0.2.0-beta.1` 这样的预发布版本）是多架构镜像（`linux/amd64` 和 `linux/arm64`）；Docker 会选择正确的那个。`beta-0.0.1` 这样的分支标签只有 amd64：在 ARM 主机上请固定一个版本标签。

- **ARM 上的硬件转码** 仅限于通过 RKMPP 使用的 Rockchip SoC（RK3588、RK3576、RK3568 级别）。映射 `/dev/mpp_service`、`/dev/dri`，以及内核提供时的 `/dev/rga`；Evey 会在使用编码器之前对其进行探测和基准测试。项目尚未在真实硬件上验证过 RKMPP。树莓派或基于 Realtek 的 ARM NAS 没有 Evey 可以使用的硬件编码器：这些服务器依靠直接播放和软件转码，其规模由与其他主机相同的校准来确定。
- **HDR 色调映射** 在 ARM（NEON）上使用与 x86-64（AVX2）相同的 SIMD `tonemapx` 滤镜。

## 镜像标签

镜像为 `gravityz/evey`。

- 版本标签 `vX.Y.Z` 会发布 `X.Y.Z`、`X.Y`、`X` 和 `latest`。`latest` 总是最新的版本。
- 分支构建会发布分支名称（例如 `beta-0.0.1`）和 `sha-<commit>`。
- **插件运行时。** 默认镜像为插件捆绑了 Python 3。`-nodejs` 变体加入 Node.js；`-full` 加入 PHP、Ruby 和 Lua。只有当你安装的插件需要时才使用变体。
- **GPU 系列。** 发布的镜像包含所有受支持的 Intel 和 AMD 代际的驱动。自行构建的镜像可以只保留一种：`docker build --build-arg GPU=intel-legacy .`（Intel Gen9 到 Gen11）、`GPU=intel`（Gen12 及更新）、`GPU=amd` 或 `GPU=none`。NVIDIA 不需要变体：NVIDIA Container Toolkit 会在运行时注入主机的驱动。

## 插件沙箱

镜像包含了隔离插件所需的一切，无需特权模式。默认的沙箱模式（`auto`）会选择主机允许的最强后端：在任何现代内核上都是 Landlock。在 **Admin → 插件** 的 **安全仪表板** 中检查实际生效的模式。显示 `basic` 的主机无法隔离插件的文件访问：这意味着什么以及要改什么，见[插件沙箱设置](../plugins/sandbox-deployment.md)。各层本身在[安全模型与插件沙箱](../SECURITY.md)中说明。
