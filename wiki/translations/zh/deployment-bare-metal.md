---
source_sha: 8e943dfd72345f44add4c70011cd163dc77c351f5f410f69d2165ba7c88ea11d
---

# 不使用容器的安装（不用 Docker）

本页讲的是把 Evey 直接跑在宿主机上（用 systemd、init 脚本，或者手动启动），而不是用
[Docker 镜像](installation.md)。推荐的方式仍然是 Docker 镜像 - 它已经把扩展沙箱、
加固措施和各种运行时都接好了。不使用容器，就意味着这套安全姿态得由**你自己**复现：
把该有的二进制文件放在一起、建一个专用的非 root 用户、选一个内核支持的沙箱后端，
并把存放密钥的目录设成仅属主可读。本页就是这件事的一步步操作说明。

如果你只想图省事装好就用，那读到这里就够了，直接去看
[installation.md](installation.md)。

## 1. 前置条件

### 二进制文件（从源码构建）

两个 Go 二进制文件都要构建，并且**放进同一个目录**。扩展运行器查找启动器时，先看
`$EVEY_PLUGIN_LAUNCHER`，再找自身可执行文件旁边（`os.Executable()`），最后才是 `$PATH` -
所以把两者放在一起是最省事也最正确的布局。

```bash
# 在仓库根目录执行
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# 并排安装，例如：
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/
```

`launcherPath()`（`internal/adapters/driven/pluginrunner/runner_landlock.go`）的查找顺序：

1. `$EVEY_PLUGIN_LAUNCHER`（必须是一个可执行文件）- 显式覆盖。
2. 与正在运行的 `evey` 二进制文件**同目录**下的 `evey-plugin-launcher`。
3. `$PATH` 中任意位置的 `evey-plugin-launcher`。

### LD_PRELOAD seccomp 垫片

构建 `libevey-sandbox.so` 垫片（一个很小的 C 文件，需要 C 编译器）：

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

运行器只会在**硬编码路径 `/app/libevey-sandbox.so`** 上找它（`runner.go` 里的
`sandboxLibPath`）。文件不存在时，扩展会**拒绝启动**（fail-closed）- 除非跑在开发模式下。
这个垫片为动态链接的扩展加上一份 seccomp 拒绝清单，叠加在 Landlock/bubblewrap 之上。

> Docker 镜像把开源二进制文件、SDK 和这个垫片都放在 `/app` 下（那里没有任何密钥）。
> 不使用容器时，你可以照搬这个路径（`/app/libevey-sandbox.so`），也可以做个符号链接。

### 运行时

你的扩展在 `entrypoint:` 里调用什么，就装什么。运行器认得这些解释器
（`runner.go` 里的 `knownInterpreters`）：`python3`、`python`、`node`、`php`、`ruby`、
`perl`、`lua`、`bash`、`sh`。至少要装：

- **`python3`** - 社区扩展绝大多数都用它。
- **`node`**、**`php`**、**`ruby`**、**`lua`** - 仅在你的扩展用到时才需要。

此外还有核心的运行时依赖：

- **ffmpeg**（转码；镜像里还捆绑了 VAAPI 驱动和用于片头识别的 `chromaprint`）。
- **PostgreSQL**（服务端加一个数据库）。
- **Meilisearch**（搜索索引；也可以用官方二进制以嵌入方式运行）。
- **`bwrap`**（bubblewrap）- 仅在你打算使用 bubblewrap 沙箱后端时才需要（见下文）。
- **`libcap`**（`setcap`）- 用来授予第 4 节所说的那项文件能力。

## 2. 用专用的非 root 用户运行

**绝不要以 root 身份运行服务器。** 按扩展宿主进程使用的 UID 建两个无特权用户
（`pluginhost/host_server.go` 里的 `pluginUID = 1001`、`pluginGID = 1001`）：

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey`（UID 1000）用来跑服务器。
- `evey-plugin`（UID 1001）是扩展被降权到的那个 UID（启用 UID 隔离时由启动器完成降权），
  这样一来，即使某个扩展被攻破，它落在的也是另一个无特权用户身上，
  单靠文件系统权限就读不到服务器的密钥。见第 4 节。

服务器在启动时会把自己标记为**不可转储**（`PR_SET_DUMPABLE=0`，
`cmd/evey/hardening_linux.go`），这样同机运行的扩展就无法翻找
`/proc/<server-pid>/{mem,environ}`。

## 3. 按内核挑选沙箱后端

设置 `EVEY_PLUGINS_SANDBOX_MODE`。默认值 `auto` 会按
**landlock → bubblewrap → basic** 的顺序解析，挑出宿主机支持的最强一档
（`runner_landlock.go` 里的 `resolveAutoMode()`）。如果你想要 fail-closed 的行为，
就明确指定一个。

### Landlock（推荐）

它提供按路径的文件系统白名单（扩展只看得到自己的目录和套接字目录，绝对看不到你的数据和密钥），
外加一层 Landlock v4 的 TCP 出网过滤。它**不需要特权、不需要挂载、也不需要命名空间** -
启动器先把规则套在自己身上，再 `execve` 启动扩展。

**内核要求** - Landlock 既要编译进内核，*也*要出现在当前启用的 LSM 列表里。运行器的
`landlockAvailable()`（`landlock_probe_linux.go`）会试探
`landlock_create_ruleset(VERSION)`；当该系统调用不存在（内核 `<5.13`），
或者 Landlock 不在启用的 LSM 列表中时，它会返回一个 errno。验证方法：

```bash
uname -r                              # 需要 >= 5.13
cat /sys/kernel/security/lsm          # 必须包含 "landlock"
```

如果列表里没有 `landlock`，就在内核命令行上把它打开并重启（具体的列表因发行版而异 -
保留原有的条目，在末尾追加 `landlock`）：

```
# 例如写在 GRUB_CMDLINE_LINUX 里：
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

