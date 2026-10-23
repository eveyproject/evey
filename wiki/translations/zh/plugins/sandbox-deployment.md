---
source_sha: 9c17a56bd4379a8c15fd8d637fe0c4914e9b9cf848503ddd72e806f574506caa
reviewed: true
---
# 插件沙箱设置 { #plugin-sandbox-setup }

本页用通俗的语言说明哪种沙箱设置适合您的服务器。对几乎所有人来说，答案都是：保持默认设置。
服务器会检测这台机器，并选择它能提供的最强隔离。只有当您使用旧款 NAS、不使用 Docker 运行 Evey，
或者需要更强的隔离时，才需要继续阅读。完整的技术说明见[安全模型与插件沙箱](../SECURITY.md)。

## 保护您的措施 { #what-protects-you }

您安装的插件在隔离环境中运行。根据机器的不同，这种隔离：

- **限制它能访问的文件**：只能访问它自己的文件夹和它需要的系统库，不能访问其他任何文件（不能访问您的媒体、您的密钥，也不能访问其他插件）；
- 在内核层面**阻止危险的系统操作**：调试其他进程、加载模块、修改时钟、挂载磁盘；
- 始终**切断直接的网络访问**：一个申请了网络权限、并且您批准了的插件，也只能经过服务器访问它声明过的主机；
- **限制 CPU、内存、磁盘和进程数量**，这样占用资源过多的插件不会导致服务器停止运行；
- **让您的密钥无法被读取**：用来给登录签名的密钥，以及备份。

除此之外，只有管理员才能安装插件，插件会被扫描是否有可疑代码，而且在它运行之前，您要先批准它的权限。

**查看实际生效的保护：** **管理 → 插件 → 安全仪表板**会显示主机沙箱，也就是 `auto` 最终选择的模式
（`landlock`、`bubblewrap` 或 `basic`），以及各项检测的结果（**Landlock 内核**、**沙箱启动器**、**Bubblewrap**）。
`basic` 表示主机无法按路径隔离文件：插件只靠切换用户（uid）和 seccomp 运行，检测结果会说明原因，下面几节会说明要改什么。

## 找到您的情况 { #pick-your-situation }

| 您的服务器 | 默认提供的保护 | 需要做什么吗？ |
|---|---|---|
| **在 Docker 里运行的现代 Linux**（内核 5.13 或更新，大致是 2021 年以后的系统），最常见的情况 | 自动得到最强的隔离（Landlock） | 不用 |
| **在 Docker 里运行的旧款 NAS**（内核比 5.13 旧） | 良好的隔离，但没有按文件夹的文件限制；密钥、网络和系统操作仍然受到严格限制 | 不用：uid 分离（见下文）会自动开启 |
| **任意 Linux，不用 Docker** | 能得到的最强隔离：内核较新时用 Landlock，否则用 bubblewrap 命名空间 | 按照[不使用容器](../deployment-bare-metal.md)操作 |

## 您可能需要修改的唯一设置：uid 分离 { #the-one-setting-you-might-change-uid-separation }

uid 分离让插件以一个和服务器不同的用户账户运行，这样，即使其他机制都没有阻止它，它也无法读取服务器的机密文件。
它只在没有按文件夹的文件限制的机器上才有意义（也就是 `basic` 的情况）；在现代机器上，这种文件限制已经使密钥无法被访问，
uid 分离只是一层冗余的额外防护。

`EVEY_PLUGINS_UID_SEPARATION`：

| 值 | 含义 | 什么时候用 |
|---|---|---|
| `auto`（默认） | 只在需要时开启（`basic` 的情况），在冗余时关闭 | 始终使用，除非属于下面两种情况之一 |
| `on` | 始终开启，在文件限制之上再加一层独立的保护 | 您希望同时使用两种保护，并接受下面所述的小代价 |
| `off` | 从不开启 | 某个插件在 uid 分离下运行不正常（很少见） |

!!! warning
    uid 分离和 Docker 的 `no-new-privileges` 选项不能同时使用：只能二选一。在旧机器上（`basic`），
    如果无法切换用户，服务器会拒绝启动该插件，而不是让它在能接触到您密钥的环境中运行；在现代机器上，
    服务器会记录一条警告，然后在没有这层额外保护的情况下运行插件。镜像里已经带好了所需的一切，您只需要设置这个变量。

`on` 的小代价：插件以一个单独的账户运行，所以在自己文件夹里写临时文件的插件可能会运行失败。
如果遇到这种情况，请设为 `off`。这就是默认值为 `auto` 的原因：它只在真正有好处的地方才承担这个风险。

## 恶意插件能做什么 { #what-a-malicious-plugin-could-do }

**现代 Linux（Docker 或裸机）：** 恶意插件被完全隔离。它读不到您的密钥，看不到也碰不到其他插件的文件，
不能随意发起网络请求，也不能篡改系统。唯一现实的突破方式是内核中的严重漏洞，任何不是完整虚拟机的沙箱都是如此。

**在 Docker 里运行、没有按文件夹的文件限制的旧款 NAS（`basic`）：**

- 您的密钥（登录密钥、备份）仍然受到保护，靠的是单独的用户账户，`auto` 在这种情况下会把它开启；
- 插件不能直接上网；获准上网的插件也只能访问它声明过的主机；
- 危险的系统操作仍然在内核层面被阻止；
- 但由于没有按文件夹的文件限制，恶意插件可以读取普通的、非机密的文件，列出正在运行的进程，
  而且因为所有插件共用一个账户，它还能读取或干扰另一个插件的文件。

在这样的主机上，请只安装您信任的插件，或者换用较新的内核，以获得完整的文件限制。

## 配置示例 { #configuration-examples }

每一行都写在 compose 文件的 `environment:` 下，格式为 `KEY: value`（例如 `EVEY_PLUGINS_UID_SEPARATION: "on"`）。

```bash title="1. 现代服务器，推荐做法（什么都不用做）"
EVEY_PLUGINS_SANDBOX_MODE=auto          # 选择能用的最强方式（默认）
# EVEY_PLUGINS_UID_SEPARATION=auto      # （默认）只在需要的地方开启
```

```bash title="2. 旧款 NAS（什么都不用设置，列出来只是为了说清楚）"
# 不要在 compose 的 security_opt 里加 no-new-privileges，它会阻止切换用户
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # （默认）在 basic 的情况下开启
```

```bash title="3. 在现代服务器上同时使用两种保护"
EVEY_PLUGINS_SANDBOX_MODE=auto          # 最终选择 landlock
EVEY_PLUGINS_UID_SEPARATION=on          # 再加上独立的 uid 分离
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

## 运行中更改沙箱模式 { #change-the-sandbox-mode-while-running }

在**管理 → 插件**的**资源限制**卡片里修改**沙箱模式**。正在运行的插件会自动重启，让新模式立即生效，
**安全仪表板**会显示实际生效的模式。无法在新模式下运行的插件（例如在无特权容器里使用 `bubblewrap`）会显示为出错：此时请改回原来的模式。

## 确认沙箱正常工作 { #verify-it-is-working }

```bash
# 启动器带有 uid 分离所用的能力（capability）
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# 正在运行的插件以哪个用户运行（evey = 和服务器共用，
# evey-plugin = 已分离）；镜像里有 ps，所以 `docker exec evey-server ps` 也可以
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

实际生效的沙箱模式显示在**管理 → 插件 → 安全仪表板**的标记中。

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/en/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
