---
source_sha: a6c61f9f7b1e619f46c4d5eea7ca2c2b93356f93c04681e95963303bdf287723
reviewed: false
---
# 安装

Evey 以一个容器运行。镜像在一个端口上同时提供 API 和网页界面，并在容器内运行自己的数据库（Postgres）和搜索引擎（Meilisearch），它们的密码在首次启动时生成。下面的 compose 文件就是全部的安装：不需要 `.env`，不需要特权模式，不需要内存或 CPU 设置。

!!! warning "不是仓库里的 docker-compose.yml"

    Git 仓库根目录下的 `docker-compose.yml` 是另一种布局（Postgres 作为单独的服务、自带 Caddy、必须有 `.env`）。请使用本页的文件。另一种布局在[高级部署](operations/advanced-deployments.md)中说明。

## 需求

- 带 Compose v2 的 Docker（`docker compose`，而不是旧的 `docker-compose`）。
- 一块用来存放 Evey 数据的持久磁盘（下面的 compose 文件把数据放在文件旁边的 `./evey-data` 中）。
- 一台 x86-64 主机，或者使用版本标签的 ARM64 主机：见[ARM 主机](operations/advanced-deployments.md#arm-hosts)。

Evey 提供未加密的 HTTP。要在家庭网络之外通过 HTTPS 访问它，之后在它前面放一个反向代理：见[远程访问与 HTTPS](administration/remote-access.md)。

## compose 文件

把它保存为 `docker-compose.yml`，并替换最后一行卷中的媒体路径。

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. `latest` 是最新的稳定版本。真正开始使用 Evey 后请固定一个标签，这样相隔一天的两次拉取运行的是同样的代码：见[备份与更新](backups.md#updating-evey)。
2. API 和网页界面，在一个端口上，使用未加密的 HTTP。
3. 属于 Evey 的一切：数据库、签名密钥、生成的密码、搜索索引、时间轴缩略图、图片、日志、备份、插件。这是需要备份的文件夹。
4. 转码缓存：分段、缩放后的照片和封面。可以随时删除，需要时会重新生成。
5. 你的媒体。特意以读写方式挂载：[媒体健康](runbooks/media-health.md)的修复会就地改写文件。加上 `:ro` 可得到只读媒体库，那些修复就会失败。更多文件夹挂载为 `/media2`、`/media3`……
6. 在 `docker stop` 时给内置数据库留出干净停止的时间。Docker 默认的 10 秒对大型片库来说太短。

## 首次启动

```bash
docker compose up -d
docker compose logs -f evey
```

首次启动时，服务器在 `./evey-data` 中生成签名密钥和密码，创建数据库并执行迁移。不需要手动准备任何东西。

然后打开 `http://<server-address>:9091`。还没有管理员的服务器会打开设置向导：继续阅读[首次设置](getting-started/first-setup.md)。

## 数据存放在哪里

| 位置 | 内容 | 可以删除吗？ |
| --- | --- | --- |
| `./evey-data` | 数据库、签名密钥、生成的密码、搜索索引、时间轴缩略图、图片、日志、备份、插件 | 不可以。丢失它就会丢失所有账户、观看记录和设置。只有其中的 `meili/` 可以删除：搜索索引可以用 `evey admin reindex-search` 从数据库重建。 |
| `evey-cache` 卷 | 转码后的分段、缩放后的照片和封面 | 可以。需要时会重新生成。 |
| 你的媒体文件夹 | 你的文件 | Evey 不会删除它们；在 Evey 中删除一个条目只会把它从媒体库中移除，而不会从磁盘上删除。它只在三种情况下写入这里：从媒体健康执行的修复（原文件作为备份保留）、从 OpenSubtitles 下载的字幕（电影旁边的 `.subtitles` 文件夹），以及一个几秒钟的探测文件，用于检查文件夹监视器仍能收到事件。 |

## 常见变体

每一种都只是在同一个文件里加几行。

**第二个媒体文件夹。** 在 `volumes:` 下添加 `- /other/path:/media2`。

**文件所有权。** 默认情况下，容器采用你的 `/media` 挂载的所有者，以便修复可以写入。要自己选择，在 `environment:` 下设置 `PUID` 和 `PGID`（即 `id -u` 和 `id -g` 的输出）。容器只为应用这个映射而以 root 启动；服务器本身从不以 root 运行。

**用于转码的显卡。** 把设备映射进容器：见[硬件加速](administration/hardware-acceleration.md)。

**在你已有的反向代理之后。** 见[远程访问与 HTTPS](administration/remote-access.md)。

??? note "在 NAS 上（Synology Container Manager、QNAP Container Station）"

    - 在首次启动前创建 `evey-data` 文件夹。与 Docker 命令行不同，这些界面会拒绝文件夹不存在的绑定挂载。其他什么都不需要创建。
    - 把 `PUID` 和 `PGID` 设置为拥有媒体的 NAS 用户（通过 SSH 执行 `id <user>`；在 Synology 上通常在 `1026` 左右，组为 `100`）。NAS 共享文件夹常常属于 `root`，而自动检测从不采用 root。
    - 带 Quick Sync 的 Intel NAS：添加[硬件加速](administration/hardware-acceleration.md)中描述的 `devices:` 和 `group_add:` 行。

## 不需要配置的东西

转码容量、显卡、每台设备的播放方式以及远程访问都由服务器决定：它在第一个空闲时刻测量机器，检测你映射的硬件，并在你给它一个公网地址时开启远程访问。之后可能改变的设置（服务器名称、计划、限制、API 密钥）在管理面板中，而不在 compose 文件里。确实存在的环境变量列在[配置参考](configuration.md)中。
