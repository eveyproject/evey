---
source_sha: 0846a653f87edb54addf042a0876b46fd3ab9e42930e3f12e27706e59ce0bc97
---
# 扩展沙箱 - 你的服务器该用哪种配置

Evey 在沙箱中运行第三方扩展。**对几乎所有人来说，答案都是：保持默认。** 服务器会检测你的
机器，并自动选用它能提供的最强隔离。只有当你用的是**老旧 NAS**、**不用 Docker** 运行 Evey，
或者想**进一步收紧**时，才需要往下读。

完整的技术模型见 [SECURITY.md](../SECURITY.md)。本页是用大白话写的决策指南。

---

## 是什么在保护你（大白话版）

你安装的扩展被关在一个盒子里。根据你的机器，这个盒子会：

- **限制它能碰的文件** - 它只能看到自己的文件夹和运行所需的系统库，别的都看不到（看不到
  你的媒体、你的密钥，也看不到其他扩展）。
- **阻止危险的系统操作** - 调试其他进程、加载内核模块、修改时钟、挂载磁盘等，都会被内核
  拒绝。
- **切断网络**，除非扩展明确申请了网络（并且你批准了这项权限）。
- **限制 CPU、内存、磁盘和进程数**，免得一个失控的扩展拖垮服务器。
- **让你的密钥无法被读取** - 用于签署登录的密钥和备份。

此外：扩展**只有管理员能安装**，会被**扫描**是否含有可疑代码，并且在运行前由你**批准它们的
权限**。

**查看实际生效的情况：**管理 → Plugins → Security Dashboard 显示*主机沙箱* - 即 `auto`
最终解析出的模式（`landlock`、`bubblewrap` 或 `basic`）以及每项探测的结果。如果显示
**basic**，说明你的主机不提供按路径的文件系统隔离（扩展只靠切换 UID + seccomp 运行）；
探测结果会告诉你原因，下面的各节会告诉你怎么解决。

---

## 选择你的情况

| 你的服务器 | 开箱即得 | 需要做什么吗？ |
|---|---|---|
| **Docker 中的现代 Linux**（内核 ≥ 5.13 - 大约 2021 年以后的基本都是）- *常规情况* | **最强隔离**（Landlock），自动启用。 | **什么都不用做。** 保持默认。 |
| **Docker 中的老旧 NAS**（如较老的群晖，内核 < 5.13） | 隔离良好，**但缺少按文件夹划分的文件墙**。密钥、网络和系统操作仍被锁死。 | 可选：开启 **UID 隔离**（见下文），给密钥再加一重保护。 |
| **任意 Linux，不用 Docker**（裸机） | **最强隔离**，自动启用（现代内核上用 Landlock，否则用 bubblewrap 命名空间）。 | 参照[不使用容器的部署指南](../deployment-bare-metal.md)。 |

如何知道你属于哪种：打开**管理 → Plugins** - 安全面板上的徽章显示实际生效的沙箱模式：
`landlock` 或 `bubblewrap` 表示强隔离，`basic` 表示"老旧 NAS"的情况。

---

## 你可能会动的唯一开关：UID 隔离

**它做什么（大白话）：**让扩展以一个与 Evey 本身**不同的用户账户**运行，于是它在物理上就
读不了 Evey 的机密文件 - 即使没有别的东西拦着它。

**什么时候有用：**只在**没有按文件夹划分文件墙的老机器**上（即 `basic` 的情况）。在现代
机器上，文件墙已经让密钥可望而不可即，所以那里的 UID 隔离只是一层多余的额外保护。

**设置项** - `EVEY_PLUGINS_UID_SEPARATION`：

| 值 | 含义 | 适用场景 |
|---|---|---|
| `auto` *（默认）* | **只在真正需要时开启**（`basic` 的情况），多余的地方就关闭。 | **保持这样就行** - 它会在每台机器上做对的事。 |
| `on` | **始终开启**，现代机器上也一样 - 在文件墙之外，再给密钥加一道独立的墙。 | 你想要最大程度的双保险，并接受下面提到的小代价。 |
| `off` | **从不开启。** | 某个特定扩展在它下面行为异常（少见）。 |

