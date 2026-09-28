---
source_sha: 2afa372fc288cb82c9e90ad0f1bb879056bb0e14682a438cab6ce64d779bf48d
---
# 扩展沙箱与威胁模型

> **你要找的是哪一页？** 这一页描述*服务器*为约束扩展做了什么：各防御层、沙箱模式、每种模式
> 仍然拦不住什么，以及如何配置。如果你在编写扩展，想知道可以声明什么、什么会被拒绝，请改读
> [扩展安全模型](plugins/security-model.md)。两页从相反的两端讲同一个主题，刻意互不重复。

本文描述 Evey 对第三方扩展施加的防御层、这些防御的局限，以及走向更强隔离的路线图。

## 威胁模型：恶意扩展会尝试什么

Evey 扩展以独立的操作系统进程运行，通过 Unix 套接字与宿主交换 gRPC。宿主默认不信任任何扩展代码。我们防御的风险，按严重程度排列：

1. **冒充宿主 RPC 调用方** - 恶意扩展冒充另一个扩展以获取其权限。
2. **提权** - 扩展逃出沙箱，以服务器用户的身份执行任意命令。
3. **文件系统泄露 / 写入** - 扩展读写其安装目录之外的路径。
4. **网络外泄** - 扩展建立出站连接把数据发回去。
5. **资源滥用** - 挖矿、fork 炸弹、耗尽文件描述符。
6. **供应链** - 扩展作者提供经过混淆或编译的恶意软件，静态分析器无法检查。

## 防御层（当前）

### 1. 安装时的静态分析（PREVIEW + INSTALL）

每个扩展 ZIP 都被解压到临时目录，并由 [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go) 遍历。扫描的类别及严重性：

| 类别 | 严重性 | 示例 |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`、`subprocess.run`、`exec()`、`child_process.exec`、PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`、`preg_replace /e`、Ruby 反引号/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`、`fetch`、反弹/绑定 shell |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`、`.ssh/`、`rm -rf`、符号链接操作 |
| `obfuscation` | CRITICAL | `eval`、`exec`、base64 解码、十六进制序列、`__builtins__`、PHP `assert(base64_decode(...))`、shell `base64 -d \| bash`、`echo <blob> \| openssl` |
| `privilege` | CRITICAL | `setuid`、`chmod 4777`、`sudo`、向 PID 1 发信号 |
| `cryptomining` | CRITICAL | `stratum+tcp`、`xmrig`、`cryptonight` |
| `runtime_install` | CRITICAL | `pip install`、`npm install`、`curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`、`importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`、`process.binding`、`new Function`、Python `__subclasses__()`、`__class__.__bases__`、通过 `__mro__` 遍历类层级 |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`/`pickle.load`、`marshal.loads`、`__reduce__`/`__reduce_ex__`；WARNING：未显式指定 Loader 的 `yaml.load()` |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`、`ffi.Library`；WARNING：Go `unsafe.Pointer`、`unsafe.Slice`、`unsafe.SliceData`、`unsafe.StringData` |
| `compiled_binary` | WARNING → BLOCK | `.so`、`.dll`、`.exe`、`.wasm`、`.jar`（扫描器看不透） |
| `embedded_archive` | WARNING → BLOCK | `.zip`、`.tar.gz`（可解压出代码） |
| `mime_mismatch` | WARNING（按严重处理） | 扩展名声称是图片，内容却是别的东西 |
| `suspicious_url` | WARNING | URL 字面量中的非 HTTP 协议 |
| `unvendored_deps` | WARNING | 存在 `requirements.txt`、`package.json` |

CRITICAL 问题或 `mime_mismatch` 会阻止安装（`Safe = false`）。

`compiled_binary` 和 `embedded_archive` **默认**阻止安装，除非清单声明了 `native-binary` 权限。管理员在接受之前的预览步骤中会同时看到警告和所申请的权限。

### 2. 清单权限关卡

扩展的 `plugin.yaml` 声明它申请的权限。运行器在两个层面执行这些权限：

- **`subprocess`** - 允许创建子进程。没有它时，LD_PRELOAD seccomp 垫片会对解释型扩展拒绝 `execve`/`execveat`。（启动器的内核 seccomp *不*拒绝 `execve` - 它得执行扩展 - 但子进程无论如何都会继承过滤器 + Landlock 域。）
- **`native-binary`** - 附带 `.so` / `.dll` / `.exe` / 压缩包时必需。管理员必须在安装时接受这些不透明代码。
- **API 权限**（`api.library`、`api.user`、`api.metadata`、`media.read`、`media.write`、`notification.sender`）- 在每次宿主 RPC 时由 [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go) 检查。

