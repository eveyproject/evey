---
source_sha: 641ed9ad2a04551e1c5e1db07a26cb739f2708ce4ec146e2a5590265cd4216ca
---

# 扩展系统

Evey 的扩展为服务器补上元数据来源、媒体处理、Webhook、通知、前端模块和 CSS 主题。
每个扩展都作为一个**独立的子进程**运行，通过 Unix 域套接字上的 gRPC 与宿主通信；
扩展自带的界面则渲染在一个**沙箱化的 iframe** 里，经由 `postMessage` 桥与宿主对话。

本页只是一张地图，细节在下面三份专门的文档里：

| 文档 | 内容 |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | 写出你的第一个扩展：前置条件、脚手架、安装与迭代的循环、签名、提交到目录、排错。**请从这里开始。** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | 契约本身：清单的结构、后端 IPC 与数据存储、`evey.*` 前端桥、插槽列表，以及每一项能力（`metadata.provider`、`media.read/write`、`notification.sender`、`webhook.receiver`、`public`、`prefs`）。 |
| **[plugins/security-model.md](plugins/security-model.md)** | 威胁模型与各层防御：UID 隔离、内核与 LD_PRELOAD 两层 seccomp 拒绝清单、沙箱模式（`auto` 会依次尝试 landlock → bubblewrap → basic；`strict` 会在 bubblewrap 之上再加载 LD_PRELOAD 库）、用户态资源监视器、静态分析、CSS 净化器、签名与版本管理、插槽白名单，以及已知的局限。 |

扩展令牌背后的授权模型（默认拒绝、硬锁定的作用域、令牌绝不进入 iframe 的桥接方式）见
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md)；
资源上限见 [adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md)。

## 架构

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

扩展可以用**任何 `entrypoint:` 启动得起来的语言**编写，运行器并不加以限制。
Python、Node.js、PHP、Ruby、Dart、Java 和 Swift 都随附预先生成的 gRPC SDK 桩代码
（`sdk/<lang>/`，安装时自动拷贝）；Go 没有捆绑桩代码，请用 `protoc-gen-go` 和
`protoc-gen-go-grpc` 自行生成；没有捆绑桩代码的解释型语言（Bash、Lua、Perl 等）
直接调用解释器即可，而编译好的二进制（Go、Rust 等）则需要 `native-binary` 权限。
各语言的具体配置见 [plugins/dev-setup.md](plugins/dev-setup.md#per-language)。

三大支柱：

- **能力与作用域** - 扩展在自己的清单里声明需要哪些能力，管理员在安装时授予。
  宿主在每一次调用上按默认拒绝的原则执行检查。完整目录见
  [plugins/api-reference.md](plugins/api-reference.md#capabilities)。
- **沙箱** - 后端进程在沙箱中运行（文件系统、系统调用、网络三重约束），
  并处在用户态资源监视器之下；前端代码则跑在一个被锁死的 iframe 里。
  详见 [plugins/security-model.md](plugins/security-model.md)。
- **信任** - 扩展的 SHA256 会被固定下来，还可以用 Cosign 签名；服务器可以设成必须验签。
  见 [plugins/security-model.md](plugins/security-model.md#update-workflow)。

## 启用扩展子系统

扩展默认开启（`EVEY_PLUGINS_ENABLED=true`）。沙箱模式由 `EVEY_PLUGINS_SANDBOX_MODE` 决定
（默认 `auto`，在官方镜像上会落到 Landlock）。资源上限（`EVEY_PLUGINS_MEMORY_MB`、
`_PIDS_MAX`、`_DISK_MAX_MB`、`_CPU_NICE`）可由管理员在**管理 → 扩展**中修改。
参见 [configuration.md](configuration.md) 和 [plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox)。
