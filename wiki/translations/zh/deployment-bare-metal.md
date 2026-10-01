---
source_sha: 87e3250d0329fdea4d7710ddda0cd4624c81d6c2177820f2e45502be4f6fe75f
reviewed: false
---
# 裸机部署（不使用 Docker）

直接在主机上运行 Evey（systemd、init 脚本或手动），而不是使用 [Docker 镜像](installation.md)。Docker 镜像是推荐的方式：它预先接好了插件沙箱、加固措施和运行时。选择裸机意味着要由**你**来重现这套安全配置：把正确的二进制文件放在一起、一个专用的非 root 用户、你的内核支持的沙箱后端，以及只有所有者可访问的机密目录。本页就是为此准备的分步说明。

如果你只想要简单的安装，到此为止，使用 [installation.md](installation.md)。

## 1. 前提条件

### 二进制文件（从源码构建）

构建两个 Go 二进制文件，并**把它们放在同一个目录中**。插件运行器查找启动器的顺序是：先检查 `$EVEY_PLUGIN_LAUNCHER`，然后是它自己的可执行文件旁边（`os.Executable()`），最后是 `$PATH`，所以让它们放在一起是最简单的正确布局。

```bash
# 在仓库根目录
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# 并排安装，例如：
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# 可选：shell 补全和手册页，都由二进制文件输出
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`launcherPath()`（`internal/adapters/driven/pluginrunner/runner_landlock.go`）中的查找顺序：

1. `$EVEY_PLUGIN_LAUNCHER`（必须是可执行文件）：显式覆盖。
2. 与正在运行的 `evey` 二进制文件位于**同一目录**中的 `evey-plugin-launcher`。
3. `$PATH` 中某处的 `evey-plugin-launcher`。

### LD_PRELOAD seccomp 垫片

构建 `libevey-sandbox.so` 垫片（一个小的 C 文件；需要 C 编译器）：

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

运行器在**硬编码路径 `/app/libevey-sandbox.so`**（`runner.go` 中的 `sandboxLibPath`）查找它。如果它不存在，插件会**拒绝启动**（失败即拒绝），除非你在开发模式下运行。这个垫片为动态链接的插件添加一个 seccomp 拒绝列表，叠加在 Landlock/bubblewrap 之上。

> Docker 镜像把开源二进制文件、SDK 和这个垫片放在 `/app` 下（那里没有机密）。在裸机上，你可以照搬这个路径（`/app/libevey-sandbox.so`），或者创建一个符号链接。

### 运行时

安装你的插件的 `entrypoint:` 所调用的任何东西。运行器识别这些解释器（`runner_start.go` 中的 `knownInterpreters`）：`python3`、`python`、`node`、`php`、`ruby`、`perl`、`lua`、`bash`、`sh`。至少需要：

- **`python3`**：大多数社区插件。
- **`node`**、**`php`**、**`ruby`**、**`lua`**：只在你的插件使用它们时需要。

再加上核心运行时依赖：

- **ffmpeg**（转码；镜像还捆绑了 VAAPI 驱动和用于片头检测的 `chromaprint`）。
- **PostgreSQL**（服务器 + 一个数据库）。
- **Meilisearch**（搜索索引；或者通过官方二进制文件以内置方式运行）。
- **`bwrap`**（bubblewrap）：只在你打算使用 bubblewrap 沙箱后端时需要（见下文）。
- **`libcap`**（`setcap`）：用于授予 §4 中描述的文件能力。

## 2. 以专用的非 root 用户运行

**永远不要以 root 运行服务器。** 像 Docker 镜像那样，创建一个组和两个无特权用户；插件会降权到 UID 1001（`pluginhost/host_server.go` 中的 `pluginUID`）：

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

组的 id 很重要：启动器会把插件降权到 UID 1001 和服务器自己的组，并拒绝任何低于 1000 的 id。仅用 `useradd --system` 创建的组会得到一个低于 1000 的系统 id，此时 `basic` 模式下的插件就会被拒绝。

- `evey`（UID 1000）运行服务器。
- `evey-plugin`（UID 1001）是插件被降权到的 UID（在 uid 分离生效时由启动器完成），这样被攻破的插件会落到一个不同的无特权用户上，仅凭普通的文件系统权限就无法读取服务器的机密。见 §4。

服务器在启动时把自己标记为**不可转储**（`PR_SET_DUMPABLE=0`，`cmd/evey/hardening_linux.go`），这样同一台机器上的插件就无法读取 `/proc/<server-pid>/{mem,environ}`。

## 3. 为你的内核选择沙箱后端

设置 `EVEY_PLUGINS_SANDBOX_MODE`。`auto`（默认）按 **landlock → bubblewrap → basic** 的顺序解析，并选择你的主机支持的最强的一个（`runner_landlock.go` 中的 `resolveAutoMode()`）。如果你想要失败即拒绝的行为，就显式选择。

### Landlock（推荐）

按路径的文件系统允许列表（插件只能看到自己的目录 + socket 目录，永远看不到你的数据或机密），加上 Landlock v4 的 TCP 出站过滤。**不需要特权、挂载或命名空间**：启动器把它应用到自己身上，然后 `execve` 插件。

**内核要求**：Landlock 必须既被编译进内核，*又*在活动的 LSM 列表中。运行器的 `landlockAvailable()`（`landlock_probe_linux.go`）会探测 `landlock_create_ruleset(VERSION)`；当系统调用不存在（内核 `<5.13`）或 Landlock 不在活动的 LSM 列表中时，它会返回一个 errno。验证：

```bash
uname -r                              # 需要 >= 5.13
cat /sys/kernel/security/lsm          # 必须包含 "landlock"
```

如果列表中缺少 `landlock`，在内核命令行中启用它并重启（具体列表因发行版而异：保留现有条目并追加 `landlock`）：

```
# 例如在 GRUB_CMDLINE_LINUX 中：
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

