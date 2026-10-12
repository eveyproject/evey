---
source_sha: 88e0bffcb0012a56a1138eec253a0e443e751536aff3d4d36eb018d38b7a86e9
reviewed: true
---
# 插件沙箱设置

用简单的话说明哪种沙箱设置适合您的服务器。对几乎所有人来说，答案都是：保持默认设置。
服务器会检测这台机器，并选择它能做到的最强隔离。只有当您用的是旧款 NAS、不用 Docker 运行 Evey，
或者想把安全收得更紧时，才需要往下读。完整的技术说明见[插件沙箱与威胁模型](../SECURITY.md)。

## 是什么在保护您

您安装的插件会被关在一个“盒子”里。根据机器的不同，这个盒子：

- **限制它能碰的文件**：只有它自己的文件夹和它需要的系统库，别的都不行（碰不到您的媒体、您的密钥，也碰不到其他插件）；
- 在内核层面**阻止危险的系统操作**：调试其他进程、加载模块、修改时钟、挂载磁盘；
- 始终**切断直接的网络访问**：一个申请了网络权限、并且您批准了的插件，也只能经过服务器访问它声明过的主机；
- **限制 CPU、内存、磁盘和进程数量**，这样失控的插件不会拖垮服务器；
- **让您的密钥无法被读取**：用来给登录签名的密钥，以及备份。

除此之外，只有管理员才能安装插件，插件会被扫描是否有可疑代码，而且在它运行之前，您要先批准它的权限。

**查看实际生效的保护：** **管理 -> 插件 -> 安全仪表板**会显示主机沙箱，也就是 `auto` 最终选择的模式
（`landlock`、`bubblewrap` 或 `basic`），以及各项检测的结果（**Landlock 内核**、**沙箱启动器**、**Bubblewrap**）。
`basic` 表示主机无法按路径隔离文件：插件只靠切换用户（uid）和 seccomp 运行，检测结果会说明原因，下面几节会说明要改什么。

## 找到您的情况

| 您的服务器 | 不用设置就能得到什么 | 需要做什么吗？ |
|---|---|---|
| **在 Docker 里运行的现代 Linux**（内核 5.13 或更新，大致是 2021 年以后的系统），最常见的情况 | 自动得到最强的隔离（Landlock） | 不用 |
| **在 Docker 里运行的旧款 NAS**（内核比 5.13 旧） | 良好的隔离，只是没有按文件夹划分的文件“墙”；密钥、网络和系统操作仍然被锁住 | 不用：uid 分离（见下文）会自动开启 |
| **任意 Linux，不用 Docker** | 能得到的最强隔离：内核较新时用 Landlock，否则用 bubblewrap 命名空间 | 按照[不使用容器](../deployment-bare-metal.md)操作 |

## 您可能会动的唯一一个开关：uid 分离

uid 分离让插件以一个和服务器不同的用户账户运行，这样，即使没有别的东西拦住它，它在物理上也读不到服务器的机密文件。
它只在没有按文件夹划分文件墙的机器上才重要（也就是 `basic` 的情况）；在现代机器上，文件墙已经让密钥够不着，
uid 分离只是一层多余的额外防护。

`EVEY_PLUGINS_UID_SEPARATION`：

| 值 | 含义 | 什么时候用 |
|---|---|---|
| `auto`（默认） | 只在需要时开启（`basic` 的情况），在多余的地方关闭 | 一直用它，除非遇到下面两种情况之一 |
| `on` | 始终开启，在文件墙之上再加一道独立的墙 | 您想要双重保险，并接受下面说的小代价 |
| `off` | 从不开启 | 某个插件在 uid 分离下运行不正常（很少见） |

!!! warning
    uid 分离和 Docker 的 `no-new-privileges` 选项不能同时使用：只能二选一。在旧机器上（`basic`），
    如果无法切换用户，服务器宁可不启动这个插件，也不会让它在您的密钥旁边运行；在现代机器上，
    服务器会记录一条警告，然后在没有这道额外墙的情况下运行插件。镜像里已经带好了所需的一切，您只需要设置这个变量。