然后：

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # 显式指定 + fail-closed（Landlock 无法生效时直接拒绝启动扩展）
# 或者保持默认值：
EVEY_PLUGINS_SANDBOX_MODE=auto        # 可用时会解析为 landlock
```

`landlock` 模式会给启动器传 `-require-landlock`：一旦 Landlock 没有生效，
启动器宁可以非零状态退出，也不会让扩展在无约束的状态下运行。

### bubblewrap

如果实在用不了 Landlock，就装上 `bwrap` 二进制。bubblewrap 会把每个扩展放进
user/pid/uts/ipc/(net) 命名空间，并配一份挂载白名单。它要求**开启无特权用户命名空间**
且 **`mount` 系统调用可用** - 在大多数不用容器的 Linux 上都满足，
但在非特权的 Docker 容器里会被禁掉（这正是容器里默认用 Landlock 的原因）。
只有在 Landlock **不可用**、并且一次性的 bwrap 试探确实成功时，`auto` 才会选中 bubblewrap
（`ProbeSandbox` / `sandbox_probe.go`）。

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap 命名空间
# 或者：
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap 加上 LD_PRELOAD seccomp 垫片
```

确认宿主机确实允许这套命名空间加挂载的组合：

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic（最后的退路）

`EVEY_PLUGINS_SANDBOX_MODE=basic` - 降 UID 加上 LD_PRELOAD seccomp 垫片，
同时启动器照样会装上内核 seccomp 拒绝清单，并在条件允许时尽力启用 Landlock。
这一档**没有按路径的文件系统隔离**：扩展以 UID 1001 运行，只受 seccomp 约束。
只有在 Landlock 和 bubblewrap 都用不了的老旧或受限内核上才用它，
并靠第 4 节的 UID 隔离来保护密钥。

> **拿到 `subprocess` 权限的扩展必须有启动器**，basic 模式下也不例外。
> LD_PRELOAD 垫片在子进程 `exec` 之后就失效了，所以它约束不了一个获准派生子进程的扩展 -
> 能约束它的只有启动器的内核 seccomp（在 `exec` 前一刻装上，并跨 `exec` 继承下去）。
> 如果 `evey-plugin-launcher` 不存在（见第 1 节），这类扩展在生产环境下会
> **拒绝启动**（fail-closed），而不是在无约束的状态下跑起来。没有该权限的扩展，
> 则按上文所述退回到 LD_PRELOAD 垫片。

## 4. UID 隔离（不靠 root 也能保护密钥）

UID 隔离让每个扩展跑在一个单独的用户下（UID 1001，`evey-plugin`），与服务器（UID 1000）
区分开，这样扩展就无法凭普通的文件系统权限（DAC）读到服务器的 `jwt.key`、备份或日志。
它在 **`basic`** 模式下最为关键（因为那里没有 Landlock 的路径隔离）；
在 Landlock 或 bubblewrap 之下，文件系统本来就已经隔开了，这一层就显得多余。

由 `EVEY_PLUGINS_UID_SEPARATION` 控制（`auto` | `on` | `off`，默认
`auto`）：

- `auto` - 只在沙箱最终解析为 `basic` 时才启用。在现代的不用容器的内核上（走 Landlock）
  它保持关闭，这是有意为之。
- `on` - 始终启用，即使在 Landlock/bubblewrap 之下（多一层彼此独立的防护）。
- `off` - 从不启用。

**降权由启动器完成，而不是服务器。** 服务器始终以非 root 运行，且**不**持有任何能力；
真正带着一项文件能力的是那个很小的 `evey-plugin-launcher`，它会在 exec 扩展的前一刻
把自己 `setuid` 到 1001：

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

这样服务器就彻底没有特权 - 服务器端的 RCE 也没法 `setuid(0)`，因为服务器手里一个能力都没有；
持有能力的只有那个不吃任何外部输入的小启动器（而且它会拒绝任何低于 1000 的目标 UID）。
这一层是**尽力而为**的：如果启动器降不下去（缺少该能力，或者设置了 `no_new_privs`），
它会记一条警告，并让扩展以服务器的 UID 运行，而不是直接失败 - 所以它绝不会导致扩展起不来。