**要注意的坑：**UID 隔离与 Docker 的加固选项 `no-new-privileges` **互不兼容** - 二选一。
如果你的环境在老机器（`basic`）上无法授予它，Evey 会**拒绝启动该扩展**，而不是让它挨着你的
密钥运行；在现代机器上，它会记录一条警告并照常运行扩展（你只是少了那道额外的墙）。官方镜像
已经自带所需的一切；你只需设置这个变量。

**`on` 的小代价：**扩展以单独的账户运行，所以一个在*自己文件夹内*写临时文件的扩展可能会
出问题。遇到这种情况，就设为 `off`。（这也是默认值为 `auto` 的原因 - 它只在确有好处的地方
承担这个风险。）

---

## 风险（大白话版）

一个**恶意**扩展能做什么、不能做什么 - 按场景来看：

**现代 Linux（Docker 或裸机）：**
坏扩展被完全关住。它**不能**读取你的密钥，**不能**查看或触碰其他扩展的文件，**不能**任意
发起网络请求，也**不能**篡改系统。唯一现实的出口是 Linux **内核**的严重漏洞 - 这一点对
*任何*不是完整虚拟机的沙箱都成立。

**Docker 中的老旧 NAS，没有按文件夹的墙（`basic`）：**
- ✅ 你的**密钥仍受保护**（登录密钥、备份）- *尤其是你把 UID 隔离设为 `on` 时*。
- ✅ 除非获准，扩展**没有网络**。
- ✅ **危险的系统操作仍会**在内核层面**被阻止**。
- ⚠️ 但没有按文件夹的墙，坏扩展**可能读取普通的非机密文件**、**列出正在运行的进程**，
  而且 - 因为扩展共用一个账户 - 可能**读取或干扰其他扩展的文件**。
- **结论：**在这样的主机上只安装你信任的扩展，或者换用现代内核以获得完整的文件墙。

---

## 配置示例（可直接复制）

这些写进你的 `.env`（Docker compose）或作为 `EVEY_*` 环境变量。

**1. 普通的现代服务器 - 推荐（什么都不做）：**
```bash
# Defaults are correct. Optionally make it explicit:
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

**2. 老旧 NAS，最大限度保护密钥：**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=on          # force the secret wall on
# and do NOT set no-new-privileges in your compose security_opt
```

**3. 现代服务器上的最大双保险（两道墙叠加）：**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid wall on top
```

**4. 固定某个模式（进阶）：**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable (fail‑closed)
```

**5. 某个扩展在 UID 隔离下出问题：**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. 资源上限（之后都可以在管理 → Plugins 里修改）：**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes/threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install‑dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

---

## 运行中更改沙箱模式

你可以在**管理 → Plugins** 的 **Resource limits** 卡片里更改模式。正在运行的扩展会**自动
重启**，让新模式立即生效（无需手动停用再启用）。安全徽章显示实际生效的模式。如果某个扩展在
新模式下无法运行（例如你在 Docker 上选了需要特权的 `bubblewrap`），它会在面板里显示为
**出错** - 改回去即可。

---

## 验证是否生效

```bash
# the launcher carries the capability used for uid separation:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# which user a running plugin runs as (evey = shared with server, evey-plugin = separated):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# the sandbox mode actually in force: the Security badge on the Admin → Plugins page.
```

---

## 另见

- [SECURITY.md](../SECURITY.md) - 完整的扩展安全模型（每一层、按模式的对照表、已知局限）。
- [deployment-bare-metal.md](../deployment-bare-metal.md) - 不用 Docker 运行。
- [installation.md](../installation.md) - 用 Docker 自托管的完整步骤。
