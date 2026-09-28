---
source_sha: 8da868d1b798afe178e9ef7c621a9a0513b023c77f535745e4e5d2c44b5dae50
---
# 扩展开发环境准备

面向刚克隆了 Evey、想要编写、测试并发布扩展或主题的人的完整流程。选择与你扩展所用语言相匹配的运行时；其余部分都是通用的。

## 1. 前置条件

Evey 运行宿主进程（Go），并以 ZIP 的形式分发扩展。扩展需要什么运行时，取决于你的代码调用了什么。

### 始终需要

| 工具 | 用途 | 安装 |
|---|---|---|
| Go ≥ 1.25 | 构建 Evey 宿主 | `https://go.dev/dl/` |
| Docker + Docker Compose | 默认开发栈（Postgres、Meilisearch、服务器、网页） | https://docs.docker.com/engine/install/ |
| `bubblewrap`（`bwrap`） | 扩展进程的沙箱 - `bubblewrap` 和 `strict` 模式必需 | `apt install bubblewrap`（Debian/Ubuntu）、`dnf install bubblewrap`（Fedora）、`pacman -S bubblewrap`（Arch）。macOS：请在虚拟机里装 Linux，bwrap 只支持 Linux。 |

`basic` 沙箱模式不需要 bwrap 也能运行，但除了用完即弃的本地实验之外绝不应该使用 - 能换就尽快换掉。

### 按语言

只准备你的扩展真正用到的；不需要全部都装。

| 扩展语言 | 工具 | 说明 |
|---|---|---|
| Python 3 | `python3`、`pip` | 用 `pip install --user` 保持宿主干净。SDK 桩代码在 `sdk/python/`。 |
| Node.js | Node 20+，`npm` 或 `pnpm` | SDK 在运行时通过 `@grpc/proto-loader` 加载 `plugin_api.proto`。 |
| Go | Go ≥ 1.21 | 不附带桩代码 - 用 `protoc-gen-go` + `protoc-gen-go-grpc` 生成；编译好的 Go 二进制还需要 `native-binary` 权限。 |
| Dart / Java / PHP / Ruby / Swift | 各语言自己的工具链 | 桩代码已预先生成在 `sdk/<lang>/`。 |
| 纯 CSS 主题 | 无需额外工具 | 主题就是一个包含 `kind: theme` + 一个 `theme.css` 的 ZIP。 |
| 带 JS 的主题 | 无需额外工具（宿主会在一个带 `sandbox="allow-scripts"` 和严格 CSP 的隔离 `iframe` 中运行你的 JS） | 静态扫描器会拒绝任何对 `document`、`window`、`fetch`、`localStorage` 等的访问 - 见 `internal/pkg/themejs/scan.go`。 |

### 为其他语言生成桩代码

上面这些语言的桩代码都已预先生成，但扩展的 gRPC 契约只是一个 proto 文件 - 你可以面向
**任何 `protoc` 支持的语言**（Rust、C#、Kotlin、Elixir……）。唯一的权威来源是：

```
proto/plugin/v1/plugin_api.proto
```

用 `protoc`（或 `buf`）生成，把你所用语言的 gRPC 插件指向它：

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

内置语言的现成做法（每种语言需要哪个 gRPC 插件：`grpc_php_plugin`、
`protoc-gen-grpc-java`、`protoc-gen-grpc-swift`……）写在根目录 `Makefile` 的
`plugin-sdk` 目标里，以及各个 `sdk/<lang>/README.md` 中（例如 `sdk/php/README.md`）。
`make plugin-sdk` 会重新生成随附的桩代码。

请把生成的桩代码放进你的扩展 ZIP - 宿主只会为它自带的语言（`sdk/<lang>/`）自动复制桩代码。

### 可选但推荐

| 工具 | 用途 |
|---|---|
| `cosign`（Sigstore） | 为你的扩展 ZIP 签名。`EVEY_PLUGINS_COSIGN_POLICY` 默认是 `warn`（不是 `off`）- 在 `warn` 且未配置公钥时，验证什么也不做（只记一条警告，不阻止任何东西）；设置 `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` 才会真正验证签名，设为 `enforce` 则拒绝未签名的上传。https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | 在提交扩展之前发现明显的问题。 |
| `golangci-lint` | 如果你的扩展用 Go 编写，并希望与宿主保持同样的 lint 标准，则需要它。 |

