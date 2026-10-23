---
source_sha: f0e5f75d4befda29fbffbe7da80a4cc45e7a86e9429753abdee557b19bc5a40b
reviewed: true
---
# 高级部署 { #advanced-deployments }

[安装](../installation.md)页面介绍的单容器安装适合大多数服务器。在以下情况下请看这一页：
您想把数据库或搜索引擎作为单独的服务运行，主机是 ARM，您需要某个镜像变体，或者插件沙箱报告的不是 `landlock`。
如果想完全不用 Docker 运行 Evey，请看[不使用容器](../deployment-bare-metal.md)。

## 内置数据库 { #the-embedded-database }

数据库集群放在 `./evey-data/postgres` 里（Postgres 17，第一次启动时创建，归服务器用户所有）。
它只监听容器内部的本机回环地址，并且需要自动生成的密码，所以以其他用户运行的插件无法打开它。
`docker stop` 会在服务器关闭它的连接之后，正常关闭数据库。

数据目录必须在本地文件系统上（ext4、btrfs、xfs……），不能放在 SMB/CIFS 或 NFS 共享上。打开 SQL 命令行：

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### 升级 Postgres 大版本 { #move-to-a-newer-postgres-major }

镜像固定了内置数据库的大版本。如果新镜像带的是更新的大版本，它会拒绝打开旧的集群，并给出一条指向这里的提示，
而不是去运行一个它无法读取的数据目录。升级的办法是：用旧镜像导出（dump），再用新镜像恢复：

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # 在确认恢复成功之前，先保留它
# 在 compose 文件里填上新的镜像标签，然后：
docker compose up -d                              # 用新的大版本创建一个空集群
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## 把 Postgres 作为单独的服务运行 { #run-postgres-as-a-separate-service }

如果您想自己管理数据库的生命周期（托管的 Postgres、固定的大版本、您自己的导出策略），
可以把它作为单独的服务运行，并让服务器连接它。设置了 `EVEY_DB_HOST`，内置数据库就会关闭。
如果密码是默认的 `evey`，服务器会拒绝启动。

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ……和单容器文件一样，再加上：
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
    EVEY_DB_PASSWORD=<strong-password>
    ```

## 把 Meilisearch 作为单独的服务运行 { #run-meilisearch-as-a-separate-service }

搜索引擎也是同样的道理：关闭内置的那个，让服务器连接您的搜索引擎，并共用同一个密钥。
明确设置的 `MEILI_MASTER_KEY` 优先于自动生成的 `./evey-data/keys/meili.key`。

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ……再加上：
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

索引可以用 `docker exec evey-server evey admin reindex-search` 根据数据库重建，所以 `meili-data` 随时都可以删除。

## 改回内置数据库 { #move-back-onto-the-embedded-database }

把 Postgres 作为单独服务运行的安装，可以改回单容器。内置数据库会在第一次启动、服务器运行之前，
把 `./evey-data/postgres-import.sql.gz` 里的导出文件导入到它新建的集群里，只导入一次；然后再在上面执行服务器的迁移。

```bash
# 1. 导出当前的数据库（纯 SQL，用 gzip 压缩）
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. 把 compose 文件换成单容器的版本，保留您的
#    媒体文件夹、反向代理设置和硬件相关的几行。

# 3. 第一次启动：创建集群、导入、迁移、启动服务器
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. 外部 Meilisearch 的索引不会带过来：重建它
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`、`MEILI_MASTER_KEY` 以及 `EVEY_DB_HOST` / `EVEY_MEILI_URL` 设置都不再需要了。
在检查过服务器之前，请保留 `./postgres-data`。如果已有集群旁边放着一个导出文件，它会被报告并忽略，绝不会合并进去；
导入失败时，新建的集群会被丢弃，这样下次启动会重新从导出文件开始。

## ARM 主机 { #arm-hosts }

从发布标签构建的镜像（`0.1.0`，也包括预发布版本）支持多种架构（`linux/amd64` 和 `linux/arm64`）；
Docker 会自动选择对应的架构。分支标签只有 amd64：在 ARM 主机上请固定使用发布标签。

- **ARM 上的硬件转码**只支持 Rockchip 芯片（RK3588、RK3576、RK3568 这一类），通过 RKMPP 实现。
  请映射 `/dev/mpp_service`、`/dev/dri`，如果内核提供的话，再映射 `/dev/rga`；服务器在使用编码器之前，会先探测并测试它的性能。
  本项目还没有在真实硬件上验证过 RKMPP。树莓派或基于 Realtek 的 ARM NAS 没有服务器能用的硬件编码器：
  这些主机依靠直接播放和软件转码，能力由和其他主机一样的校准来确定。
- **HDR 色调映射**在 ARM 上（NEON）和在 x86-64 上（AVX2）使用同一个 SIMD `tonemapx` 滤镜。

## 镜像标签和变体 { #image-tags-and-variants }

镜像是 `eveyproject/evey`。

| 标签 | 由什么发布 | 说明 |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | 发布标签 `vX.Y.Z` | `latest` 永远是最新的稳定版 |
| 预发布标签（带连字符） | 预发布标签 | 只发布它自己的标签，从不改动 `latest` |
| 分支名，`sha-<commit>` | 分支构建 | 只有 amd64 |

每个标签都有三种变体：默认的变体带有全部插件运行时（Python 3、Node.js、PHP、Ruby 和 Lua），`-nodejs` 保留 Python 和 Node.js，`-lean` 只保留 Python
（例如 `latest-lean`）。只有在镜像体积要紧时才选用更精简的变体：如果插件声明的语言在镜像里没有，安装程序会拒绝它，并在提示里写明该用默认镜像。

发布的镜像带有所有受支持的 Intel 和 AMD 显卡世代的驱动。自己构建镜像时，可以只保留一个系列：
`docker build --build-arg GPU=intel-legacy .`（Intel 第 9 代到第 11 代）、`GPU=intel`（第 12 代及更新）、`GPU=amd` 或 `GPU=none`。
NVIDIA 不需要变体：NVIDIA Container Toolkit 会在运行时注入主机的驱动。

## 在特殊主机上的插件沙箱 { #the-plugin-sandbox-on-an-unusual-host }

镜像里带有隔离插件所需的一切，不需要特权模式。默认的沙箱模式（`auto`）会选择主机允许的最强后端：
在任何现代内核上都是 Landlock。实际生效的模式可以在**管理 → 插件**的**安全仪表板**里查看。
显示 `basic` 的主机无法隔离插件对文件的访问：[插件沙箱设置](../plugins/sandbox-deployment.md)说明了这意味着什么，以及要改什么。
各层防护本身见[安全模型与插件沙箱](../SECURITY.md)。

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/en/admin.yaml - plugins.security.title
-->