### 3. UID 隔离（DAC）

扩展进程可以被降到 UID 1001（`evey-plugin`），与服务器的 UID（1000，`evey`）不同，这样仅靠普通的 DAC，服务器的密钥对它就是不可读的。由 `EVEY_PLUGINS_UID_SEPARATION` 控制（`auto` | `on` | `off`，默认 `auto`）：

- `auto` - **只在沙箱解析为 `basic` 时**降权（没有 Landlock/bubblewrap 的文件系统约束）- 这是 DAC 成为保护密钥唯一手段的唯一情形。在 landlock/bubblewrap 下跳过，因为那里是多余的。
- `on` - 始终降权（在 Landlock/bubblewrap 之上再加一层独立防线）。
- `off` - 从不。

**降权由启动器执行，而不是服务器。**服务器保持非 root、没有任何 capability；`evey-plugin-launcher` 带有 `cap_setuid`（文件 capability，`Dockerfile` 中的 `setcap cap_setuid+ep`），并在 exec 之前把自己 `setgid`/`setuid` 到共享组 + UID 1001。它拒绝任何低于 1000 的目标 UID。如果无法降权（没有 capability、设置了 `no_new_privs`，或启动器二进制本身缺失），结果取决于模式：在 `basic` 中，UID 是保护密钥的唯一一道墙，扩展会被**拒绝**（封闭失败）；在 `landlock` 下，文件系统已经被隔离，它会发出警告并以服务器的 UID 运行扩展。以 root 运行的服务器直接通过 `SysProcAttr` 为扩展降权，不需要 capability。大白话指南：[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。

它带来的好处，**即使在没有 Landlock 的内核上**（启用时）：

- 服务器的密钥受 DAC 保护，uid 1001 的扩展无法读取：`jwt.key`（`0600`）和备份（`0700` 目录 + `0600` 文件）会在**运行时**被设为仅属主可读，所以即使 `/data` 是绑定挂载的，保护依然成立；uid 1001 的扩展读不到它们。（镜像也会对 `/data/{keys,logs,backups}` 执行 `chmod 700`，但从主机绑定挂载的 `/data` 保留它自己的权限 - 真正有保证的是上面那些运行时设置的模式。）
- 与服务器上的 `PR_SET_DUMPABLE=0` 配合（见 [§8](#8-server-process-hardening-pr_set_dumpable0)），内存中的 JWT 密钥和环境变量里的密钥（`DB_PASSWORD`、`MEILI_MASTER_KEY`）无法通过 `/proc/<server>/{mem,environ,maps}` 读取。

所有扩展共用 UID 1001，所以仅靠 DAC 它们*彼此之间*并不隔离 - 这正是文件系统约束层（Landlock / bubblewrap）所补充的。在 `basic` + UID 隔离下，恶意扩展可能读取或干扰同伴的文件；没有 Landlock 时要让扩展彼此隔离，需要*每个扩展一个* UID。见 [§4](#4-syscall-confinement-kernel-seccomp--ld_preload)、下面按模式的对照表，以及[已知局限](#known-limits-of-the-sandbox)。

### 4. 系统调用约束（内核 seccomp + LD_PRELOAD）

两个 seccomp 过滤器施加几乎相同的黑名单（只在信号相关的系统调用上有差异 - 见表）；两者都先调用 `prctl(PR_SET_NO_NEW_PRIVS)`，再安装一个 BPF 程序，对被拒绝的系统调用返回 EPERM，其余一律放行。

1. **由内核施加（主要防线）。** [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) 垫片被加在扩展 argv 的前面，并按构建架构**在 `execve` 之前**安装 BPF 过滤器（[`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)）。过滤器会通过 `execve` 继承给扩展及其之后创建的任何子进程。因为它由启动器安装而非通过 `LD_PRELOAD`，所以它**同样约束静态/编译型的扩展二进制** - 它们从不加载 `libevey-sandbox.so`，过去可以完全绕过垫片。`seccomp_data` 的 ABI 字段中架构不匹配会触发 `SECCOMP_RET_KILL_PROCESS`（挫败 i386/x32 系统调用号混淆）。
2. **LD_PRELOAD（动态扩展的双保险）。** [`sandbox/sandbox.c`](../sandbox/sandbox.c) 是预加载进解释型扩展的一个小构造函数；黑名单相同，另外还会为扩展自身设置 `PR_SET_DUMPABLE=0`。

**两者中哪一个生效，取决于沙箱模式**（见下面按模式的对照表）：启动器的内核过滤器在 `landlock` 和 `basic` 中施加；LD_PRELOAD 垫片在 `strict` 和 `basic` 中施加。`bubblewrap`/`strict` **不**运行启动器，所以在单纯 `bubblewrap` 下的**静态**扩展二进制完全没有系统调用黑名单 - 在那里约束它的是 bwrap 命名空间 + 文件系统隔离，而不是 seccomp。

被拦截的系统调用，按类别分组：

| 类别 | 被拦截的系统调用 |
|---|---|
| 进程控制 / 调试 | `ptrace`、`process_vm_readv`、`process_vm_writev` |
| 进程信号 | `kill`（两个过滤器都拦）；启动器还直接拒绝 `tgkill`，而 LD_PRELOAD 垫片只在 `tkill`/`tgkill` 指向 PID 1（服务器）时才拒绝 |
| 文件系统挂载 | `mount`、`umount2`、`pivot_root`、`chroot` |
| 命名空间 | `unshare`、`setns` |
| 内核模块 | `init_module`、`finit_module`、`delete_module` |
| BPF / perf | `bpf`、`perf_event_open` |
| 硬件 I/O | `ioperm`、`iopl`（仅 LD_PRELOAD 垫片） |
| 交换分区 / 内存管理 | `swapon`、`swapoff` |
| 系统控制 | `syslog`、`reboot`、`settimeofday`、`clock_settime`、`clock_adjtime`、`adjtimex` |
| Kexec | `kexec_load`（两个过滤器）；`kexec_file_load`（仅 LD_PRELOAD 垫片） |
| setuid 系列 | `setuid`、`setgid`、`setreuid`、`setregid`、`setresuid`、`setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`、`request_key`、`keyctl` |

`execve`/`execveat` **在 LD_PRELOAD 垫片中被拒绝**（这样没有 `subprocess` 的解释型扩展无法创建子进程），但**在启动器的内核过滤器中不被拒绝** - 启动器自己必须 `execve` 扩展。创建出的子进程会继承启动器的过滤器 + Landlock 域，因此无论如何都受约束。

**所有扩展都没有 IP 出站（自 2026-07-11 起）。**每个扩展 - 无论有无网络权限 - 都会被传入 `-deny-inet`（landlock/basic）或 `--unshare-net`（bubblewrap）：扩展进程根本无法打开任何 `socket(AF_INET)` / `socket(AF_INET6)`。需要网络的扩展不直接连接；它通过自己的 `AF_UNIX` HostService 套接字（仍然允许）调用宿主的 `Fetch` 代理，由代理在宿主侧执行 `external_fetch` 白名单和防 SSRF 的连接器。这堵上了按主机的 DNS 白名单和按端口的内核过滤器都堵不上的 IP 字面量出站漏洞（例如 `169.254.169.254`）。见 [ADR-0014](adr/0014-landlock-plugin-sandbox.md)。

局限 - 请仔细阅读：

- **黑名单，不是白名单。** `open`、`read`、`write`、`connect`、`socket`、`unlink`、`mmap`、`mprotect`、`clone` 等全部允许。（`strict` 模式在 bubblewrap 之上增加 LD_PRELOAD 层；真正的系统调用白名单仍在路线图上。）
- **文件系统隔离来自另一层。** seccomp 不约束路径；那是 Landlock（或 bubblewrap）的事。在既没有 Landlock 也没有 bubblewrap 的内核上，扩展可以读取任何人人可读的文件（但读不到服务器的密钥 - 它们受 DAC 保护，见 §3）。
- **网络约束。** 每个扩展的直接 IP 出站都被切断，无论有无网络权限：`-restrict-net`（不授予任何 `-net-connect`）加上 `-deny-inet` 在内核层面阻止所有 `AF_INET`/`AF_INET6` 套接字。没有按端口或按主机的内核过滤器。有网络权限的扩展只能通过自己的 `AF_UNIX` 套接字经宿主的 `Fetch` 代理访问网络，由代理在宿主侧执行 `external_fetch` 白名单。
- **解释型扩展封闭失败。**如果运行器找不到 `libevey-sandbox.so`，而扩展又没有 `subprocess`，[`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) 会拒绝启动它并发出 `plugin.error` 事件。启动器的内核 seccomp 是尽力而为的（没有 seccomp 的内核会退化到其他防御层）。

### 5. 每个扩展独立的宿主 gRPC 套接字 + SO_PEERCRED 认证

宿主为每个扩展在 `/tmp/evey/plugins/<plugin_id>/host.sock` 暴露一个 Unix 套接字。每个套接字都由一个专用的 gRPC 服务器提供服务，其拦截器会：

1. 通过 `getsockopt(SO_PEERCRED)` 读取对端 PID - **这个值由内核提供，无法在用户态伪造**。
2. 把它与运行器在 `cmd.Start` 之后立即用 `SetExpectedPID` 登记的 PID 比较。
3. 通过闭包把扩展 ID 写入 gRPC 上下文（ID 来自监听套接字的身份，绝不来自传输的数据）。

效果：

- 扩展无法通过发送 `x-plugin-id` 请求头冒充另一个扩展（该请求头被忽略）。
- 扩展无法连上另一个扩展的宿主套接字并调用其 RPC - 即使它 `connect()` 成功，PID 检查也会以 `Unauthenticated: PID mismatch` 拒绝。
- 通过 fork/exec 创建子进程的扩展，无法借子进程转发对宿主的调用（PID 不同 → 被拒绝）。这是有意为之，与 `subprocess` 权限关卡一致。

### 6. 管理员预览关卡

`POST /api/v1/admin/plugins/preview` 运行静态分析器，在不安装的情况下返回清单 + 分析结果。管理员会看到：

- 申请的权限（包括 `native-binary`、`subprocess`、`network`、`api.*`）。
- 所有问题，附严重性、文件、行号和类别。
- 按类型分组提取出的 URL（字面量、模板、拼接）。
- 源文件的 SHA256（与即将进行的安装一致）。

管理员是提升权限的信任边界 - Evey 负责展示攻击面，由管理员决定接受还是拒绝。

### 7. 扩展崩溃通知

在以下情况运行器会发布 `plugin.error` 领域事件：

- 沙箱库缺失，而扩展没有 `subprocess`。
- 扩展进程以非零状态退出。
- 认证拦截器记录了一次 PID 不匹配的尝试（仅 warning 级别的 slog - 尚未接入总线）。

通知服务订阅该事件，通过配置的渠道（网页推送 / 邮件 / webhook）通知管理员。

### 8. 服务器进程加固（`PR_SET_DUMPABLE=0`）

服务器启动时调用 `prctl(PR_SET_DUMPABLE, 0)`（[`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go)）。这会把 `/proc/<server>/{mem,environ,maps,fd,…}` 的属主改为 root，使共用服务器 UID 的进程无法读取它们。这在没有 Landlock 的退路中（例如老旧的群晖内核）最为重要，否则扩展可以通过 `/proc` 刮取服务器内存中的 JWT 密钥及其环境变量密钥（`DB_PASSWORD`、`MEILI_MASTER_KEY`）。在有 Landlock 的内核上，扩展本来就访问不了服务器的 `/proc`；这是跨内核的双保险。它不影响服务器读取自己的文件；失败只会被记录，不会致命。

## 沙箱模式

`sandbox_mode` 通过 `evey.yaml` 或 `EVEY_PLUGINS_SANDBOX_MODE` 配置（默认 `auto`）。它选择约束的**后端**，系统调用/出站机制随后端而定：`landlock` 和 `basic` 通过启动器运行扩展，由启动器安装内核 seccomp 黑名单 + `-deny-inet`；`bubblewrap`/`strict` 则用命名空间隔离网络（`--unshare-net`），没有启动器 seccomp（`strict` 只额外加上仅对动态扩展有效的 LD_PRELOAD 垫片）。**所有**模式都适用的是：UID 隔离、用户态资源监控，以及**直接 IP 出站被切断**（通过 `-deny-inet` 或 `--unshare-net`），所以需要网络的扩展只能经由宿主的 `Fetch` 代理出站。运行器在 [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go) 中选择后端；`auto` 在第一个扩展启动时解析一次（[`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go)）。

### `auto`（默认）

按以下顺序解析出主机实际支持的最佳后端：**landlock → bubblewrap → basic**。

- 优先 **landlock**：它不需要特权，在 Docker 默认的 seccomp + AppArmor 下运行、主机无需任何改动，而且一旦施加就无法放宽 - 对主要的容器化部署来说是正确的默认值。
- 只有在没有 Landlock，*并且*一个临时的 bwrap 沙箱确实能搭起来时（[`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) - 具备可用 userns + mount 的裸机 / 特权容器），才退回 **bubblewrap**。
- 否则用 **basic**。解析出的模式会在第一个扩展启动时记录下来。

### `landlock`

- **约束：**通过启动器施加的 Landlock LSM 白名单约束文件系统 - 系统目录（`/usr`、`/lib`、`/lib64`、`/bin`、`/sbin`、`/etc`、`/app`、`/dev`）只读/可执行；扩展自己的安装目录 + 其套接字目录可读写；其余一切（尤其是 `/data`、相邻扩展的目录）拒绝。网络：**任何扩展都没有直接 IP 出站** - `-restrict-net`（不授予 `-net-connect`）加 `-deny-inet` 阻止所有 `AF_INET`/`AF_INET6` 套接字；有网络权限的扩展只能通过 `AF_UNIX` 套接字经宿主的 `Fetch` 代理访问网络。此外还有：内核 + LD_PRELOAD 的 seccomp 黑名单。
- **需要：**内核 ≥ 5.13 且 Landlock 在活动 LSM 列表中，以及 `evey-plugin-launcher` 二进制。无需 mount、无需 unshare、无需特权 - 在 Docker 默认的安全配置下干净运行。
- **何时选用：**`sandbox_mode=landlock`（无法强制执行 Landlock 时扩展封闭失败），或 `auto` 解析为它（官方镜像的常规情况）。
- **局限：**每个扩展的直接 IP 出站都被切断（`-deny-inet` 在 Landlock ABI <4 上依然成立）；按主机的白名单现在位于宿主侧的 `Fetch` 代理中，而不在内核里。文件系统约束在任何情况下都成立。

### `bubblewrap`

- **约束：**通过 6 个隔离的命名空间（`--unshare-user/-pid/-uts/-ipc/-cgroup/-net` - `--unshare-net` 无条件作用于**每个**扩展；直接出站经由代理）和一份挂载白名单来约束文件系统 + 网络 + 进程视图 - 只读的系统目录、可写的扩展目录 + 套接字目录、合成的 `/proc` 和最小化的 `/dev`、tmpfs 的 `/tmp`；`/data`、`/var`、`/home`、`/root`、相邻目录根本不会被挂载。`applySandbox` 把这个模式交给 `applyBwrapSandbox`，后者从不调用启动器：启动器的内核 seccomp 在这里**不**生效，LD_PRELOAD 垫片也不生效（那只属于 `strict`）。系统调用约束完全来自隔离的命名空间。
- **需要：**`bwrap` 二进制，以及 `mount`/`unshare` 系统调用；在 Docker 里这需要放宽安全限制，所以实际上只适用于裸机（或特权容器）。
- **何时选用：**`sandbox_mode=bubblewrap`，或在没有 Landlock 且 bwrap 探测通过的主机上由 `auto` 选用。
- **局限：**`--unshare-cgroup` 是一个 cgroup *命名空间*，不是资源限制（资源限制来自用户态监控）。这个模式下不施加任何内核 seccomp 黑名单（见上文）；也不使用 `-deny-inet`，因为 `--unshare-net` 已经阻止了所有 IP 套接字。

### `strict`

- 与 `bubblewrap` 相同，**再加上** LD_PRELOAD seccomp 库。需求与局限同 bubblewrap。（真正的系统调用*白名单*仍在路线图上。）

### `basic`

- **约束：**降到 UID 1001 + 内核 seccomp 黑名单 + `-deny-inet` + 尽力而为的 Landlock - 全部通过用启动器包裹扩展来实现，前提是启动器二进制存在（官方镜像中有）。如果缺少启动器（某些开发镜像），就退化为只有 LD_PRELOAD + 降权。
- **何时选用：**`sandbox_mode=basic`，或 `auto` 退到它（没有 Landlock 且 bubblewrap 不可用 - 例如被锁死的老内核）。
- **局限：**这是*文件系统*约束最弱的模式：内核没有 Landlock 时，**没有按路径的文件系统约束**，扩展可以读取人人可读的非敏感文件，`/proc` 中的进程*列表*可见。但它**并不是**"没有隔离"：服务器密钥仍受 DAC 保护（UID 隔离 + `PR_SET_DUMPABLE=0`），扩展没有 IP 出站（`-deny-inet`），危险的系统调用在内核层面被拦截（包括静态二进制），资源也受监控器限制。

### 态势总结

| 部署方式 | 文件系统约束 | 网络 | 危险系统调用 | 密钥 | 结论 |
|---|---|---|---|---|---|
| Docker + Landlock（`auto`→landlock） | 按路径白名单 | 没有直接出站（`-deny-inet`）；经代理 | 被拦截（含静态） | 受保护 | **强** |
| 没有 Landlock 的 Docker（如老旧 NAS，`auto`→basic） | 无（人人可读的内容可见） | 没有 IP 出站（`-deny-inet`） | 被拦截（含静态） | 受保护（DAC + DUMPABLE + seccomp） | 还不错，但不等同于 Landlock |
| 裸机（`auto`→bubblewrap） | 命名空间 + 挂载白名单 | 网络命名空间 | 命名空间（PID/mount/net）隔离；`strict` 中有 LD_PRELOAD seccomp - bwrap 模式下*不*施加启动器的内核过滤器 | 受保护 | **最强** |

## 沙箱的已知局限

| 局限 | 影响 | 目前的缓解措施 | 路线图 |
|---|---|---|---|
| `basic` 模式下（没有 Landlock 的主机）没有按路径的文件系统约束 | 扩展可以读取人人可读的非敏感文件；相邻扩展的目录也可见 | 服务器密钥仍受 DAC 保护（UID 隔离 + `PR_SET_DUMPABLE=0`）；分析器会标记 `/etc/passwd` 这类模式；`auto` 在可用时优先 Landlock/bubblewrap | -（答案就是 Landlock 或 bubblewrap；两者都已随附） |
| 所有扩展共用 UID 1001 | 恶意扩展可以向同伴发信号（向 PID 1 / 服务器发信号被 seccomp 拦截） | `PR_SET_DUMPABLE=0` 关闭了同 UID 进程之间对 `/proc/<pid>/{environ,mem,maps,fd}` 的读取；Landlock/bubblewrap 把各扩展目录完全隔离 | 通过 bubblewrap 为每个扩展提供独立的用户命名空间 |
| seccomp 是黑名单而非白名单 | 扩展可以自由地 `open`/`read`/`write`/（AF_UNIX）`socket`/`connect`；ptrace、mount、命名空间、bpf、kexec、加载模块、setuid 系列、keyring 以及 IP 套接字（所有扩展）都在内核层面被拦截，包括静态二进制 - 见 [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | 通过启动器施加的内核 seccomp（exec 之前，覆盖静态二进制）+ `-deny-inet`；`subprocess` 权限关卡 | 为 `strict` 扩展提供真正的系统调用白名单模式 |
| 代理把 SSRF 攻击面转移到了宿主 | 代理的 URL/IP 校验若有 bug，扩展可能经由宿主访问到内部地址 | 代理用 Go 执行 `external_fetch` 白名单 + 防 SSRF 的连接器（连接时拒绝私有/保留 IP）；运维者**应当**再加一层独立的容器出站防火墙（丢弃 `169.254.0.0/16`，只放行数据库 + Meili） | 按命名空间的 nftables / 代理专用出站（V2） |
| 正则分析器可被混淆绕过 | 有决心的攻击者可以隐藏模式 | 管理员预览 + 权限审查；强制边界是沙箱，而不是分析器 | 服务器端风险评分 + 扩展作者的信誉检查 |

## `sandbox_mode` 配置

可通过 `evey.yaml`（或环境变量 `EVEY_PLUGINS_SANDBOX_MODE`）配置；按模式的详细说明见上面的[沙箱模式](#sandbox-modes)一节：

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION`（`auto` | `on` | `off`，默认 `auto`）是一个独立的开关，控制扩展是否以不同的 UID 运行（见 [§3](#3-uid-separation-dac)）：`auto` 只在模式解析为 `basic` 时启用，`on` 在所有地方强制启用（额外的独立防线），`off` 关闭。由启动器实现，在 `basic` 中封闭失败（只在 `landlock` 下尽力而为）；与 `no_new_privs` 不兼容。按场景的讲解：[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)。

按模式对照（所有取值都取决于主机能力 - 例如 Landlock 需要内核 ≥ 5.13 且 Landlock 在活动 LSM 列表中）：

| 防御层 | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| UID 隔离（1001）- `EVEY_PLUGINS_UID_SEPARATION`，默认 `auto` | 仅在 basic 中开启 | 关（设为 `on` 可强制） | 开 | 通过 userns¹ | 通过 userns¹ |
| 内核 seccomp（启动器，exec 之前，覆盖静态二进制） | 解析为 landlock/basic 时有；解析为 bubblewrap 时没有 | 有 | 有 | 没有（bwrap 没有启动器 seccomp；`applySandbox` 把 bubblewrap/strict 交给 `applyBwrapSandbox`，后者从不调用启动器） | 没有（strict 只在 bwrap 之上加 LD_PRELOAD 垫片 - 仅限动态扩展，没有内核 seccomp） |
| LD_PRELOAD seccomp（解释型扩展） | 有 | 有 | 有 | 没有 | 有 |
| `-deny-inet`（任何扩展都没有 IP 套接字） | 有 | 有 | 有 | 不适用（网络命名空间） | 不适用（网络命名空间） |
| 文件系统约束 | 解析为可用的最佳方案 | Landlock 白名单 | 无（仅 DAC） | 挂载白名单 | 挂载白名单 |
| 网络（所有扩展：没有直接 IP 出站；经宿主 `Fetch` 代理出站） | `-deny-inet` | `-restrict-net` + `-deny-inet` | `-deny-inet` | `--unshare-net` | `--unshare-net` |
| 命名空间（user/pid/uts/ipc/cgroup） | 不适用 | 不适用 | 不适用 | 隔离 | 隔离 |
| 资源上限 | 用户态监控 + `nice` | 用户态监控 + `nice` | 用户态监控 + `nice` | 用户态监控 + `nice` | 用户态监控 + `nice` |

¹ 在 `bubblewrap`/`strict` 中不使用启动器，所以 `cap_setuid` 降权不适用；由隔离的**用户命名空间**提供 UID 隔离。

注意：`strict` 的真正系统调用*白名单*仍在路线图上；目前 `strict` = bubblewrap + LD_PRELOAD 黑名单。

实现依赖：

- `evey-plugin-launcher`（纯 Go，无 CGO），随服务器二进制一起发布 - 负责施加 Landlock + 内核 seccomp 过滤器。`landlock`、`auto` 和 `basic` 使用它。
- `bubblewrap`，在 `Dockerfile` 的运行时阶段用 `apt-get` 安装（Debian 软件包，约 200 KB）；`bubblewrap`/`strict` 使用它。
- 对 bubblewrap：主机内核需启用无特权用户命名空间（在存在该 sysctl 的系统上为 `kernel.unprivileged_userns_clone=1`）。在 Docker 里，搭建命名空间/挂载通常需要 `--security-opt seccomp=unconfined` 或放宽的 AppArmor - 这就是为什么 `auto` 对容器化部署优先选择 Landlock。

## 扩展的网络出站（经代理，自 2026-07-11 起）

扩展**没有直接的网络访问**：沙箱对每个扩展拒绝所有 IP 套接字（`--unshare-net` / `-deny-inet`），
与 `network` 权限无关。需要网络的扩展通过自己的 `AF_UNIX` 宿主套接字调用宿主的
`HostService.Fetch` RPC，该调用受 `network` 权限把关。随后宿主
（`internal/adapters/driving/pluginhost/fetch.go`）用 Go：

- 检查目标主机是否在扩展声明的 `external_fetch` 白名单中（现在这是真正的按主机契约，而非
  尽力而为的 DNS 提示）- 精确主机名，不区分大小写匹配（匹配器也认得 `*.domain` 形式，但
  安装器会拒绝通配符条目，所以从来不会有条目到达它）；
- 把协议限制为 `http`/`https`，方法限制为 `GET`/`HEAD`；
- 通过防 SSRF 客户端（`internal/pkg/ssrf`）拨号，它在连接时、DNS 解析**之后**拒绝私有/保留/
  链路本地 IP，因此 DNS 重绑定和重定向都会被重新校验；
- 不附带服务器自己的任何凭据，并限制响应体大小。

**运维加固（推荐）。**代理把 SSRF 攻击面转移到了宿主，所以请在 `evey-server` 容器上再加一层
不信任代码的独立出站防线：丢弃 `169.254.0.0/16`（云元数据 / 链路本地），并把内部子网限制为
只能访问 Postgres + Meilisearch 端点。这样即使代理的校验有 bug，也仍会撞上一道网络墙；两层
防线的失效方式互不相关。

**代理覆盖不到的部分。**早期 DNS 白名单的绕过方式（IP 字面量、扩展自带解析器、通过 SNI/Host
技巧复用已解析的连接）都属于直接出站，已随之消失：扩展进程根本无法打开 IP 套接字。剩下的就是
扩展安全模型里已经说明的那条局限：代理控制调用*去哪里*，而不管它*带了什么*，所以数据仍可能
流向白名单中的主机。

**缓解建议**：
- 把 `network` 权限当作"需要人工审计"：只放行你愿意托付扩展数据的主机，并看看扩展往那里发了
  什么。
- 运维者应监控运行扩展的主机的出站流量（Prometheus + 流量导出器，或者简单地
  `ss -tnp | grep <plugin-pid>`）。

**V2 路线图**（1.0 之后）：
- 容器级 nftables 出站防火墙，作为代理之后的独立防线。
- 可选的出站 HTTP 代理，提供请求级检查。

## 网络暴露（扩展威胁模型之外）

以上内容都与扩展有关。这台服务器攻击面的另一半是"谁能访问到它"，由两条不变的原则决定。之所以
在这里写出来，是因为想了解"如何安全地暴露 Evey"的读者会从这个文件开始读：

- **只有一个监听端口是用来对外发布的，而且不是 gRPC 那个。**原生 gRPC 端口（`EVEY_SERVER_PORT`，
  默认 9090）是内部传输通道，唯一的客户端是同一进程内的 HTTP 网关；HTTP API 是该端口**加一**。
  gRPC 监听默认绑定在回环地址（`EVEY_GRPC_BIND=127.0.0.1`），正是因为下面的远程访问关卡是 HTTP
  中间件，覆盖不到它。扩大这个绑定是可以的，启动时会记录一条警告；之后用防火墙保护它就是运维者
  的责任了。
- **远程访问关卡依据在受信任代理边界上校验过的那唯一一个客户端 IP 来判断 LAN 还是 WAN，绝不依据
  请求头。**客户端无法通过 `X-Forwarded-For` 声称自己是 `127.0.0.1` 来被当作本地；从未穿过该
  边界的请求会被归为 WAN（拒绝），而不是被信任。注意这是一项*部署*控制（"这个实例只给我自己的
  网络用"），而不是认证层 - 保护数据的是认证。

详情和完整的变量列表：[configuration.md](configuration.md#remote-access-who-may-connect-from-outside-your-network)
和 [deployment-bare-metal.md](deployment-bare-metal.md)。

## 日志里可能有什么

服务器日志文件就是管理后台日志接口（跟踪、搜索、实时查看）所提供的内容，也会进入错误报告、被
备份带走，因此凭据会在**写入时**被清除（`internal/pkg/logger/redact.go`）：键名表示密钥的属性
（`authorization`、`token`、`access_token`、`refresh_token`、`api_key`、`password`、`pin`、
`secret`……）会被整体替换，任何字符串 - 包括消息本身 - 只要带有 bearer、JWT 或 `secret=value`
这样的键值对，都会被清理。对于在这项清理出现之前写入的行，管理后台日志接口在输出时也会应用
同样的规则。对代码的两点影响：非机密的值不得以机密键名记录（它会被掩码 - 目前使用的名字是
`token_info`、`device_pin`），而新的凭据类型必须在第一次被记录之前就加入这份列表。

## 报告漏洞

请私下报告，永远不要在公开 issue 中报告：`security@eveyproject.org`。这是代码仓库
[安全策略](../SECURITY.md)给出的地址，扩展的认证、权限和沙箱逃逸问题与其他问题一样都适用。
请不要对不属于你的共享或公开 Evey 实例进行测试。
