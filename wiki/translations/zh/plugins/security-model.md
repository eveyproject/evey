---
source_sha: 80123a8d7d69b50b13baa0a7c638767ebf54582aa06f7b85e176a6fee202d4ca
---
# 扩展安全模型：作者可以声明什么

> **你要找的是哪一页？** 这一页面向扩展作者：权限目录、外部请求必须满足的规则、你可以挂载的
> 插槽，以及更新时会发生什么。如果你运营服务器，想知道扩展是如何被约束的、沙箱还有哪些拦不住的
> 地方，请改读[扩展沙箱与威胁模型](../SECURITY.md)。

Evey 扩展是第三方代码，以独立的操作系统进程与服务器并行运行。本文说明恶意扩展可能尝试做什么、Evey 如何防御每一种攻击途径，以及当前实现的真实局限。

## 威胁模型：清单被赋予了哪些信任

| 威胁 | 说明 |
|---|---|
| 服务器 RCE | 扩展逃出沙箱，以服务器进程的身份执行任意命令 |
| 密钥外泄 | 扩展从文件系统读取 JWT 密钥、数据库凭据或用户令牌 |
| 横向越权 | 扩展通过 IPC 冒充另一个扩展，调用自己无权调用的 RPC |
| 网络外泄 | 扩展建立出站连接，把窃取的数据发送到攻击者控制的主机 |
| DoS / 资源滥用 | 扩展耗尽 CPU、内存、PID 或磁盘（fork 炸弹、挖矿、写满磁盘） |
| 前端钓鱼 | 扩展在用户浏览器里注入全屏遮罩来窃取凭据 |
| 供应链 | 作者提供经过混淆或编译的代码，静态分析无法检查 |

## 防御层

### 1. 清单校验（安装时）

每个 `plugin.yaml` 都在解压前校验。出现以下情况会拒绝安装：

