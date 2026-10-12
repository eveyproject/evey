---
source_sha: 0b2dda412b186b3b40f33ea4027ceb0d328e5f8f39f87d2312a8c7bdab2edbed
reviewed: true
---
# 不使用容器

这一页说明怎样直接在主机上运行 Evey（用 systemd、init 脚本，或者手动运行），而不用 Docker 镜像（见[安装](installation.md)）。
推荐的方式是镜像：插件沙箱、安全加固和插件运行时都已经在里面配好了。不用容器运行，就意味着要自己做到同样的安全程度：
相关的程序放在一起，一个专用的非 root 用户，您的内核支持的沙箱后端，以及只有所有者能访问的密钥目录。
本页按顺序逐步说明。

## 构建并安装程序

构建两个 Go 程序，并把它们安装在同一个目录里：插件运行器先在 `$EVEY_PLUGIN_LAUNCHER` 里找启动器，
然后在它自己的可执行文件旁边找，最后在 `$PATH` 里找。

```bash
# 在代码仓库的根目录下执行
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# 可选：命令行自动补全和手册页，都由程序自己输出
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` 不需要任何配置，就能显示版本号、提交和构建日期。

### seccomp 垫片库

构建 `libevey-sandbox.so`（只有一个 C 文件，需要 C 编译器），并把它安装到运行器要求的路径 `/app/libevey-sandbox.so`：

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

这个路径在运行器中是固定的。没有这个文件，插件会拒绝启动（无法隔离就拒绝），除非服务器以开发模式运行。
这个垫片库在 Landlock 或 bubblewrap 之上，为动态链接的插件再加一份 seccomp 禁止清单。
镜像把它和 SDK 一起放在 `/app` 下；用符号链接也可以。

### 运行时和服务

请安装您的插件的 `entrypoint:` 会调用的程序。运行器识别 `python3`、`python`、`node`、`php`、`ruby`、`perl`、`lua`、`bash` 和 `sh`；
大多数社区插件只需要 `python3`。然后是服务器要通信的那些服务：

| 依赖 | 用途 |
|---|---|
| **ffmpeg** 和 **ffprobe** | 转码、探测文件；镜像里还带了 VAAPI 驱动和用于片头检测的 `fpcalc`（chromaprint） |
| **PostgreSQL**（服务器加一个数据库）和 **`pg_dump`** | 数据库；`pg_dump` 用来在迁移前自动导出数据库 |
| **Meilisearch** | 搜索索引；没有它，搜索会改用 SQL |
| **`bwrap`**（bubblewrap） | 只有使用 bubblewrap 沙箱后端时才需要 |
| **`setcap`**（libcap） | 用来给启动器授予文件能力（capability） |

内置的数据库和搜索引擎是镜像入口脚本提供的功能，不是程序本身的：不用容器时，这两者都要您自己运行，
并把 `EVEY_DB_HOST` 和 `EVEY_MEILI_URL` 指向它们。

## 以专用的非 root 用户运行

永远不要以 root 运行服务器。像镜像那样，创建一个组和两个无特权的用户：

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey`（UID 1000）运行服务器。
- `evey-plugin`（UID 1001）是开启 uid 分离时插件切换到的用户。这样，被攻破的插件会以另一个无特权用户的身份运行，
  单凭普通的文件权限，它就无法读取服务器的密钥。

!!! warning
    组 id 很重要：启动器会把插件切换到 UID 1001 和服务器自己的组，并且拒绝任何小于 1000 的 uid 或 gid。
    只用 `useradd --system` 创建的组会得到一个小于 1000 的系统 id，这时 `basic` 模式下的插件就会被拒绝。

服务器在启动时把自己标记为不可转储（`PR_SET_DUMPABLE=0`），这样同一台机器上的插件读不到 `/proc/<server-pid>/{mem,environ}`。

## 为您的内核选择沙箱后端

设置 `EVEY_PLUGINS_SANDBOX_MODE`。`auto`（默认）按 landlock、bubblewrap、basic 的顺序尝试，选择主机支持的最强方式。
如果您希望“无法隔离就拒绝”，请明确设置一种模式。

### Landlock（推荐）

按路径的文件系统允许清单（插件只看得到自己的目录和套接字目录，永远看不到您的数据或密钥），
再加上 Landlock v4 的 TCP 出站过滤。它不需要特权、不需要挂载，也不需要命名空间：启动器先对自身应用它，然后再执行插件。

Landlock 必须编译进内核，并且在当前启用的 LSM 列表里。运行器会探测 `landlock_create_ruleset`；
如果没有这个系统调用（内核比 5.13 旧），或者 Landlock 不在启用列表里，探测就会失败。检查方法：

```bash
uname -r                              # 需要 5.13 或更新
cat /sys/kernel/security/lsm          # 必须包含 "landlock"
```

如果列表里没有 `landlock`，请把它加到内核启动参数里，然后重启（保留原有的项，这个列表因发行版而异）：

```
# 例如在 GRUB_CMDLINE_LINUX 里：
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