然后：

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # 显式 + 失败即拒绝（如果无法强制执行 Landlock 就拒绝插件）
# 或者保持默认：
EVEY_PLUGINS_SANDBOX_MODE=auto        # 可用时解析为 landlock
```

`landlock` 模式会向启动器传入 `-require-landlock`，如果 Landlock 没有生效，启动器会以非零状态退出，而不是在没有隔离的情况下运行插件。

### bubblewrap

如果无法使用 Landlock，安装 `bwrap` 二进制文件。bubblewrap 把每个插件放在 user/pid/uts/ipc/(net) 命名空间中，并使用挂载白名单。它需要**启用无特权用户命名空间**，并且 **`mount` 系统调用可用**：这在大多数裸机 Linux 上成立，但在无特权的 Docker 容器内会被阻止（这正是 Landlock 成为容器默认选项的原因）。`auto` 只在 Landlock **不存在** 并且一次临时的 bwrap 探测确实成功时才会选择 bubblewrap（`ProbeSandbox` / `sandbox_probe.go`）。

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap 命名空间
# 或者：
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + LD_PRELOAD seccomp 垫片
```

检查主机确实允许命名空间 + 挂载的设置：

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic（最后手段）

`EVEY_PLUGINS_SANDBOX_MODE=basic`：UID 降权 + LD_PRELOAD seccomp 垫片，启动器仍会安装内核 seccomp 拒绝列表，并在可用时尽力应用 Landlock。**没有按路径的文件系统隔离**：插件以 UID 1001 运行，只有 seccomp。只在既没有 Landlock 也没有 bubblewrap 的旧内核或受限内核上使用它，并依靠 §4 中的 UID 分离来保护机密。

> **被授予 `subprocess` 权限的插件需要启动器**，即使在 basic 模式下也是如此。LD_PRELOAD 垫片在子进程 `exec` 之后不再存在，所以它无法隔离被允许启动子进程的插件：只有启动器的内核 seccomp（在 `exec` 之前安装并在其后继承）可以。如果缺少 `evey-plugin-launcher`（见 §1），这样的插件在生产环境中会**拒绝启动**（失败即拒绝），而不是在没有隔离的情况下运行。没有该权限的插件会像上面说明的那样退回 LD_PRELOAD 垫片。

## 4. UID 分离（无需 root 也能保护机密）

UID 分离让每个插件以一个单独的用户（UID 1001，`evey-plugin`）运行，与服务器（UID 1000）不同，这样插件仅凭普通的文件系统权限（DAC）就无法读取服务器的 `jwt.key`、备份或日志。它在 **`basic`** 模式下最重要（没有 Landlock 的路径隔离）；在 Landlock 或 bubblewrap 下，文件系统已经被隔开，所以那里它是多余的。

由 `EVEY_PLUGINS_UID_SEPARATION` 控制（`auto` | `on` | `off`，默认 `auto`）：

- `auto`：只在沙箱解析为 `basic` 时启用。在现代的裸机内核上（Landlock），它按设计保持关闭。
- `on`：始终启用，即使在 Landlock/bubblewrap 下（额外的一层独立保护）。
- `off`：从不启用。

**降权由启动器完成，而不是服务器。** 服务器保持非 root 且**没有**任何能力；小巧的 `evey-plugin-launcher` 二进制文件带有唯一的一项文件能力，并在 exec 插件之前把自己 `setuid` 到 1001：

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

这让服务器保持完全无特权：服务器的远程代码执行漏洞无法 `setuid(0)`，因为服务器不持有任何能力；只有那个小巧、不接受输入的启动器持有（并且它会拒绝任何低于 1000 的目标 UID）。如果启动器无法降权（缺少能力，或者设置了 `no_new_privs`），`basic` 模式下的插件会被**拒绝**，因为在那里 UID 是它与机密之间唯一的墙；在 Landlock 下，它会记录一条警告并以服务器的 UID 运行。