`on` 的小代价：插件以一个单独的账户运行，所以在自己文件夹里写临时文件的插件可能会出问题。
如果遇到这种情况，请设为 `off`。这就是默认值为 `auto` 的原因：它只在真正有好处的地方才承担这个风险。

## 恶意插件能做什么

**现代 Linux（Docker 或裸机）：** 坏插件被完全关在盒子里。它读不到您的密钥，看不到也碰不到其他插件的文件，
不能随意发起网络请求，也不能篡改系统。唯一现实的逃出办法是内核里的严重漏洞，任何不是完整虚拟机的沙箱都是如此。

**在 Docker 里运行、没有按文件夹划分文件墙的旧款 NAS（`basic`）：**

- 您的密钥（登录密钥、备份）仍然受到保护，靠的是单独的用户账户，`auto` 在这种情况下会把它开启；
- 插件不能直接上网；获准上网的插件也只能访问它声明过的主机；
- 危险的系统操作仍然在内核层面被阻止；
- 但由于没有按文件夹划分的墙，坏插件可以读取普通的、不机密的文件，列出正在运行的进程，
  而且因为所有插件共用一个账户，它还能读取或干扰另一个插件的文件。

在这样的主机上，请只安装您信任的插件，或者换用较新的内核，以获得完整的文件墙。

## 配置示例

这些写在您的 `.env`（compose）里，或者作为 `EVEY_*` 环境变量设置。

```bash title="1. 现代服务器，推荐做法（什么都不用做）"
EVEY_PLUGINS_SANDBOX_MODE=auto          # 选择能用的最强方式（默认）
# EVEY_PLUGINS_UID_SEPARATION=auto      # （默认）只在需要的地方开启
```

```bash title="2. 旧款 NAS（什么都不用设置，列出来只是为了说清楚）"
# 不要在 compose 的 security_opt 里加 no-new-privileges，它会阻止切换用户
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # （默认）在 basic 的情况下开启
```

```bash title="3. 在现代服务器上加双重保险"
EVEY_PLUGINS_SANDBOX_MODE=auto          # 最终选择 landlock
EVEY_PLUGINS_UID_SEPARATION=on          # 再加上独立的 uid 墙
```

```bash title="4. 固定一种模式（高级）"
EVEY_PLUGINS_SANDBOX_MODE=landlock      # 如果 Landlock 不可用，就拒绝启动插件
```

```bash title="5. 某个插件在 uid 分离下出问题"
EVEY_PLUGINS_UID_SEPARATION=off
```

```bash title="6. 资源上限（之后可以在 管理 -> 插件 -> 资源限制 里修改）"
EVEY_PLUGINS_MEMORY_MB=256              # 每个插件的最大内存
EVEY_PLUGINS_PIDS_MAX=64                # 最大进程和线程数
EVEY_PLUGINS_DISK_MAX_MB=100            # 安装目录的最大大小（0 = 关闭）
EVEY_PLUGINS_CPU_NICE=10                # 调度优先级 0-19（越大越谦让）
```

## 运行中更改沙箱模式

在**管理 -> 插件**的**资源限制**卡片里修改**沙箱模式**。正在运行的插件会自动重启，让新模式立即生效，
**安全仪表板**会显示实际生效的模式。无法在新模式下运行的插件（例如在无特权容器里使用 `bubblewrap`）会显示为出错：这时请改回原来的模式。

## 确认它在工作

```bash
# 启动器带有 uid 分离所用的能力（capability）
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# 正在运行的插件以哪个用户运行（evey = 和服务器共用，
# evey-plugin = 已分离）；镜像里没有 ps，所以问 Docker
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

实际生效的沙箱模式，就是**管理 -> 插件 -> 安全仪表板**上的标记。

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/en/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