然后，可以继续用 `auto`，也可以固定下来：

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # 无法隔离就拒绝：无法强制执行 Landlock 时，插件会被拒绝
```

`landlock` 会给启动器传入 `-require-landlock`，启动器会以非零状态退出，而不是在没有隔离的情况下运行插件。

### bubblewrap

没有 Landlock 时，请安装 `bwrap`。它把每个插件放进 user、pid、uts、ipc 和 network 命名空间里，并配上一份挂载允许清单。
它需要无特权用户命名空间和 `mount` 系统调用。大多数直接运行在主机上的 Linux 都满足，但在无特权容器里会被阻止，
这就是为什么容器里默认用 Landlock。只有在没有 Landlock、并且一次临时的 `bwrap` 探测成功时，`auto` 才会选择 bubblewrap。

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # 命名空间
EVEY_PLUGINS_SANDBOX_MODE=strict      # 命名空间加上 seccomp 垫片库
```

检查主机是否允许设置命名空间和挂载：

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic（最后选择）

`EVEY_PLUGINS_SANDBOX_MODE=basic` 就是切换用户（uid）加上 seccomp 垫片库；如果有启动器，它仍然会装上它的内核 seccomp 禁止清单，
并尽力加上 Landlock。没有按路径的文件系统隔离：插件以 UID 1001 运行，只有 seccomp 保护。
请在 Landlock 和 bubblewrap 都不能用的旧内核或受限内核上使用它，并依靠下面的 uid 分离来保护密钥。

获得了 `subprocess` 权限的插件，即使在 `basic` 模式下也需要启动器：垫片库在子进程 `exec` 之后就不再起作用，
只有启动器设置的内核 seccomp 还在。没有启动器时，这样的插件会拒绝启动，而不是在没有隔离的情况下运行。

## 防止插件访问密钥

uid 分离让每个插件以 `evey-plugin`（UID 1001）运行，和服务器（UID 1000）分开，这样，单凭普通的文件权限，
插件就无法读取 `jwt.key`、备份或日志。它在 `basic` 模式下很重要；在 Landlock 或 bubblewrap 下，文件系统已经受到限制，所以它在那里是多余的。

`EVEY_PLUGINS_UID_SEPARATION`：

| 值 | 行为 |
|---|---|
| `auto`（默认） | 只在沙箱最终是 `basic` 时开启 |
| `on` | 始终开启，作为额外的一层独立防护 |
| `off` | 从不开启 |

切换用户由启动器完成，不由服务器完成。服务器始终不是 root，也没有任何能力（capability）；
启动器带有一项文件能力，在执行插件之前切换到 UID 1001：

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

启动器拒绝任何小于 1000 的目标 uid 或 gid。如果无法切换（缺少能力，或者设置了 `no_new_privs`），
`basic` 模式下的插件会被拒绝，因为在那里 uid 是它唯一的保护；在 Landlock 下，它会记录一条警告，然后以服务器的 uid 运行。

!!! warning
    uid 分离和 `no_new_privs` 不能同时使用。如果服务器在 systemd 的 `NoNewPrivileges=yes` 下运行，
    文件能力在 exec 时会被忽略：`basic` 模式下的插件会被拒绝，而在 Landlock 下，uid 分离会退化为使用同一个 uid。只能二选一。

如果服务器改为以 root 运行（不推荐），它会直接把插件切换到 1001，就不需要这项能力了。
用简单的话说明每种情况，见[插件沙箱设置](plugins/sandbox-deployment.md)。

## 密钥文件的权限

按照镜像的做法：存放密钥的目录只有所有者能访问，这样，即使没有 Landlock，切换到 UID 1001 的插件也无法读取它们。