> **与 `no_new_privs` 不兼容。** 如果你以 `no-new-privileges`（或 systemd 的 `NoNewPrivileges=yes`）启动服务器，文件能力在 exec 时会被忽略：`basic` 模式下的插件会被拒绝，而在 Landlock 下，uid 分离会退化为同一个 UID（并带有警告）。二者只能选其一。

如果服务器改为**以 root** 运行（不推荐），它会直接把插件降权到 1001，就不需要启动器的能力了。

按场景说明的通俗指南：[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。

## 5. 机密文件的权限

照搬 Docker 镜像的做法（`Dockerfile`）：让包含机密的目录**只有所有者可访问**，这样即使没有 Landlock，降权到 UID 1001 的插件也无法通过普通的 DAC 读取它们。

```bash
sudo chown -R evey:evey /var/lib/evey
# 服务器拥有 plugins/（它把安装内容解压到那里）；evey 组（evey-plugin
# 属于该组）只能读取/执行已安装的文件：
sudo chmod 750 /var/lib/evey/plugins
# 机密目录只允许所有者访问（组和其他人都不行）：
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# JWT 签名密钥本身：
sudo chmod 600 /var/lib/evey/jwt.key      # （首次启动时自动以 0600 生成）
```

（根据你设置的 `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH` 调整路径。）

## 6. 环境与配置

所有运行时配置都通过 `EVEY_*` 环境变量完成。像 Docker 部署那样设置数据库、搜索、存储路径、JWT 密钥路径和反向代理变量：完整的表格见 [configuration.md](configuration.md)，数据库、搜索和反向代理的说明见 [installation.md](installation.md)。这里不重复那张表。裸机特有的注意事项：

- 让 `EVEY_DB_HOST` / `EVEY_MEILI_URL` 指向你本地的 PostgreSQL / Meilisearch。
- 服务器仍然提供**未加密的 HTTP**：在前面放一个反向代理来提供 TLS（在 **Admin → 远程访问** 中设置一个 `https://` 外部 URL 并把代理加入受信任的代理，或者使用 `EVEY_SERVER_BASE_URL` 和 `EVEY_TRUSTED_PROXIES`）。见 [reverse-proxy.md](reverse-proxy.md)。
- **服务器会打开两个端口，只有其中一个可以暴露。** `EVEY_SERVER_PORT`（默认 `9090`）是原生 gRPC 监听端口；你的客户端和反向代理使用的 HTTP API 是**该端口加一**（`9091`）。只让你的反向代理和防火墙指向 HTTP 端口。gRPC 端口是内部传输通道：决定局域网之外的客户端能否连接的远程访问网络限制位于 HTTP 链路中，**不覆盖 gRPC 监听端口**。因此，gRPC 监听端口默认**只绑定到环回地址**（`EVEY_GRPC_BIND=127.0.0.1`），这样裸机运行就不会意外暴露它。如果你有理由发布它（另一台主机上的原生 gRPC 客户端），设置 `EVEY_GRPC_BIND=0.0.0.0`；服务器会在启动时警告该限制不适用于它，此时用防火墙保护它就是你的责任了。
- 迁移在启动时自动运行；没有单独的迁移步骤。
- 插件资源上限（`EVEY_PLUGINS_MEMORY_MB`、`_PIDS_MAX`、`_DISK_MAX_MB`、`_CPU_NICE`）和沙箱模式可以在运行时从 Admin → 插件 中由管理员修改；环境变量的值只是初始默认值。

## 7. 验证你的沙箱

服务器会记录解析后的沙箱后端。第一个插件启动后，检查服务器日志：

- **auto 解析**（记录一次）：`plugin sandbox auto-detection complete  selected_mode=landlock`（或 `bubblewrap` / `basic`）。
- **每个插件**（每次插件启动时）：`plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **退回到 basic**（如果你期望 Landlock，这是你*不*想看到的警告）：`plugin sandbox auto-detection fell back to basic mode: no per-path
  filesystem isolation available …`：表示既没有找到 Landlock，也没有找到可用的 bwrap。重新检查 §3。

单独确认内核部分：

```bash
cat /sys/kernel/security/lsm          # Landlock 是否生效？（应列出 "landlock"）
grep Seccomp: /proc/<plugin-pid>/status   # 2 = 运行中的插件启用了过滤模式
```

在一个运行中的插件进程上出现 `Seccomp: 2` 行，就确认了内核 seccomp 过滤器已安装。如果你固定了 `EVEY_PLUGINS_SANDBOX_MODE=landlock` 而 Landlock 没有生效，插件会**拒绝启动**：这是失败即拒绝的约定在按预期工作。

---

其他所有事项（备份、更新、恢复 CLI、故障排除）同样适用 [installation.md](installation.md) 中面向 Docker 的部分：`evey admin` 子命令和备份机制都是同一个二进制文件。
