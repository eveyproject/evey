---
source_sha: 0ac15ab64c61bb43ae41b6ec652dda6261a8eed8431ec891b4f9ad3b33dd7d21
reviewed: false
---
# 插件沙箱与威胁模型

本页说明服务器为限制插件所做的事情：安装时的检查、运行时的各层防护、沙箱模式，以及每一项仍然无法阻止什么。如果你在编写插件，想知道可以声明什么以及什么会被拒绝，请阅读[权限与插槽](plugins/security-model.md)。

## 威胁模型

插件作为独立的进程运行，通过 Unix socket 上的 gRPC 与服务器通信。服务器默认不信任任何插件代码。它所防御的风险，从最严重到最轻：

1. **冒充**：一个插件假装成另一个插件来使用其权限。
2. **权限提升**：插件逃出沙箱，以服务器的身份执行命令。
3. **文件系统访问**：插件读写自己目录之外的内容，尤其是服务器的机密。
4. **网络外泄**：插件把数据发送到它选择的主机。
5. **资源滥用**：挖矿、fork 炸弹、耗尽内存或磁盘。
6. **供应链**：分析无法读懂的混淆代码或编译后的代码。

## 安装时

### ZIP 解压

在写入任何文件之前：

- 最多 5,000 个条目、单个文件 100 MB、解压后总计 500 MB，以及在解压后超过 1 MB 时 100:1 的压缩比（解压炸弹）；
- 路径超出压缩包（`..`）的条目会被拒绝；
- 符号链接会被拒绝；
- setuid、setgid 和 sticky 位会被去除。

来源：`internal/adapters/driven/pluginrunner/zip.go`。

### 清单与权限

清单声明插件请求的权限。未知的权限会导致安装被拒绝。之后这些权限会被强制执行：

- **`subprocess`** 允许子进程。没有它时，内核会拒绝创建进程（`-deny-fork`），`LD_PRELOAD` 过滤器还会为解释型插件拒绝 `execve`。
- **`native-binary`** 允许附带编译后的代码或压缩包，分析无法读懂它们。
- **API 权限**（`api.library`、`api.user`、`api.metadata`、`media.read`、`media.write`、`notification.sender`……）在每次调用宿主时由 [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go) 检查。