- 缺少 `kind`，或者它不是 `plugin`/`theme`
- `kind: plugin` 缺少 `language` 或 `entrypoint`
- `kind: theme` 声明了 `language`、`entrypoint` 或 `permissions`
- `frontend[].slot` 不在服务器端的插槽白名单中（见[插槽白名单](#slot-whitelist)）
- `external_fetch` 条目违反主机白名单规则（见[外部请求规则](#external-fetch-rules)）
- 声明了未知的能力或权限

权威来源：`internal/domain/plugin/model.go`、`internal/adapters/driven/pluginrunner/capabilities.go`。

### 2. 安全解压 ZIP

在向磁盘写入任何文件之前：

- 条目数上限：最多 5,000 个文件
- 单文件大小上限：100 MB
- 解压后总大小上限：500 MB
- 压缩比上限：100:1（检测 zip 炸弹）
- 路径穿越检查：解析后路径中含 `..` 的条目会被拒绝
- 符号链接：一律拒绝（检查 `ExternalAttrs` 中的 Unix 类型位）
- setuid/setgid/sticky 位：从所有解压出的文件上清除

来源：`internal/adapters/driven/pluginrunner/zip.go`。

### 3. 静态分析器（安装时）

在扩展被接纳进数据库之前，ZIP 中的每个文本文件都会经过一个基于正则的多语言分析器扫描。预先的规范化处理会化解常见的混淆手法（别名解析、字符串拼接折叠、f-string 展开）。

| 类别 | 严重性 | 示例 |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`、`subprocess.run`、`child_process.exec`、PHP `shell_exec`/`proc_open`/`preg_replace /e`、Ruby 反引号/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING/CRITICAL | `requests.get`、`fetch(`、`net.Dial`、反弹/绑定 shell 模式 |
| `filesystem` | WARNING/CRITICAL | `/etc/passwd`、`.ssh/`、`rm -rf`、符号链接操作、`/proc/<pid>/environ` |
| `obfuscation` | CRITICAL | `eval(`、`base64.b64decode`、十六进制序列、`String.fromCharCode` 链、PHP `chr()` 链、`str_rot13`、`assert(base64_decode(...))` |
| `privilege` | CRITICAL | `setuid`、`chmod 4777`、`sudo`、`os.kill(1, ...)` |
| `cryptomining` | CRITICAL | `stratum+tcp`、`xmrig`、`cryptonight` |
| `runtime_install` | CRITICAL | `pip install`、`npm install`、`curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`、`importlib.import_module`、`getattr(__builtins__, ...)` |
| `sandbox_escape` | CRITICAL | `vm.runInNewContext`、`process.binding`、`new Function(`、Python `__subclasses__()`、`__mro__` |
| `deserialization` | CRITICAL/WARNING | `pickle.loads`、`marshal.loads`、`__reduce__`；WARNING：不带 Loader 的 `yaml.load()` |
| `native_code` | CRITICAL/WARNING | `ctypes.CDLL`、`ffi.Library`；WARNING：Go `unsafe.Pointer` |
| `compiled_binary` | WARNING → BLOCK | `.so`、`.dll`、`.exe`、`.wasm`、`.jar` - 扫描器无法看透 |
| `embedded_archive` | WARNING → BLOCK | `.zip`、`.tar.gz` - 可能含有未经分析的代码 |
| `mime_mismatch` | WARNING（按严重处理） | 扩展名声称是图片/字体，内容却是可执行文件 |
| `suspicious_url` | WARNING | URL 字面量中的非 HTTP 协议（`ftp://`、`wss://`、`ssh://`） |
| `unvendored_deps` | WARNING | `requirements.txt`/`package.json` 声明了依赖，却没有随附的模块 |

CRITICAL 问题或 `mime_mismatch` 会设置 `Safe = false` 并阻止安装。

`compiled_binary` 和 `embedded_archive` 会阻止安装，除非清单声明了 `native-binary` 权限。管理员在接受之前的预览步骤中会看到警告和该权限。

来源：`internal/adapters/driven/pluginrunner/analyzer.go`、`analyzer_normalize.go`。

#### 尽力而为，并非强制边界

分析器基于正则和模式。它能化解已知类别的混淆，但**无法保证识破每一种刻意的绕过**。按类别的覆盖示例：

| 类别 | 可检测 | 说明 |
|---|---|---|
| 由字符串拼接得到的 PHP 可调用变量 | `$f = "sys" . "tem"; $f($cmd)`（单行）以及 `$x="sys"; $y="tem"; $f=$x.$y; $f($cmd)`（多步，求解到不动点） | 最多迭代 5 次到不动点，`$a=$b; $b=$a` 这样的循环会干净地终止 |
| 按名称查找 Python 内建函数 | `getattr(__builtins__, "exec")` 和 `__builtins__.__dict__["exec"]`（字面量或经解析的变量） | 当字面量危险时，字典方括号索引会触发 Critical |
| JS `Function.constructor` 逃逸 | `(fn).constructor("...")()` 立即调用，以及 `const F = (fn).constructor; F("...")`（通过绑定重写跟踪保存下来的引用） | 以变量为参数的调用 `F(code)` 会被重写为 `.constructor(code)`，并由变量参数模式匹配 |
| 多步碎片化拼接 | `$a="s"; $b="y"; $c="s"; $d="t"; $e="e"; $f="m"; $g=$a.$b.$c.$d.$e.$f; $g(...)` | 无害的碎片与"危险"值分开跟踪；危险检查只对最终拼接出的字符串触发 |

每一类绕过手法在 `tests/fixtures/plugins/malicious/` 中都有一个永久回归测试。新增一类需要：(1) 提交 fixture，(2) 扩展分析器规则或规范化处理，(3) 检测生效后，`installer_security_test.go` 把 `workerANeeded:true → false`。

**不在范围内（由沙箱强制）：**AST 层面的动态变换 - `strrev("metsys")`、`base64.b64decode(...)`、`eval(open("/encoded").read())`、运行时 regex.replace。正则分析器无法解析这些；运行时沙箱（Landlock 白名单或 bwrap 挂载白名单，加上内核 seccomp 黑名单）+ 用户态资源监控 + IPC 对端凭据，共同保证扩展无法逃逸、无法外泄 `/data`、也无法超出所声明的权限。

**已知误报：**分析器也会匹配源码注释和文档字符串里的内容。注释里的一行 `# pip install foo` 就会触发 `runtime_install` 规则。可以接受：让运维者在发布前删掉这类注释，比在正则分析器里按语言剥离注释要便宜；bwrap 挂载白名单 + 用户态资源监控使运行时的拦截与之无关。

#### 运行时的实证

挂载白名单已在一个运行中的栈上，通过一个金丝雀扩展（`tests/fixtures/plugins/canary-sandbox-runtime/`）做了端到端验证，它在启动时探测自己在沙箱里看到的情况：

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```

`/data` 之所以出现，不是因为挂载了主机的 `/data`（它从不挂载），而是因为 bubblewrap 为扩展自己的读写绑定目标（`/data/plugins/<id>`）合成了空的父目录；因此它只包含那一个子目录。看不到任何 `jwt.key`、`backups` 或 `artworks`。`/proc/1/environ` 确实可读，但命名空间里的 PID 1 就是扩展自己（`--unshare-pid`），所以读取自己的环境变量不算泄露。

每次修改 `runner_bwrap.go` 后，请重新运行金丝雀，确认挂载布局依然能拦住。

**强制边界是运行时沙箱（Landlock 或 bubblewrap）+ 内核 seccomp + 用户态资源监控 + IPC 对端凭据，而不是分析器。**一个把 CRITICAL 模式偷偷带过分析器的扩展，依然无法读取 `/data`（Landlock 拒绝 / bwrap 从不挂载它），无法发起被拒绝的系统调用，无法突破 PID 上限搞 fork 炸弹，也无法调用它无权调用的 gRPC 方法。分析器是纵深防御中的一层，用于在预览时暴露明显的恶意代码、缩小运行时攻击面 - 它不是边界。

### 4. Cosign 签名验证

可通过 `evey.yaml` 的 `cosign_policy` 键配置：

| 策略 | 行为 |
|---|---|
| `off` | 不检查签名；`Plugin.Signature` 为 nil |
| `warn`（默认） | 验证失败会被记录并展示给管理员，但不阻止安装；未配置公钥时验证什么也不做 |
| `enforce` | 验证失败会以错误阻止安装 |

来源：`internal/adapters/driven/pluginrunner/cosign.go`。

### 5. CSS 清理

安装时作用于 `theme.css`。完整规则见 [`docs/themes/security-model.md`](../themes/security-model.md)。清理会删除或改写危险的写法，而不是拒绝整个文件（警告会被记录）。

### 6. 主题 JS 扫描器

安装时作用于 `theme.js`。任何匹配的模式都会导致安装被拒。被阻止的模式列表见 [`docs/themes/security-model.md`](../themes/security-model.md)。

### 7. 运行时沙箱（Landlock / bubblewrap + seccomp + 用户态资源监控）

> **默认是 `landlock`**（在官方 Docker 镜像上，`sandbox_mode=auto` 会解析为它）：Landlock
> LSM 文件系统白名单 + `-restrict-net`（任何扩展都没有直接的 IP 出站）+ 内核与 `LD_PRELOAD`
> 的 seccomp 黑名单 + `-deny-inet`，全部运行在 Docker 默认的 seccomp+AppArmor 之下，主机无需
> 任何改动。总览见 [`docs/plugins.md`](../plugins.md)，按模式的完整对照表见
> [`docs/SECURITY.md`](../SECURITY.md#sandbox-modes)。

下面的约束层是否生效取决于主机能力。启动器（及其安装的内核 seccomp）只在 **`landlock`** 和
**`basic`** 模式下使用：`applySandbox` 会把 `bubblewrap`/`strict` 引到一条独立的 bwrap 代码
路径，那里从不调用启动器，所以这两种模式依赖的是 bwrap 自己的命名空间隔离（`strict` 还会加上
LD_PRELOAD 垫片）。

- **UID 隔离**（`EVEY_PLUGINS_UID_SEPARATION`，默认 `auto`）- 启用时，每个扩展通过**启动器**上
  的 `cap_setuid` 切换到 uid 1001（`evey-plugin`），与服务器的 uid 1000 不同（服务器本身保持
  无特权）。`auto` 只在 `basic` 模式下（没有 Landlock/bwrap）启用它，因为那里 DAC 是保护密钥的
  唯一一道墙；`on` 在所有地方强制启用；`off` 则关闭。启用后，即使没有 Landlock，`jwt.key`
  （`0600`）和 `/data/{keys,logs,backups}`（仅属主可读）对 uid 1001 的扩展来说在 DAC 层面也
  不可读。
- **内核 seccomp 黑名单（仅 `landlock`/`basic`）**- 由 `evey-plugin-launcher` 按构建架构在
  `execve` 之前安装，因此**对静态/编译型的扩展二进制同样有效**（它们从不加载
  `libevey-sandbox.so`）。拦截 `ptrace`、`mount`、`unshare`、`setns`、`bpf`、`kexec`、加载
  模块、setuid 系列、`capset`、keyring、向 PID 1 发信号等。它会通过 `execve` 继承给扩展及其
  子进程。**在 `bubblewrap`/`strict` 中启动器的这个过滤器不生效** - `strict` 只额外加上
  `LD_PRELOAD` 的 `libevey-sandbox.so` 垫片（同样的黑名单，另加 `execve`/`execveat`，使没有
  `subprocess` 的解释型扩展无法创建子进程），它只对动态扩展有效，覆盖不到静态二进制。
- **`-deny-inet`** - 对**任何**扩展，无论是否有网络权限，启动器都会在内核层面让
  `socket(AF_INET/AF_INET6)` 返回 EPERM：没有 IP 出站，即使在 Landlock ABI <4 上也成立。
  `AF_UNIX` 仍然允许，用于 HostService 的 gRPC 套接字。
- **`-deny-fork`** - 对每个**没有** `subprocess` 权限的扩展，创建进程会在内核层面返回 EPERM
  （`fork`、`vfork`、不带 `CLONE_THREAD` 的 `clone`）；创建线程不受影响，所以 node、Python 和
  Go 的运行时都不会出问题。`clone3` 返回 `ENOSYS` 而非 `EPERM`，让 glibc 退回到可检查的
  `clone(2)`，而不是直接创建线程失败。
- **服务器 `PR_SET_DUMPABLE=0`** - 服务器在启动时把自己标记为不可转储，使同 uid 的扩展无法读取
  `/proc/<server>/{mem,environ,maps}`（内存中的 JWT 密钥、`DB_PASSWORD`、`MEILI_MASTER_KEY`）。
- **用户态资源监控** - 见下文；不使用 cgroups。

**文件系统/网络后端**额外提供什么，取决于模式：

| 模式 | 文件系统约束 | 网络 | 何时选用 |
|---|---|---|---|
| `landlock` | Landlock LSM 白名单（系统目录只读；扩展目录 + 套接字目录可写；`/data` 及同级目录被拒绝） | `-restrict-net`（切断所有 IP 出站，不按端口/主机过滤）+ `-deny-inet` | `sandbox_mode=landlock`（不可用时封闭失败）；在有 Landlock 的内核上 `auto` 选用 - 官方镜像的常规情况 |
| `bubblewrap` | 在隔离的命名空间中使用挂载白名单（见下文） | 始终 `--unshare-net`（每个扩展，与 `network` 权限无关） | `sandbox_mode=bubblewrap`；没有 Landlock 但 bwrap 探测通过时由 `auto` 选用（裸机 / 特权） |
| `strict` | 同 `bubblewrap` + LD_PRELOAD seccomp | 同 `bubblewrap` | `sandbox_mode=strict` |
| `basic` | 无（仅 DAC；`auto` 的最后退路） | 仅 `-deny-inet` | `sandbox_mode=basic`；Landlock 和 bubblewrap 都不可用时由 `auto` 选用 |

`auto` 在第一个扩展启动时按 **landlock → bubblewrap → basic** 的顺序解析一次，并记录所选模式。
`basic` **并不是**"没有隔离"：它仍然施加 UID 隔离、内核 + LD_PRELOAD seccomp、`-deny-inet`、
尽力而为的 Landlock 以及资源监控 - 它只是在内核没有 Landlock 时失去了*按路径的文件系统约束*
（这时扩展可以读取所有人可读的非敏感文件，`/proc` 中的进程*列表*可见；
`/proc/<server>/{mem,environ}` 仍受保护）。

当 `sandbox_mode` 为 `bubblewrap` 或 `strict`（在 `evey.yaml` 中主动选择）时，每个扩展进程都运行在一个 bubblewrap 命名空间内，具备：

**命名空间隔离（5 个独立）：**
- `--unshare-user` - 专用的用户命名空间，UID 映射
- `--unshare-pid` - 隔离的 PID 命名空间
- `--unshare-uts` - 隔离的主机名
- `--unshare-ipc` - 隔离的 IPC
- `--unshare-cgroup` - 隔离的 cgroup 命名空间
- `--unshare-net` - 网络命名空间**始终**隔离，对每个扩展都一样，与 `network` 权限无关；直接出站需经由宿主的 `Fetch` RPC 代理

**挂载白名单：**

沙箱只以只读方式绑定挂载运行所需系统路径的**精选白名单**（解释器、共享库、SSL、DNS），绝不暴露数据路径。**不会**整体绑定 `/`：只挂载下列目录，所以在命名空间里 `/data`、`/var`、`/home`、`/root` 以及相邻扩展的目录根本不存在。

| 路径 | 访问 | 原因 |
|---|---|---|
| `/usr`、`/bin`、`/sbin` | 只读 | 解释器和系统二进制 |
| `/lib`、`/lib64` | 只读（`--ro-bind-try`） | 共享库（按发行版跳过不存在的路径） |
| `/etc/ssl`、`/etc/ca-certificates`、`/etc/resolv.conf`、`/etc/nsswitch.conf`、`/etc/{alpine,os}-release` | 只读（`--ro-bind-try`） | TLS 信任库、DNS 解析、运行时检测。**不是**整个 `/etc`：`/etc/passwd`、`/etc/shadow`、`/etc/ssh` 从不挂载。 |
| `<plugin_dir>` | 读写 | 扩展自己的安装目录 |
| `<plugin_sock_dir>` | 读写 | 每个扩展的 IPC 套接字目录（在 `/tmp` 的 tmpfs 之后重新绑定） |
| `/proc` | proc 挂载 | 运行时需要 |
| `/dev` | 合成（`--dev`） | 最小设备集（null、zero、full、random、urandom、tty、pts）- 没有物理设备 |
| `/tmp` | tmpfs | 临时工作区；看不到主机的 `/tmp` |

**不挂载**的路径：`/data`、`/var`、`/home`、`/root`、`/run`、`/etc/passwd`、`/etc/shadow`、`/etc/ssh`、其他扩展的目录。

来源：`internal/adapters/driven/pluginrunner/runner_bwrap.go` 中的 `buildBwrapCmd`。

**用户态资源监控**（每个扩展）：

资源限制由一个用户态清理器执行（而不是 cgroups，它在无特权的 Docker 容器里不可用）。监控器约每 2 秒轮询一次 `/proc/<pid>/{statm,status,stat}`，一旦超出任何上限就杀掉扩展。启动时会设置 `nice`，保证扩展永远不会让服务器抢不到 CPU。

- 内存上限（RSS，默认 256 MiB）- 通过 `/proc/<pid>/statm`
- PID/线程数上限（默认 64）- 通过 `/proc/<pid>/status`
- CPU 优先级 - 通过 `nice`（默认 10）
- 磁盘上限（安装目录，默认 100 MiB）- 约每 30 秒递归执行一次 `filepath.Walk`

放弃 cgroups 的理由见 [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md)。

注意：下面 bwrap 命名空间列表里的 `--unshare-cgroup` 会创建一个隔离的 cgroup _命名空间_（扩展看到的是一个全新的 cgroup 层级），但**不会**施加资源限制；那些限制来自上面的用户态监控。

来源：`internal/adapters/driven/pluginrunner/runner_landlock.go`、`runner_bwrap.go`、`runner_resmon.go`；`cmd/evey-plugin-launcher/{main.go,seccomp_linux.go,landlock_linux.go,droppriv_linux.go}`；`cmd/evey/hardening_linux.go`；`sandbox/sandbox.c`。

### 8.（已废弃）DNS 白名单

本文早期版本描述的 V1 版 `/etc/hosts` DNS 白名单已被**移除**。`runner_dns.go` 已不存在；
`buildBwrapCmd` 不再绑定挂载 `/etc/hosts`（见 `internal/adapters/driven/pluginrunner/runner_bwrap.go`
中的注释："no /etc/hosts whitelist is bind-mounted here anymore"）。取而代之的是一个更严格的
机制：每个扩展的直接 IP 出站都被完全切断（`landlock`/`basic` 中的 `-restrict-net` +
`-deny-inet`，`bubblewrap`/`strict` 中的 `--unshare-net`），有网络权限的扩展只能通过宿主的
`Fetch` 代理访问网络，由它在宿主侧执行 `external_fetch` 白名单和防 SSRF 的连接器（见
`docs/SECURITY.md` 中的[扩展网络出站](../SECURITY.md#plugin-network-egress-brokered-since-2026-07-11)）。

### 9. IPC 对端凭据认证

宿主为每个扩展在 `/tmp/evey/plugins/<plugin_id>/host.sock` 暴露一个 Unix 套接字。每个传入的 gRPC 调用都通过 `getsockopt(SO_PEERCRED)` 读取对端 PID 来认证 - 这个值由内核提供，在用户态无法伪造。该 PID 会与 `cmd.Start` 之后立即登记的值进行比较。

效果：
- 扩展无法冒充另一个扩展：`x-plugin-id` 请求头会被忽略；身份来自监听套接字。
- 通过 fork/exec 创建子进程的扩展，无法借子进程转发对宿主的调用（PID 不同 → `Unauthenticated`）。

来源：`internal/adapters/driving/pluginhost/`。

### 10. 前端隔离

扩展前端模块运行在一个 `<iframe>` 中，具备：
- `sandbox="allow-scripts"` - 无同源访问、不能提交表单、不能弹窗
- `srcdoc` 内的 CSP `<meta>` 标签：`default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src <external_fetch_hosts>; img-src data: blob:; font-src data:`
- 在宿主侧强制校验 postMessage 的来源
- 桥接的速率限制：`toast` 每秒最多 3 条、200 个字符、不含 HTML
- `navigate` 路径白名单：拒绝 `/admin/*`、`/auth/*`、`/setup/*`、`/profile/security/*`
- `resize` 限制在 50-800 px
- 插槽白名单在服务器端（安装）和客户端（渲染）双重执行

## 权限目录

| 权限 | 授予什么 |
|---|---|
| `network` | 扩展可以建立出站网络连接。**必须**与非空的 `external_fetch` 列表一起使用 - 缺少时安装会被拒绝。 |
| `subprocess` | 扩展可以创建子进程。没有它时两层约束生效：LD_PRELOAD seccomp 垫片为解释型扩展拦截 `execve`/`execveat`，启动器的 `-deny-fork` 则在内核中阻止创建进程本身（`fork`/`vfork`/不带 `CLONE_THREAD` 的 `clone`）。线程在任何情况下都能正常工作。（启动器的内核 seccomp 不拒绝 `execve` - 它得执行扩展 - 但任何子进程都会继承内核过滤器 + Landlock 域。） |
| `native-binary` | 扩展可以附带 `.so`/`.dll`/`.exe`/`.wasm`/`.jar` 或内嵌压缩包。管理员会在安装预览中看到这些不透明代码并必须明确接受。 |
| `filesystem` | 声明打算访问扩展自身目录以外的文件系统。在审计中执行；尚不是沙箱的硬性规则（见已知局限）。 |
| `media.read` | 扩展可以调用 `MediaRead` RPC：读取媒体文件元数据和流访问。 |
| `media.write` | 扩展可以调用 `MediaWrite` RPC：创建/更新媒体记录。 |
| `api.library` | 扩展可以调用媒体库管理 RPC：列出/搜索媒体库、触发扫描。 |
| `api.user` | 扩展可以调用用户范围的 RPC（读取自己的个人资料、偏好）。 |
| `api.metadata` | 扩展可以调用元数据 RPC：搜索并写入增强数据。 |
| `notification.sender` | 扩展可以发布通知事件（网页推送、邮件、webhook）。 |
| `webhook.receiver` | 扩展可以注册为 webhook 目标（接收外部服务的 HTTP POST）。 |
| `hardware.gpu` | **风险最高的权限 - 仅限管理员，第一方级别。**授予主机的 DRM 渲染节点（`-render-dev`，真实的 `renderD1xx`，绝不是 card 节点，绝不是 `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`）：这意味着一个内核驱动的攻击面，以及一个与转码器**共享**的 GPU 内存边界（在 AMD/Apple/Qualcomm GPU 上可能发生跨进程内存泄露；GPU 卡死会拖停转码）。没有这个权限时，GPU 设备节点被完全排除在 Landlock 设备白名单之外（无论 DAC 如何都拒绝）。授予后，启动器会为那一个文件添加一条显式的 `IOCTL_DEV` Landlock 规则（ABI v5+）；在较旧的 Landlock ABI 上，ioctl 根本不受 Landlock 管辖，所以仅凭读写权限就已经能向 GPU 提交命令。安装器会在预览时给出醒目的 CRITICAL 警告；只应授予可信的第一方扩展。 |

## 外部请求规则

在清单的 `external_fetch: [...]` 中声明，并在安装时由 `validateExternalFetch`
（`internal/adapters/driven/pluginrunner/manifest.go`）检查。全有或全无：只要有一个条目无效，
整个安装就会被拒绝。

每个条目都是一个**纯主机名**。协议前缀可以容忍但会被丢弃 - 只保留主机名。

| 条目 | 是否有效 | 原因 |
|---|---|---|
| `api.tmdb.org` | 是 | 精确的主机名，标准写法 |
| `https://api.tmdb.org` | 是 | 协议被解析后丢弃，保留主机名 |
| `*.tmdb.org` | 否 | 拒绝通配符 - 请逐一列出需要的子域名 |
| `*` | 否 | 同一规则 |
| `192.168.1.1`、`::1` | 否 | 拒绝 IP 字面量，两种地址族都一样 |

运行时，宿主代理还会加上自己的限制，清单无法放宽：只允许 `http` 和 `https`，只允许 `GET` 和
`HEAD`，目标主机与你的列表做不区分大小写的匹配，DNS 解析后拒绝私有或保留地址 - 重定向会按两条
规则重新检查。

> 代理的匹配器也认得 `*.example.com` 这种形式，但安装器会拒绝这样的条目，所以它根本到不了代理
> 那里。请写明确的主机名。

## 插槽白名单

扩展只能在明确允许的插槽中注入界面。关键路径被刻意排除在外。

| 插槽 | 需要登录 | 说明 |
|---|---|---|
| `home.section` | 是 | 主页内容区 |
| `home.hero` | 是 | 主横幅区域 |
| `library.sidebar` | 是 | 媒体库视图侧边栏 |
| `media.detail.sidebar` | 是 | 媒体详情侧边栏 |
| `player.overlay` | 是 | 播放器覆盖层界面 |
| `settings.user.appearance` | 是 | 用户设置中的外观区块 |
| `sidebar.bottom` | 是 | 导航栏底部区域 |
| `admin.nav` | 是 | 管理面板中的一个菜单项，打开扩展自己的隔离页面 - 不会注入现有管理页面 |

**永不允许：**`admin.*`（`admin.nav` 除外）、`login.*`、`setup.*`、`profile.security.*` - 这些路径对认证和权限至关重要，允许扩展在那里注入就等于打开了提权或钓鱼的通道。

`admin.nav` 的内容和其他插槽一样被隔离：它在带 CSP `default-src 'none'` 的 `<iframe sandbox="allow-scripts">`（没有 `allow-same-origin`）中渲染，因此它只是显示在管理区的第三方内容，而不是授予扩展的管理能力。

来源：`internal/domain/plugin/slots.go`。

## 更新流程

1. 管理员上传新的 ZIP → `PreviewPluginUpdate` 运行静态分析器 + cosign 检查，并返回相对于已安装版本、权限/插槽/`external_fetch` 的变更差异。
2. 如果差异非空（新权限、新插槽、外部主机变化），在应用更新之前管理员必须明确确认（`requires_ack = true`）。
3. 预览时记录源文件哈希，应用时再次校验（防 TOCTOU：被分析的 ZIP 就是被安装的 ZIP）。
4. 如果 cosign 策略是 `enforce`，即使预览通过，应用阶段签名不匹配也会阻止更新。

## 权限模型的已知局限

| 局限 | 影响 | 目前的缓解措施 |
|---|---|---|
| 在既没有 Landlock、也没有可用 bubblewrap 的主机上，`auto` 可能解析为 `basic` | 没有**按路径**的文件系统约束：扩展可以读取所有人可读的非敏感文件，`/proc` 中的进程列表可见 | 服务器密钥仍受保护（UID 隔离 + `PR_SET_DUMPABLE=0`）；内核 seccomp（包括静态二进制）+ `-deny-inet` 依然有效；解析出的模式会在第一个扩展启动时记录下来 |
| 所有扩展共用 UID 1001 | 恶意扩展可以向同伴发信号（向 PID 1 / 服务器发信号被 seccomp 拦截） | `PR_SET_DUMPABLE=0` 阻止跨进程读取 `/proc/<pid>/{environ,mem,maps,fd}`；Landlock / bubblewrap 把各扩展的目录彼此完全隔离 |
| `Fetch` 代理控制出站调用*去哪里*，但管不了*带出去什么* | 一旦某主机进入 `external_fetch` 白名单，代理就无法检查载荷内容，因此向白名单主机外泄数据仍有可能 | 管理员应只把可托付扩展数据的主机加入白名单；代理仍会拦截非白名单目标和私有/保留 IP |
| LD_PRELOAD seccomp 垫片只约束动态（解释型）扩展 | 在 `basic` 模式下且没有启动器时，静态链接的扩展二进制会完全绕过垫片的黑名单 | 官方镜像始终附带启动器（在 `landlock`/`basic` 中由内核 seccomp 覆盖静态二进制）；没有启动器二进制的开发镜像不应运行不受信任的扩展 |
| 正则分析器可被高级混淆绕过 | 有决心的攻击者可以隐藏模式 | 管理员预览 + 权限审查；强制边界是沙箱 + 资源监控 + IPC 对端凭据，而不是分析器 |

## V2 将增加什么

- 容器级的 nftables 出站防火墙，作为 `Fetch` 代理之后的一道独立防线（属于运维加固，并非扩展沙箱的缺口 - 每个扩展的直接 IP 出站都已被完全切断）
- 一个出站代理，实现完整的请求级控制
- seccomp 白名单模式（严格沙箱：只允许最小的系统调用集合）
- 通过逐请求的检查，支持 `external_fetch` 中的通配符条目