## 2. 在本地启动 Evey

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

服务器健康后，打开 https://evey.test（由 Caddy 提供）。默认凭据写在 `.env.dev` 里 - 在对外开放实例之前请先修改。

如果 `make dev` 因 `bwrap` 失败：
- 确认在宿主机上 `which bwrap` 能返回一个路径
- 在 Docker 里，开发容器自带 bwrap 二进制；如果你改过 Compose 栈，请把它挂载进去，或者*只对开发容器*设置 `EVEY_PLUGINS_SANDBOX_MODE=basic`

## 3. 搭建你的第一个扩展

示例在 `examples/plugins/` 中：Node.js 和 Python 的起步模板（`nodejs-example`、`python-example`）、一个元数据来源（`metadata-example`）、一个 webhook 接收器（`webhook-receiver`）、一个主页小组件（`weather`）、扩展配置（`config-demo`）以及两个主题（`theme-dark-blue`、`open-workshop`）。挑一个最接近你需求的，复制到一个工作目录。关于数据持久化和权限，请看 **`examples/plugins/data-demo`** - 它使用每个扩展专属的数据存储，并申请 `public` / `prefs` 权限以及 `admin.nav` 插槽。

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

编辑 `plugin.yaml`：

- `name`：kebab-case，唯一
- `version`：semver - 每次发布都要递增
- `kind`：`plugin`（需要后端）或 `theme`（仅前端）
- `language`：`python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh`（清单里的标识符 - 不是解释器二进制的名字；`python3`/`node` 不是有效的 `language` 值，见下面的 `entrypoint`）。
- `entrypoint`：运行时执行的命令行
- `permissions`：声明你调用的每一个宿主 API。安装器会拒绝未声明的调用。完整列表见[权限目录](security-model.md#permissions-catalog)。
- `external_fetch`：当你需要访问第三方 API 时使用。条目是纯主机名（`api.example.com`）- 不需要也不使用协议前缀（`https://` 这样的前缀会被解析，只保留主机名），不允许通配符（`*.example.com` 会在安装时被拒绝：`external_fetch entry %q is a wildcard - use explicit hostnames`），不允许字面 IP 地址（任何 IP 都不行，不只是 RFC1918）。校验逻辑在 `internal/adapters/driven/pluginrunner/manifest.go`（`validateExternalFetch`）；测试在 `internal/adapters/driven/pluginrunner/manifest_test.go` - 运行 `go test ./internal/adapters/driven/pluginrunner/...` 检查你的值是否合规。

前端模块（`kind: plugin` 可选，`kind: theme` 禁止）：

```yaml
frontend:
  - slot: home.section
    src: dist/index.js
```

插槽必须存在于 `internal/domain/plugin/slots.go` 中（`home.section`、`home.hero`、`library.sidebar`、`media.detail.sidebar`、`player.overlay`、`sidebar.bottom`、`settings.user.appearance`，以及 `admin.nav` - 管理区唯一受控的插槽，一个打开你扩展自己的隔离页面的菜单项）。其他任何插槽（`admin.*` 注入、`login.*`、`setup.*`、`profile.security.*`）都会在安装时被拒绝。

**存储数据。**你的后端通过宿主 socket 获得一个私有的键值存储
（`SetData`/`GetData`/`DeleteData`/`ListDataKeys`）- 不需要权限，卸载时自动清除。如果要在
iframe 里保存每个用户的偏好，请申请 `prefs` 权限；如果要只读访问非敏感的媒体信息（标题/海报），
请申请 `public`。见 [API 参考](api-reference.md#permissions)。管理、认证、系统和通用写入范围
永远无法申请。

## 4. 安装循环

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

安装器按以下顺序执行 - 每一步都可能拒绝安装：

1. **清单校验** - 直接从 ZIP 中读取 `plugin.yaml`（在下面的 ZIP 层面检查之前）并按 schema 校验。插槽白名单。权限/能力白名单。`external_fetch` 解析器（拒绝通配符和任何字面 IP；`http://` 这样的协议前缀会被接受但忽略 - 只保留主机名）。声明的 `language` 运行时必须在镜像中可用。
2. **权限升级**（仅限更新）- 安装器把新清单与上一版本对比。任何新申请的权限/能力/插槽/`external_fetch` 主机都会以一个错误阻止安装，调用方必须明确覆盖（前端传 `accept_permission_changes: true`）。全新安装跳过这一步 - 没有可比较的对象。
3. **Cosign 签名** - 配置了签名验证器时进行验证；只有在 `EVEY_PLUGINS_COSIGN_POLICY=enforce` 时，验证失败才会阻止安装。见第 6 节。
4. **解压 ZIP** - 检查扩展名，总大小 ≤ 500 MB，单个条目 ≤ 100 MB，压缩比 ≤ 100:1，不允许符号链接，不允许路径穿越条目；文件先落到暂存目录（升级时，直到后续所有步骤通过才会动到正在运行的版本）。
5. **CSS 清理**（仅主题）- 去掉 `@import`，阻止非 `data:` 的 `url()`，阻止全屏遮罩，阻止针对 `[type=password]` 和 `.admin-*` 的选择器，把 `z-index` 限制在 ≤ 999。
6. **主题 JS 扫描器**（仅主题）- 拒绝任何对 `document.cookie`、`localStorage`、`fetch`、`eval`、`globalThis` 的引用，拒绝对宿主全局对象使用方括号访问属性，以及 `constructor.constructor` 等。主题只能使用 `evey.theme.*` API。
7. **静态分析器** - 扫描每个文本文件，查找 shell 执行、反弹 shell、eval、先 base64 解码再执行、混淆模式、运行时安装命令、提权。严重问题 = 拒绝安装。警告类问题（反序列化、混淆、native_code）除非声明了相应权限，否则会升级为拒绝。
8. **持久化** - 把暂存目录换到正式位置，在 Postgres 中写入记录，并按需启动运行时。

任何一步失败，管理界面都会显示确切原因。服务器日志（`docker compose logs evey`）里有完整的调用栈。

## 5. 迭代

开发时热重载你的扩展：

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

当你递增 `version` 时，安装器会把这次上传当作更新。如果你新增了权限，就会进入权限升级流程 - 这是设计如此，不是 bug。

扩展进程的日志会出现在：
- `/admin/plugins/<id>` - 界面里显示最近 N 行
- `docker compose logs evey | grep <plugin-id>` - 完整的日志流

## 6. 为你的扩展签名（开发时可选，分发时推荐）

如果你打算把扩展分发给其他 Evey 实例，请为它签名。接收方的管理员就可以使用 `EVEY_PLUGINS_COSIGN_POLICY=warn`（或 `enforce`）来标出未签名的上传。

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

在安装该扩展的 Evey 实例上：

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey 在启动时校验密钥（解析 PEM）- 密钥文件缺失或格式错误会让启动失败，而不是等到第一次安装才失败。KMS 引用（`gcpkms://...`、`awskms://...`、`k8s://...`）会被原样接受。

## 7. 资源限制与沙箱

应用于每个扩展进程的默认值：

| 环境变量 | 默认值 | 作用 |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = 选用可用的最佳方案（通常是 `landlock`）。`landlock` = Landlock LSM 白名单 + 无条件禁止出站 TCP（`-restrict-net` + seccomp `-deny-inet`，`network` 权限也不例外）+ LD_PRELOAD seccomp。`basic` = 仅 LD_PRELOAD seccomp + 切换 UID（调试用）。`bubblewrap` = 完整命名空间 + 根目录只读绑定挂载 + `--unshare-net`（同样无条件，对每个扩展生效，不论是否有 `network` 权限）。`strict` = bubblewrap + seccomp 白名单。 |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | 最大 RSS。资源监控器轮询 `/proc`，超过即停止扩展。`0` 表示禁用。 |
| `EVEY_PLUGINS_CPU_NICE` | 10 | 启动时设置的 `nice` 值（0-19），保证扩展永远不会让服务器抢不到 CPU。 |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | 最大线程/进程数。防止 fork 炸弹。`0` 表示禁用。 |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | 安装目录的最大大小；监控器约每 30 秒测量一次，超过即停止扩展。`0` 表示禁用。 |

需要联网的扩展必须同时具备 `permissions: [network]` 和 `external_fetch: [<host>...]`。直接的出站网络在沙箱层面对每个扩展都被无条件切断，与其权限无关（见上面的沙箱模式）- 扩展进程本身永远无法向互联网打开 socket。唯一的出口是宿主侧的 `Fetch` RPC 代理：扩展通过 `EVEY_PLUGIN_SOCK` Unix socket 调用它，宿主根据 `external_fetch` 检查目标主机，然后通过一个防 SSRF 的客户端发起连接（连接时拒绝私有/保留 IP，重定向会针对解析出的 IP 和白名单主机重新校验）。见 `internal/adapters/driving/pluginhost/fetch.go`。

## 8. 分发扩展

目前还没有官方目录：管理员通过在管理面板里上传扩展的 ZIP 来安装它，ZIP 可以来自你发布的
任何地方（GitHub release、你自己的网站）。如今"分发"意味着：

- 把 ZIP、它的 `.sig` 和你的 `cosign.pub` 一起提供（第 6 节），让接收方管理员可以用
  `EVEY_PLUGINS_COSIGN_POLICY=warn` 或 `enforce` 验证上传。
- 在 README 里写明权限和 `external_fetch` 主机 - 管理员在安装时会看到并必须批准它们；那里
  出现意外，是安装被拒的首要原因。
- 想让你的扩展和内置示例列在一起，就向 `examples/plugins/` 提交一个包含源码和 README 的 PR。
  示例会经过审查并从源码构建；PR 不代表背书，也不带项目的签名。

## 9. 排错

| 现象 | 原因 | 解决办法 |
|---|---|---|
| `bwrap: ... permission denied` | Docker 默认的 seccomp 阻止了 `unshare`。 | 仓库自带一个 seccomp 配置文件（`docker/seccomp.json`）。确认你的 compose 文件应用了它。 |
| `forbidden pattern "fetch(" found in theme JS` | 主题 JS 扫描器发现了被禁用的 API。 | 主题只能调用 `evey.theme.*`。被禁用的内容见 `internal/pkg/themejs/scan.go`。 |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | 所有通配符在安装时都会被拒绝，没有例外。 | 使用 `api.tmdb.org`（并逐一列出你需要的每个子域名），不要用 `*.tmdb.org` 或 `cdn.*.tmdb.org`。 |
| `installer: slot "admin.section" not allowed` | 扩展前端被禁止出现在特权区域。 | 从白名单里选一个非关键插槽，或者重新考虑扩展的设计。 |
| `cosign verify-blob: failed signature` | 服务器上的公钥与签名所用的私钥不匹配。 | 用正确的密钥重新签名，或更新 `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`。 |
| 扩展启动后以退出码 137 结束 | 被 cgroup 因内存不足杀掉。 | 降低内存占用，或调高 `EVEY_PLUGINS_MEMORY_MB`。 |
| 看不到前端模块 | 扩展未启用、插槽不匹配，或 `frontend.src` 中的 `web_root` 前缀错误。 | 查看管理后台的 Plugins 页面（状态徽章），确认插槽名与真实的 `<PluginSlot name=...>` 挂载点一致，并在网络面板里查看对 `/api/v1/plugins/{id}/assets/...` 的请求。 |

## 10. 更进一步

- 端到端的安全模型：[security-model.md](security-model.md)
- 宿主 API 的范围：[api-reference.md](api-reference.md)
- 可用插槽：[api-reference.md](api-reference.md#slots)
- 各语言的 SDK 代码：`sdk/`
- 参考扩展与主题：`examples/plugins/`