完整的权限列表见[权限与插槽](plugins/security-model.md#permissions-catalog)。

### 静态分析

包中的每个文件都会由 [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go) 扫描，扫描之前会先进行一次规范化处理，还原常见的混淆手法（别名、字符串拼接、f-string）：

| 类别 | 严重程度 | 示例 |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`、`subprocess.run`、`child_process.exec`、PHP `system()`/`shell_exec()`/`proc_open()`、`preg_replace /e`、Ruby 反引号、`open('\|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`、`fetch`、反向 shell 和绑定 shell |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`、`.ssh/`、`rm -rf`、符号链接操作 |
| `obfuscation` | CRITICAL | `eval`、base64 解码、十六进制序列、`__builtins__`、`base64 -d \| bash` |
| `privilege` | CRITICAL | `setuid`、`chmod 4777`、`sudo`、向 PID 1 发送信号 |
| `cryptomining` | CRITICAL | `stratum+tcp`、`xmrig`、`cryptonight` |
| `runtime_install` | CRITICAL | `pip install`、`npm install`、`curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`、`importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`、`process.binding`、`new Function`、`__subclasses__()`、`__mro__` |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`、`marshal.loads`、`__reduce__`；WARNING：不带 Loader 的 `yaml.load()` |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`、`ffi.Library`；WARNING：Go 的 `unsafe` |
| `compiled_binary` | WARNING，然后 BLOCK | `.so`、`.dll`、`.exe`、`.wasm`、`.jar` |
| `embedded_archive` | WARNING，然后 BLOCK | `.zip`、`.tar.gz` |
| `mime_mismatch` | 按 CRITICAL 处理 | 扩展名声称是图片，内容却是别的东西 |
| `suspicious_url` | WARNING | URL 字面量中的非 HTTP 协议 |
| `unvendored_deps` | WARNING | 有 `requirements.txt` 或 `package.json` 却没有附带模块 |

CRITICAL 发现或 `mime_mismatch` 会拒绝安装。`compiled_binary` 和 `embedded_archive` 会拒绝安装，除非清单声明了 `native-binary`。

**分析无法保证什么。** 它基于模式工作。它能识别已知的混淆类别（分多步把字符串拼接成可调用对象、按名称查找内置函数、`Function.constructor` 逃逸），每一类在 `tests/fixtures/plugins/malicious/` 中都有回归用例，但它无法追踪运行时完成的每一种变换（`strrev("metsys")`、解码从文件读取的载荷）。它也会匹配注释中的内容：注释 `# pip install foo` 会触发 `runtime_install`。**真正的防线是运行时沙箱，而不是分析**：一个让某种模式逃过分析的插件，仍然无法读取服务器的数据、发起被拒绝的系统调用、超出资源限制，或者调用它没有权限的宿主方法。

### 签名

包可以用 cosign 签名。`EVEY_PLUGINS_COSIGN_POLICY` 决定验证失败时怎么做：

| 策略 | 效果 |
|---|---|
| `off` | 不检查签名。 |
| `warn`（默认） | 记录失败并显示给管理员，但不阻止。没有配置公钥时，不验证任何东西。 |
| `enforce` | 失败会拒绝安装或更新。 |

公钥由 `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` 提供。来源：`internal/adapters/driven/pluginrunner/cosign.go`。

### 管理员预览

`POST /api/v1/admin/plugins/preview` 只运行分析而不安装。管理员会看到请求的权限、每个发现及其文件、行号和类别、代码中发现的 URL，以及包的 SHA256；应用安装时会检查同一个哈希，所以被分析的包就是被安装的包。更新会显示权限、插槽和主机的差异，新权限需要明确确认。

管理员是高级权限的信任边界：Evey 展示攻击面，管理员接受或拒绝。

## 运行时

### 文件系统

插件只能看到自己的目录和它的 socket 目录，看不到服务器的数据。如何强制执行取决于[沙箱模式](#sandbox-modes)：Landlock 允许列表，或者 bubblewrap 挂载白名单。在没有 Landlock 的内核上的 `basic` 模式中，没有按路径的隔离：此时保护服务器机密的是单独的用户和文件权限（见下文）。

### 网络

无论声明什么，插件都**没有直接的网络访问**：任何 IP socket 都会被拒绝（`landlock` 和 `basic` 中为 `-deny-inet`，`bubblewrap` 和 `strict` 中为网络命名空间）。拥有 `network` 权限的插件通过其 Unix socket 调用宿主的 `Fetch` 方法。宿主在 `internal/adapters/driving/pluginhost/fetch.go` 中：

- 对照插件的 `external_fetch` 列表检查目标（精确主机，不区分大小写）；
- 只允许 `http` 和 `https`，只允许 `GET` 和 `HEAD`；
- 通过一个防 SSRF 的客户端（`internal/pkg/ssrf`）连接，它在 DNS 解析后拒绝私有、保留和链路本地地址，所以 DNS 重绑定和重定向都会被再次检查；
- 不附加任何服务器的凭据，并限制响应的大小。

这堵住了直接出站留下的漏洞（`169.254.169.254` 之类的字面 IP、插件自己的解析器）。仍然存在的是：代理控制的是调用*去哪里*，而不是它*携带什么*。见[已知限制](#known-limits)。

**给管理员的建议。** 代理把 SSRF 攻击面转移到了宿主上。在 `evey-server` 容器上加一条与代码无关的出站规则，可以增加第二道墙：丢弃 `169.254.0.0/16`，并把内部网络限制为数据库和搜索的端点。

### 系统调用

两个 seccomp 过滤器拒绝同样的危险系统调用：

1. **内核过滤器**，由 [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) 在启动插件之前安装（[`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)）。插件及其子进程都会继承它，并且它**也覆盖编译后的静态二进制文件**。系统调用中架构不匹配会杀死进程。在 `landlock` 和 `basic` 中应用。
2. **`LD_PRELOAD` 过滤器**，[`sandbox/sandbox.c`](../sandbox/sandbox.c)，加载到没有声明 `subprocess` 的解释型插件中。它还会拒绝 `execve`/`execveat`，并把插件标记为不可转储。在 `landlock`、`basic` 和 `strict` 中应用。如果缺少这个库，这样的插件会被拒绝启动（`plugin.error`）。

被拒绝的系统调用：

| 类别 | 系统调用 |
|---|---|
| 调试 | `ptrace`、`process_vm_readv`、`process_vm_writev` |
| 信号 | `kill`；内核过滤器还拒绝 `tgkill`，`LD_PRELOAD` 过滤器拒绝针对 PID 1 的 `tkill`/`tgkill` |
| 挂载 | `mount`、`umount2`、`pivot_root`、`chroot` |
| 命名空间 | `unshare`、`setns` |
| 内核模块 | `init_module`、`finit_module`、`delete_module` |
| BPF、perf | `bpf`、`perf_event_open` |
| 硬件 I/O | `ioperm`、`iopl`（仅 x86-64，存在的地方） |
| 交换区 | `swapon`、`swapoff` |
| 系统控制 | `syslog`、`reboot`、`settimeofday`、`clock_settime`、`clock_adjtime`、`adjtimex` |
| Kexec | `kexec_load`、`kexec_file_load` |
| 内核漏洞利用原语 | `io_uring_setup`、`io_uring_enter`、`io_uring_register`、`userfaultfd` |
| 身份 | `setuid`、`setgid`、`setreuid`、`setregid`、`setresuid`、`setresgid`、`capset` |
| 密钥环 | `add_key`、`request_key`、`keyctl` |

对于没有 `subprocess` 的插件，内核还会拒绝创建进程（`fork`、`vfork`、不带 `CLONE_THREAD` 的 `clone`）；线程仍然可以工作。

这是一个**拒绝列表**：`open`、`read`、`write`、`mmap`、Unix `socket` 和 `connect` 仍然允许。路径由文件系统层限制，而不是由 seccomp 限制。在 `bubblewrap` 中不使用启动器：静态二进制文件在那里没有 seccomp 过滤器，由命名空间和挂载白名单来限制。

### 单独的用户

插件可以以用户 1001（`evey-plugin`）运行，与服务器（1000）不同，这样仅凭文件权限，服务器的机密对它来说就是不可读的。由 `EVEY_PLUGINS_UID_SEPARATION` 控制：

- `auto`（默认）：只在沙箱解析为 `basic` 时启用，这是文件权限成为机密周围唯一一道墙的唯一情况；
- `on`：始终启用，作为额外的一层；
- `off`：从不启用。

切换由持有 `cap_setuid` 的启动器完成；服务器本身在没有特权和能力的情况下运行。启动器拒绝任何低于 1000 的目标。如果它无法切换，`basic` 中的插件会被拒绝（在那里用户是唯一的墙），而在 `landlock` 中它会以服务器的用户运行并给出警告。在 `bubblewrap` 和 `strict` 中，用户命名空间承担这个角色。

启用时，签名密钥（`0600`）和备份（目录 `0700`，文件 `0600`）在运行时只有所有者可以访问，即使 `/data` 是从主机绑定挂载的也是如此。所有插件共用用户 1001：它们之间由文件系统层隔离，而不是由用户隔离。

### 宿主 socket 认证

每个插件都有自己的 socket，`/tmp/evey/plugins/<plugin_id>/host.sock`，由它自己的 gRPC 服务器提供服务。每次调用时，服务器都从内核读取调用者的 PID（`SO_PEERCRED`，用户空间无法伪造），并与插件启动时记录的 PID 进行比较。插件的身份来自 socket，而从不来自请求头。因此，插件无法冒充另一个插件、调用另一个插件的 socket，或者通过子进程转发调用。

### 资源限制

一个用户空间的监视器（在无特权的容器中无法使用 cgroups）大约每 2 秒读取一次 `/proc/<pid>`，并停止超出限制的插件：

- 内存（RSS），默认 256 MiB；
- 进程和线程，默认 64；
- 其目录使用的磁盘，默认 100 MiB，每 30 秒检查一次；
- CPU 优先级，默认 `nice` 10，在启动时应用。

限制在 **Admin → 插件** 中按插件设置。见 [ADR-0013](adr/0013-userspace-plugin-resource-monitor.md)。

### 界面隔离

插件的界面运行在一个 `<iframe sandbox="allow-scripts">` 中，没有同源访问、表单或弹出窗口，并受一个只允许连接到其 `external_fetch` 主机的 Content-Security-Policy 约束。桥接限制了它能向页面请求的内容：`toast` 每秒最多 3 条、200 个字符、不能包含 HTML；`navigate` 只接受 `/home`、`/library`、`/media`、`/player`、`/settings` 和 `/search`；`resize` 在 50 到 800 px 之间。插槽在安装时检查，渲染时会再次检查。

### 服务器加固

服务器在启动时把自己标记为不可转储（`PR_SET_DUMPABLE=0`，[`hardening_linux.go`](../cmd/evey/hardening_linux.go)）：以同一用户运行的进程无法通过 `/proc` 读取它的内存或环境，这保护了内存中的签名密钥以及数据库和搜索的密码，即使在没有 Landlock 的内核上也是如此。

### 崩溃通知

当需要沙箱库的插件缺少该库，或者插件以错误退出时，运行器会发布一个 `plugin.error` 事件。通知服务会通过配置的渠道把它转发给管理员。宿主 socket 上的 PID 不匹配会被记录为警告。

## 沙箱模式

模式决定文件系统和网络如何被限制。`EVEY_PLUGINS_SANDBOX_MODE` 设置初始值（默认 `auto`）；可以在 **Admin → 插件** 中更改。运行器在 [`applySandbox`](../internal/adapters/driven/pluginrunner/runner_sandbox.go) 中应用它。在所有模式下：没有直接的 IP 出站、有资源监视器，以及宿主 socket 认证。

### `auto`（默认）

在第一个插件启动时解析一次，得到主机支持的最强模式：先 **landlock**，再 **bubblewrap**，最后 **basic**。Landlock 排在第一位，因为它不需要特权，并且能在 Docker 的默认安全配置下运行。只有当 Landlock 不存在且一个测试沙箱确实能建立时，才会选择 bubblewrap（[`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go)），这在实践中意味着裸机或特权容器。解析得到的模式会被记录，并显示在 **Admin → 插件 → 安全仪表板** 的 **主机沙箱** 一行中。

### `landlock`

- **文件系统：** 由启动器应用的 Landlock 允许列表。系统目录（`/usr`、`/lib`、`/lib64`、`/bin`、`/sbin`、`/etc`、`/app`）只读；`/dev` 中只有 `null`、`zero`、`full`、`random` 和 `urandom`，所以显卡节点和服务器的终端都无法触及；插件的目录和 socket 目录可读写；其他一切，包括 `/data` 和其他插件，都被拒绝。被授予 `hardware.gpu` 的插件会把它的渲染节点作为唯一的例外。
- **网络：** `-restrict-net` 和 `-deny-inet`；只能通过 `Fetch` 出站。
- **系统调用：** 内核过滤器和 `LD_PRELOAD` 过滤器。
- **要求：** 5.13 或更新、且 Landlock 位于活动 LSM 列表中的内核，以及启动器（在较新的主机上使用官方镜像时两者都满足）。显式的 `landlock` 会拒绝启动它无法隔离的插件。

### `bubblewrap`

- **文件系统和进程：** 六个独立的命名空间（`user`、`pid`、`uts`、`ipc`、`cgroup`、`net`）和一个挂载白名单；内部不存在其他任何东西。

  | 路径 | 访问 |
  |---|---|
  | `/usr`、`/bin`、`/sbin`、`/lib`、`/lib64` | 只读 |
  | `/etc/ssl`、`/etc/ca-certificates`、`/etc/resolv.conf`、`/etc/nsswitch.conf`、`/etc/{alpine,os}-release` | 只读（从不是整个 `/etc`：没有 `passwd`、`shadow`、`ssh`） |
  | 插件的目录、它的 socket 目录 | 可读写 |
  | `/proc` | 它自己的挂载 |
  | `/dev` | 一组最小的合成设备（null、zero、random、urandom、tty、pts） |
  | `/tmp` | 一个空的 tmpfs |

  `/data`、`/var`、`/home`、`/root`、`/run` 和其他插件的目录在内部都不存在。一个金丝雀插件（`tests/fixtures/plugins/canary-sandbox-runtime/`）会针对运行中的环境检查这一布局。来源：`internal/adapters/driven/pluginrunner/runner_bwrap.go` 中的 `buildBwrapCmd`。
- **网络：** 网络命名空间，对所有插件。
- **系统调用：** 没有 seccomp 过滤器；由命名空间提供隔离。
- **要求：** `bwrap` 二进制文件以及 `mount`/`unshare` 系统调用。在 Docker 中这需要放宽的安全配置，所以它实际上是裸机模式。

### `strict`

`bubblewrap` 加上 `LD_PRELOAD` 过滤器（仅限解释型插件）。

### `basic`

`auto` 的最后手段：单独的用户、内核过滤器、`LD_PRELOAD` 过滤器、`-deny-inet`，以及内核支持时的 Landlock。在没有 Landlock 的内核上没有按路径的隔离：插件可以读取所有人可读的非敏感文件，并看到进程列表。服务器的机密仍然由单独的用户和 `PR_SET_DUMPABLE` 保护，网络被切断，危险的系统调用被拒绝，包括编译后的二进制文件。

### 总结

| 层 | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|
| 文件系统 | Landlock 允许列表 | 没有 Landlock 时无（仅文件权限） | 挂载白名单 | 挂载白名单 |
| 网络 | `-restrict-net` + `-deny-inet` | `-deny-inet` | 网络命名空间 | 网络命名空间 |
| 内核 seccomp（也覆盖静态二进制文件） | 是 | 是 | 否 | 否 |
| `LD_PRELOAD` seccomp（没有 `subprocess` 的解释型插件） | 是 | 是 | 否 | 是 |
| 单独的用户 | 设为 `on` 时 | 是（`auto`） | 用户命名空间 | 用户命名空间 |
| 资源限制 | 是 | 是 | 是 | 是 |

| 部署方式 | 你得到的 |
|---|---|
| 较新内核上的 Docker（`auto` 得到 `landlock`） | 按路径的隔离、没有直接出站、危险系统调用被拒绝（包括静态二进制文件）。常见情况。 |
| 没有 Landlock 的旧内核上的 Docker，例如老旧 NAS（`auto` 得到 `basic`） | 机密受保护、网络被切断，但所有人可读的文件是可见的。见[插件沙箱设置](plugins/sandbox-deployment.md)。 |
| 裸机（`auto` 得到 `bubblewrap`） | 最强的文件系统和进程隔离，但没有针对静态二进制文件的 seccomp 过滤器。 |

## 配置

| 变量 | 默认值 | 作用 |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`、`landlock`、`bubblewrap`、`strict` 或 `basic`。也可以在 **Admin → 插件** 中修改。 |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`、`on` 或 `off`。与容器上的 `no_new_privs` 不兼容。 |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`、`warn` 或 `enforce`。 |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | 无 | 用于检查签名的密钥。 |

官方镜像附带 `evey-plugin-launcher`（Landlock 和内核过滤器）以及 `bubblewrap`。bubblewrap 还需要主机上的无特权用户命名空间。哪种设置适合哪种机器，通俗的说明见：[插件沙箱设置](plugins/sandbox-deployment.md)。

## 已知限制

| 限制 | 影响 | 目前的缓解措施 |
|---|---|---|
| 在没有 Landlock 的内核上，`basic` 中没有按路径的隔离 | 插件可以读取所有人可读的非敏感文件以及其他插件的目录 | 机密由单独的用户和 `PR_SET_DUMPABLE` 保护；`auto` 优先选择 Landlock 或 bubblewrap |
| 所有插件共用用户 1001 | 插件可以向同类插件发送信号（向服务器发送信号会被拒绝） | `PR_SET_DUMPABLE` 阻止通过 `/proc` 读取其他进程；Landlock 和 bubblewrap 隔离插件目录 |
| Seccomp 是拒绝列表 | 文件和 Unix socket 调用仍然允许 | 路径由 Landlock 或 bubblewrap 限制；IP socket 被拒绝 |
| 代理把 SSRF 攻击面转移到了宿主 | 代理验证中的缺陷可能访问到内部地址 | Go 实现的允许列表和防 SSRF 客户端；容器上的出站规则作为独立的一层 |
| 代理控制调用去哪里，而不控制它携带什么 | 数据可以流向插件列表中的某个主机 | 只允许你愿意托付这些数据的主机 |
| 分析基于模式 | 刻意的混淆可以隐藏代码 | 管理员预览；防线是沙箱，而不是分析 |
| `LD_PRELOAD` 只覆盖动态的解释型插件 | 没有启动器的 `basic` 中的静态二进制文件可以绕过它 | 官方镜像总是附带启动器，其内核过滤器覆盖静态二进制文件 |

## 网络暴露（插件威胁模型之外）

两条不变量决定谁能连到服务器：

- **只有 HTTP 端口应该被发布。** gRPC 端口（`EVEY_SERVER_PORT`，默认 9090）是一个内部传输通道，它唯一的客户端是同一进程中的 HTTP 网关；HTTP API 是该端口加一。gRPC 默认绑定到环回地址（`EVEY_GRPC_BIND=127.0.0.1`），因为远程访问限制是 HTTP 中间件，不覆盖它。放宽绑定会记录一条警告；此时用防火墙保护它就是管理员的责任了。
- **远程访问限制依据在受信任代理边界上验证过的那一个客户端地址做出决定**，而从不依据请求头。客户端无法通过 `X-Forwarded-For` 声称自己是 `127.0.0.1`，而从未经过该边界的请求会被算作远程。这是一项部署控制，而不是身份验证。

见[远程访问与 HTTPS](administration/remote-access.md)和[配置参考](environment/server.md#remote-access)。

## 日志中可能包含什么

服务器日志会进入 **日志与调试**、缺陷报告和备份，所以凭据在写入日志行时就会被清除（`internal/pkg/logger/redact.go`）：键名表示机密的属性（`authorization`、`token`、`api_key`、`password`、`pin`、`secret`……）会被整体替换，任何带有 bearer 令牌、JWT 或 `secret=value` 对的字符串都会被清理。日志路由在输出旧的日志行时也应用同样的规则。对于代码：永远不要用表示机密的键名记录非机密的值（它会被掩盖），并且在记录任何新类型的凭据之前，先把它加入那份列表。

## 报告漏洞

请私下报告，永远不要在公开的 issue 中报告：`security@eveyproject.org`，即仓库[安全策略](../SECURITY.md)中的地址。插件认证、权限和沙箱逃逸与其他问题一样都在其范围之内。不要在不属于你的 Evey 实例上进行测试。