> **与 `no_new_privs` 不兼容。** 如果你在启动服务器时带上了 `no-new-privileges`
> （或 systemd 的 `NoNewPrivileges=yes`），文件能力在 exec 时会被忽略，
> UID 隔离就悄悄退化成同 UID 运行（只留一条警告）。两者只能选一个。

如果服务器反而是**以 root 运行**的（不推荐），它会直接把扩展降到 1001，
也就不需要启动器的那项能力了。

想看各种场景下的通俗说明，请参见：
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。

## 5. 密钥文件的权限

照着 Docker 镜像的做法来（见 `Dockerfile`）：把存放密钥的目录设为**仅属主可读**，
这样即便没有 Landlock，降权到 UID 1001 的扩展也没法靠普通 DAC 读到它们。

```bash
sudo chown -R evey:evey /var/lib/evey
# plugins/ 归服务器所有（安装扩展时由它解压到这里）；evey 组 - evey-plugin 属于该组 -
# 只能读取和执行已安装的文件：
sudo chmod 750 /var/lib/evey/plugins
# 存放密钥的目录仅属主可访问（组和其他人无权限）：
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# JWT 签名密钥本身：
sudo chmod 600 /var/lib/evey/jwt.key      # （首次启动时自动生成，权限即为 0600）
```

（如果你用 `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH` 指向了别处，请相应调整路径。）

## 6. 环境变量与配置

所有运行时配置都通过 `EVEY_*` 环境变量提供。数据库、搜索、存储路径、JWT 密钥路径以及
反向代理相关的变量，设置方式和 Docker 部署完全一样 - 完整的表格见
[configuration.md](configuration.md)，数据库、搜索和反向代理的详细步骤见
[installation.md](installation.md)。这里不再重复那张表。以下几点是不使用容器时特有的：

- 把 `EVEY_DB_HOST` / `EVEY_MEILI_URL` 指向本机的 PostgreSQL / Meilisearch。
- 服务器提供的依然是**普通 HTTP** - TLS 要靠前面的反向代理来做
  （`EVEY_SERVER_BASE_URL` 用 `https://` 协议，再配上 `EVEY_TRUSTED_PROXIES`）。参见
  [reverse-proxy.md](reverse-proxy.md)。
- **服务器会打开两个端口，其中只有一个可以对外暴露。**
  `EVEY_SERVER_PORT`（默认 `9090`）是原生的 gRPC 监听端口；你的客户端和反向代理真正
  访问的 HTTP API 在**它加一**的端口上（`9091`）。反向代理和防火墙都只对准 HTTP 端口。
  gRPC 端口属于内部传输：决定局域网之外的客户端能否连接的远程访问网络开关，
  是接在 HTTP 链路上的，**并不覆盖 gRPC 监听端口**。正因如此，gRPC 监听端口默认
  **只绑定环回地址**（`EVEY_GRPC_BIND=127.0.0.1`），不使用容器运行时也就不会意外把它暴露出去。
  如果你确有理由要把它公开出来 - 比如另一台主机上有个原生 gRPC 客户端 - 就设置
  `EVEY_GRPC_BIND=0.0.0.0`；服务器会在启动时警告该开关对它不生效，
  之后为它加防火墙就是你自己的事了。
- 数据库迁移在启动时自动执行，不需要单独跑一步迁移。
- 扩展的资源上限（`EVEY_PLUGINS_MEMORY_MB`、`_PIDS_MAX`、`_DISK_MAX_MB`、`_CPU_NICE`）
  和沙箱模式都可以在运行时由管理员从“管理 → 扩展”里修改；环境变量的值只是初始默认值。

## 7. 验证沙箱是否生效

服务器会把最终解析出的沙箱后端写进日志。第一个扩展启动之后，查看服务器日志：

- **自动解析的结果**（只记录一次）：
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  （也可能是 `bubblewrap` / `basic`）。
- **每个扩展**（每次扩展启动都会记录）：
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **退回到了 basic**（如果你本来指望走 Landlock，那这条警告就是你*不*想看到的）：
  `plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available …` - 意思是 Landlock 和可用的 bwrap 都没找到。
  请回头检查第 3 节。

再独立确认一下内核这边的情况：

```bash
cat /sys/kernel/security/lsm          # Landlock 生效了吗？（列表里应有 "landlock"）
grep Seccomp: /proc/<plugin-pid>/status   # 2 = 运行中的扩展已进入过滤模式
```

某个正在运行的扩展进程上出现 `Seccomp: 2`，就说明内核 seccomp 过滤器确实装上了。
如果你固定了 `EVEY_PLUGINS_SANDBOX_MODE=landlock` 而 Landlock 又没有生效，
扩展会**拒绝启动** - 这正是 fail-closed 约定在按设计工作。

---

其余的一切（备份、升级、恢复用的命令行工具、故障排查），
[installation.md](installation.md) 中面向 Docker 的那些章节同样适用 -
`evey admin` 子命令和备份机制用的都是同一个二进制文件。