```bash
sudo chown -R evey:evey /var/lib/evey
# plugins/ 归服务器所有（安装的插件解压在这里）；evey 组
# （evey-plugin 属于这个组）只能读取和执行已安装的文件：
sudo chmod 750 /var/lib/evey/plugins
# 密钥目录只有所有者能访问：
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# JWT 签名密钥本身（第一次启动时以 0600 生成）：
sudo chmod 600 /var/lib/evey/jwt.key
```

请把路径改成您给 `EVEY_*_DIR` 变量和 `EVEY_JWT_PRIVATE_KEY_PATH` 设置的位置。

## 配置环境变量

所有配置都是 `EVEY_*` 环境变量；完整的表格见[配置参考](configuration.md)。不用容器运行时，需要注意这几点：

- 把 `EVEY_DB_HOST` 和 `EVEY_MEILI_URL` 指向您的 PostgreSQL 和 Meilisearch。`EVEY_DB_PASSWORD` 不能是默认的 `evey`。
- Go 程序里路径的默认值是为容器选的：请把 `EVEY_LOG_DIR`（默认是相对路径 `logs`）、`EVEY_BACKUP_DIR`（`/data/backups`）、
  `EVEY_TRANSCODING_CACHE_DIR`（`/data/transcode`）、`EVEY_TRICKPLAY_CACHE_DIR`（`/cache/trickplay`）
  和 `EVEY_JWT_PRIVATE_KEY_PATH`（`/data/jwt.key`）设为您自己的目录。
- 服务器提供的是普通的 HTTP：请在前面放一个反向代理来提供 TLS，把**外部 URL** 设为 `https://` 地址，
  并把代理加到**受信任的代理**里（**管理 → 远程访问**；`EVEY_SERVER_BASE_URL` 和 `EVEY_TRUSTED_PROXIES` 是后备值）。
  见[反向代理配置](reverse-proxy.md)。
- **服务器会打开两个端口，只能对外开放其中一个。**
  `EVEY_SERVER_PORT`（默认 `9090`）是原生的 gRPC 监听端口；您的客户端和反向代理访问的 HTTP API 是这个端口加一（`9091`）。
  远程访问控制决定局域网以外的客户端能否连接，它位于 HTTP 处理链中，不覆盖 gRPC 监听端口，
  所以 gRPC 默认只绑定本机回环地址（`EVEY_GRPC_BIND=127.0.0.1`）。如果您有理由对外开放它，请设置 `EVEY_GRPC_BIND=0.0.0.0`；
  服务器启动时会警告远程访问控制不适用于该端口，此时需要您自己用防火墙限制它。
- 迁移在启动时执行；没有单独的迁移步骤。
- 插件的资源上限（`EVEY_PLUGINS_MEMORY_MB`、`_PIDS_MAX`、`_DISK_MAX_MB`、`_CPU_NICE`）和沙箱模式，
  可以在运行时从**管理 → 插件 → 资源限制**修改；环境变量的值只是初始默认值。
- 管理面板里的重启服务器功能依赖镜像里的进程管理程序，不用容器运行时会显示为不可用：请自己重启服务。

## 确认沙箱正常工作

服务器会在日志里记录它最终选择的后端。第一个插件启动后，请查找：

- 使用 `auto` 时，会出现一次：`plugin sandbox auto-detection complete
  selected_mode=landlock`（或者 `bubblewrap`、`basic`）；
- 每次插件启动时：`plugin sandbox applied sandbox_mode=landlock
  plugin_id=...`；
- 在您期望使用 Landlock 时，不想看到的警告是：`plugin sandbox
  auto-detection fell back to basic mode: no per-path filesystem isolation
  available ...`，意思是既没有找到 Landlock，也没有找到能用的 `bwrap`。请重新检查上面关于后端的那一节。

另外单独确认内核这一侧：

```bash
cat /sys/kernel/security/lsm              # 列出 "landlock"
grep Seccomp: /proc/<plugin-pid>/status   # 2 = 插件上有一个生效的 seccomp 过滤器
```

正在运行的插件进程上出现 `Seccomp: 2` 这一行，说明内核过滤器已经安装。
如果设置了 `EVEY_PLUGINS_SANDBOX_MODE=landlock`，而 Landlock 没有生效，插件会拒绝启动：这说明“无法隔离就拒绝”的机制在正常工作。

关于备份、更新和恢复命令，容器的那些页面同样适用：[备份和更新](backups.md)、[恢复](runbooks/recovery.md)
和[命令行](runbooks/cli.md)介绍的是同一个 `evey admin` 程序。

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/en/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
