---
source_sha: a60b9bf26b8f2185669ef22ab5ea9503766f3f95c501055727691026e718f7f9
reviewed: true
---
# 安装 { #install }

本页面向要为全家运行 Evey 的人。您需要一台持续开机、装好了 Docker 的电脑或 NAS。一共三步：

1. 保存一个文件，也就是 compose 文件。
2. 用一条命令启动 Evey。
3. 在浏览器里打开 Evey。

接下来，[首次设置](getting-started/first-setup.md)向导会引导您完成后续步骤。

Evey 只运行一个容器。数据库和搜索引擎都在容器里，所以不用另外安装任何东西，不用写 `.env` 文件，也不用设置内存或处理器。

!!! warning "请用本页的文件，不要用 Git 仓库里的那个"

    Git 仓库根目录下的 `docker-compose.yml` 是另一套配置，给开发者和高级安装用的：它需要一个 `.env` 文件，还会运行额外的服务。相关说明见[高级部署](operations/advanced-deployments.md)。

## 您需要准备什么 { #what-you-need }

- **一台持续开机的电脑或 NAS**，处理器是 x86-64 架构（Intel 或 AMD）。ARM64 机器需要正式发布的版本标签，目前还没有：见 [ARM 主机](operations/advanced-deployments.md#arm-hosts)。
- **带 Compose v2 的 Docker。** 命令是 `docker compose`（中间有空格），不是旧的 `docker-compose`。
- **您的媒体文件夹**：放电影、剧集、音乐或照片的地方。
- **一些磁盘空间，给 Evey 存自己的数据**（数据库、海报图片、搜索索引）。下面的 compose 文件会把这些数据放在文件旁边一个叫 `evey-data` 的文件夹里。

在家里的网络中，Evey 使用普通的 HTTP。要在家庭网络以外观看，需要 HTTPS，可以以后再添加：[远程访问与 HTTPS](administration/remote-access.md)。

## 第 1 步：保存 compose 文件 { #the-compose-file }

为 Evey 建一个文件夹，比如 `evey`，然后把下面的内容保存到里面，文件名为 `docker-compose.yml`。只需改一行：您媒体文件的路径（标着 5 的那一行）。比如您的电影在 `/srv/media`，这一行就改成 `- /srv/media:/media`。

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
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

1. Evey 的版本。`latest` 是最新的正式版本。如果想固定在某个版本，请使用 `0.1.0` 这样的版本标签，或 `sha-<commit>` 标签：见[备份与更新](backups.md#updating-evey)。
2. 端口。左边的数字就是您以后在浏览器和应用里要输入的那个。除非已经有别的程序占用了 `9091`，否则保持不变。
3. Evey 自己的数据：数据库、密钥、自动生成的密码、搜索索引、海报图片、日志、备份、插件。**需要备份的就是这个文件夹。**
4. 缓存：转换过的视频、缩放过的照片和音乐封面。随时可以删除，它会自动重新生成。海报的各种尺寸不在其中，它们位于 `evey-data/artworks`。
5. 您的媒体。把 `/path/to/your/media` 换成真实的文件夹。在 Evey 里，这个文件夹叫 `/media`。它特意设成可写，这样您要求时，[媒体健康](runbooks/media-health.md)才能修复文件。在末尾加上 `:ro` 就变成只读：Evey 就无法在其中写入，修复也会失败。
6. 容器停止时，给数据库留出时间正常关闭。Docker 默认只给 10 秒，对大型片库来说太短。

## 第 2 步：启动 Evey { #first-launch }

在终端里，进入放着这个文件的文件夹，运行：

```bash
docker compose up -d
```

Evey 会在后台启动。想看它在做什么，可以运行 `docker compose logs -f evey`；按 `Ctrl+C` 停止查看，Evey 会继续运行。

第一次启动时，Evey 会在 `evey-data` 里创建自己的密码和数据库。您什么都不用准备。

## 第 3 步：在浏览器里打开 Evey { #the-address-to-open-next }

地址是 `http://`，加上运行 Docker 那台机器的地址，再加上 `:9091`：

| 您的浏览器在哪里 | 要输入的地址 |
| --- | --- |
| 就在运行 Docker 的那台机器上 | `http://localhost:9091` |
| 家里的另一台电脑或手机上 | `http://` + 运行 Docker 那台机器的 IP 地址 + `:9091`，比如 `http://192.168.1.20:9091` |
| 在家庭网络以外 | 暂时无法访问。这需要 HTTPS 和公网地址：见[远程访问与 HTTPS](administration/remote-access.md)。 |

要查运行 Docker 那台机器的 IP 地址，可以看路由器里已连接设备的列表，或者在 Linux 上运行 `ip a`，在 Windows 上运行 `ipconfig`，在 macOS 上运行 `ifconfig`。如果您在第 1 步改了端口，请用 `ports:` 那一行左边的数字代替 `9091`。

第一次打开这个地址时，会进入设置向导。请接着看[首次设置](getting-started/first-setup.md#1-the-setup-wizard)。向导完成后，同一个地址会打开登录页面。

!!! tip "把这个地址记下来"

    家里每个人都会用到它。Android 和 Android TV 应用在第一个界面就会要这个地址，要带上 `http://` 和端口：[连接应用](getting-started/apps.md#the-server-address-to-enter)。

## 您的数据放在哪里 { #where-the-data-lives }

| 位置 | 里面有什么 | 能删吗？ |
| --- | --- | --- |
| `./evey-data` | 数据库、密钥、自动生成的密码、搜索索引、进度条缩略图、海报图片、日志、备份、插件 | **不能。** 丢了它，所有账户、观看历史和设置都会丢失。唯一的例外是里面的 `meili/`，也就是搜索索引：命令 `evey admin reindex-search` 可以根据数据库重建它。 |
| `evey-cache` 卷 | 转换过的视频、缩放过的照片和音乐封面（海报的各种尺寸位于 `evey-data/artworks`） | 能。它会自动重新生成。 |
| 您的媒体文件夹 | 您的文件 | Evey 从不删除它们。在 Evey 里删除一部影片，只是把它从媒体库移除，不会从硬盘上删除。 |

Evey 只在三种情况下向您的媒体文件夹写入内容：

- 您在**媒体健康**里发起的修复（原文件会保留作为备份）；
- 从 OpenSubtitles 下载的字幕（保存在电影旁边的 `.subtitles` 文件夹里）；
- 一个小测试文件，用来确认 Evey 还能发现新文件，写完马上删除。

## 常见改动 { #common-variations }

下面每一项都只是在同一个文件里改几行。

**第二个媒体文件夹。** 在 `volumes:` 下面加一行 `- /other/path:/media2`。第三个就是 `/media3`，以此类推。

**同一台机器上再跑一个 Evey。** 请给它另一个 `container_name` 和另外的主机端口，否则 `docker compose up` 会拒绝启动（名称已被占用）。

**换个端口。** 只改左边的数字，比如 `"8080:9091"`。之后在浏览器和应用里输入 `:8080`。

**文件归谁所有。** 默认情况下，Evey 会使用您媒体文件夹的所有者身份，这样修复时才能写入。如果想自己指定，就在 `environment:` 下面加上 `PUID` 和 `PGID`，值用 `id -u` 和 `id -g` 给出的数字。容器只是为了应用这个设置才以 root 启动；Evey 本身从不以 root 身份运行。

**用显卡转换视频。** 让容器能访问显卡：见[硬件加速](administration/hardware-acceleration.md)。

**您运行哪个版本。** 由 `image:` 这一行决定：

| `image:` | 您得到什么 |
|---|---|
| `eveyproject/evey:latest` | 最新的正式版本，每次 `docker compose pull` 都会升到下一个版本 |
| `eveyproject/evey:0.1` | 该次版本线上的最新修复（0.1.0、0.1.1、...），不会升到新的次版本 |
| `eveyproject/evey:0.1.0` | 恰好是这个版本，直到您修改这一行 |
| `eveyproject/evey:sha-<commit>` | 某一个确切的构建，是唯一永不变动的标签 |
| `eveyproject/evey:master` | 开发分支，每次推送都会重新构建：用于测试，不适合您依赖的服务器 |

版本号遵循[语义化版本](https://semver.org/)，从 `0.1.0` 开始；0.x 系列是公开测试版。要更换版本，修改这一行，然后运行 `docker compose pull && docker compose up -d`；您的数据仍保存在 `./evey-data`。当前运行的版本可在 **管理 → 仪表盘**、`/healthz` 以及 `docker exec evey-server evey version` 中看到。每个镜像也有 `-nodejs` 和 `-lean` 版本，是插件运行时更少的精简构建（[详情](operations/advanced-deployments.md#image-tags-and-variants)）。

**您已经在用反向代理**（一个在您的服务前面加上 HTTPS 的程序）：见[远程访问与 HTTPS](administration/remote-access.md)。

??? note "在 NAS 上（Synology Container Manager、QNAP Container Station）"

    - 第一次启动前，先建好 `evey-data` 文件夹。和 Docker 命令行不同，这些界面无法挂载不存在的文件夹。其他都不用建。
    - 把 `PUID` 和 `PGID` 设成拥有媒体文件的那个 NAS 用户（通过 SSH 运行 `id <user>` 查看；Synology 上通常在 `1026` 左右，用户组 `100`）。NAS 的共享文件夹常常归 `root` 所有，而 Evey 绝不会自动使用 root 身份。
    - 带 Quick Sync 的 Intel NAS：按[硬件加速](administration/hardware-acceleration.md)里的说明，加上 `devices:` 和 `group_add:` 这几行。

## Evey 自动完成的设置 { #what-evey-sets-up-by-itself }

这些都不用您配置：

- **服务器能同时转换多少视频。** 它会在第一次空闲时测量这台机器。
- **显卡。** Evey 会检测您提供给容器的显卡，并用一次真实的转换来检验它。
- **每台设备怎么播放文件。** 每次播放时决定：见[Evey 如何选择播放方式](playback/how-playback-is-decided.md)。
- **从家庭网络以外访问。** 默认关闭，直到您给服务器设置公网地址。

以后可能要改的设置（服务器名称、计划任务时间、各种限制、API 密钥）都在管理面板里，不在 compose 文件里。为数不多的环境变量列在[配置参考](configuration.md)里。
