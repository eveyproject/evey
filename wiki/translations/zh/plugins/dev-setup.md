---
source_sha: c9ccdddebbef17e96ff8f3da3475c803b0d09dc237cd4c140c897bdfdaf289c5
reviewed: false
---
# 编写插件

为刚克隆了 Evey、想要编写、测试和发布插件或主题的人准备的完整流程。选择与你的插件语言相匹配的运行时；其余部分都是通用的。

## 1. 前提条件

Evey 运行宿主进程（Go），并以 ZIP 形式分发插件。插件运行时取决于你的代码调用什么。

### 始终需要

| 工具 | 原因 | 安装 |
|---|---|---|
| Go ≥ 1.27 | 构建 Evey 宿主 | `https://go.dev/dl/` |
| Docker + Docker Compose | 默认的开发环境（Postgres、Meilisearch、服务器、网页） | https://docs.docker.com/engine/install/ |
| `bubblewrap`（`bwrap`） | 插件进程的沙箱，`bubblewrap` 和 `strict` 模式需要它 | `apt install bubblewrap`（Debian/Ubuntu）、`dnf install bubblewrap`（Fedora）、`pacman -S bubblewrap`（Arch）。macOS：在虚拟机中安装 Linux，bwrap 只能在 Linux 上运行。 |

`basic` 沙箱模式不需要 bwrap 也能运行，但除了一次性的本地实验之外绝不应该使用：尽快弃用它。

### 按语言

选择你的插件实际使用的；不需要全部安装。

| 插件语言 | 工具 | 说明 |
|---|---|---|
| Python 3 | `python3`、`pip` | 使用 `pip install --user` 保持主机干净。SDK 存根位于 `sdk/python/`。 |
| Node.js | Node 20+、`npm` 或 `pnpm` | SDK 在运行时通过 `@grpc/proto-loader` 加载 `plugin_api.proto`。 |
| Go | Go ≥ 1.21 | 没有附带存根：用 `protoc-gen-go` + `protoc-gen-go-grpc` 生成；编译后的 Go 二进制文件还需要 `native-binary` 权限。 |
| Dart / Java / PHP / Ruby / Swift | 各语言的工具链 | 存根已预先生成在 `sdk/<lang>/` 中。 |
| 纯 CSS 主题 | 不需要额外工具 | 主题是一个包含 `kind: theme` + `theme.css` 的 ZIP。 |
| 主题 JS | 不需要额外工具（宿主在带有 `sandbox="allow-scripts"` 和锁定 CSP 的沙箱化 `iframe` 中运行你的 JS） | 静态扫描器会拒绝 Cookie、存储、网络调用以及对宿主页面的访问（`document.cookie`、`localStorage`、`fetch(`、`window.parent`……）：见[主题](../themes.md#what-the-scanner-refuses)。 |

### 为其他语言生成存根

上面这些语言的存根是预先生成的，但插件的 gRPC 契约只是一个 proto 文件：你可以面向 **`protoc` 支持的任何语言**（Rust、C#、Kotlin、Elixir……）。唯一的事实来源是：

```
proto/plugin/v1/plugin_api.proto
```

用 `protoc`（或 `buf`）生成，把你所用语言的 gRPC 插件指向它：

```bash
# 通用形式：具体的 --*_out 选项取决于语言或插件
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

附带语言的现成做法（每种语言需要哪个 gRPC 插件：`grpc_php_plugin`、`protoc-gen-grpc-java`、`protoc-gen-grpc-swift`……）位于根目录 `Makefile` 的 `plugin-sdk` 目标中，以及各个 `sdk/<lang>/README.md` 中（例如 `sdk/php/README.md`）。`make plugin-sdk` 会重新生成附带的存根。

把生成的存根放进你的插件 ZIP 中：宿主只会自动复制它附带的语言的存根（`sdk/<lang>/`）。

### 可选但推荐

| 工具 | 原因 |
|---|---|
| `cosign`（Sigstore） | 为你的插件 ZIP 签名。`EVEY_PLUGINS_COSIGN_POLICY` 默认为 `warn`（不是 `off`）：在 `warn` 且没有配置公钥时，验证不做任何事（记录一条警告，不阻止任何东西）；设置 `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` 才会真正验证签名，把策略设为 `enforce` 才会拒绝未签名的上传（没有公钥的 `enforce` 会让服务器在启动时停止，而不是不经验证就接受所有插件）。https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | 在提交插件之前发现明显的问题。 |
| `golangci-lint` | 如果你的插件是用 Go 写的，并且想要与宿主相同的代码检查，就需要它。 |

## 2. 在本地启动 Evey

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # 生成一份合理的本地配置
make dev                        # docker compose up：postgres、meilisearch、服务器（Air）、网页（Vite）
```

服务器健康后，在浏览器中打开 https://evey.test（由 Caddy 提供）。设置向导会在首次启动时创建管理员账户，如[首次设置](../getting-started/first-setup.md)所述。

如果 `make dev` 因 `bwrap` 失败：
- 确认 `which bwrap` 在主机上返回一个路径
- 在 Docker 中，开发容器自带 bwrap 二进制文件；如果你自定义了 Compose 配置，把它挂载进来，或者*仅为开发容器*设置 `EVEY_PLUGINS_SANDBOX_MODE=basic`

## 3. 搭建你的第一个插件

示例位于 `examples/plugins/`：Node.js 和 Python 的起步模板（`nodejs-example`、`python-example`）、一个元数据提供者（`metadata-example`）、一个 Webhook 接收器（`webhook-receiver`）、一个首页小部件（`weather`）、插件配置（`config-demo`）和两个主题（`theme-dark-blue`、`open-workshop`）。选择最接近你用例的一个，复制到工作目录中。关于持久化和权限功能，见 **`examples/plugins/data-demo`**：它使用插件自己的数据存储，并请求 `public` / `prefs` 权限以及 `admin.nav` 插槽。

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

编辑 `plugin.yaml`：

- `name`：kebab-case，唯一
- `version`：semver，每次发布都要递增
- `kind`：`plugin`（必须有后端）或 `theme`（仅前端）
- `language`：`python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh`（清单标识符，而不是解释器二进制文件的名称；`python3`/`node` 不是有效的 `language` 值，见下面的 `entrypoint`）。
- `entrypoint`：运行时执行的命令行
- `permissions`：声明你调用的每个宿主 API。安装器会拒绝未声明的调用。完整列表见[权限目录](security-model.md#permissions-catalog)。
- `external_fetch`：当你需要与第三方 API 通信时。条目是纯主机名（`api.example.com`）：不需要也不使用协议（任何 `https://` 前缀都会被解析，只保留主机名），不能使用通配符（`*.example.com` 在安装时会被拒绝：`external_fetch entry %q is a wildcard - use explicit hostnames`），不能使用字面 IP 地址（所有 IP，不仅仅是 RFC1918）。验证器位于 `internal/adapters/driven/pluginrunner/manifest.go`（`validateExternalFetch`）；它的测试位于 `internal/adapters/driven/pluginrunner/manifest_test.go`：运行 `go test ./internal/adapters/driven/pluginrunner/...` 检查你的值是否有效。

前端模块（对 `kind: plugin` 可选，对 `kind: theme` 禁止）：

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

插槽必须存在于 `internal/domain/plugin/slots.go` 中（`home.section`、`home.hero`、`library.sidebar`、`media.detail.sidebar`、`player.overlay`、`sidebar.bottom`、`settings.user.appearance`，以及 `admin.nav`，这是管理区唯一受控的插槽，一个打开你的插件自己的沙箱化页面的菜单项）。其他任何插槽（注入 `admin.*`、`login.*`、`setup.*`、`profile.security.*`）在安装时都会被拒绝。

**存储数据。** 你的后端通过宿主 socket 获得一个私有的键值存储（`SetData`/`GetData`/`DeleteData`/`ListDataKeys`）：不需要任何权限，卸载时自动清除。要从你的 iframe 保存每个用户的偏好，请求 `prefs` 权限；要只读访问非敏感的媒体信息（标题/海报），请求 `public`。见 [API 参考](api-reference.md#permissions)。管理、认证、系统和通用写入范围永远无法请求。

## 4. 运行安装循环

```sh
# 压缩你的插件
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# 通过管理界面上传
open https://evey.test/admin/plugins
# 或者通过 API
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# 签名放在第二个字段中： -F "sig=@my-first-plugin.zip.sig"
# 请求新权限的更新： -F "accept_permission_changes=true"
```

安装器按以下顺序运行，每一步都可能拒绝安装：

1. **清单验证**：直接从 ZIP 中解析 `plugin.yaml`（在下面的 ZIP 级检查之前），并检查其架构。插槽白名单。权限/能力白名单。`external_fetch` 解析器（拒绝通配符和任何字面 IP 地址；`http://` 之类的协议前缀会被接受但忽略：只保留主机名）。声明的 `language` 运行时必须在镜像中可用。
2. **权限提升**（仅更新）：安装器把新的清单与之前的版本进行比较。任何新请求的权限、能力、插槽或 external_fetch 主机都会以一个错误阻止安装，调用方必须明确覆盖它（从前端传入 `accept_permission_changes: true`）。全新安装跳过这一步：没有可比较的对象。
3. **Cosign 签名**：在配置了签名验证器时进行验证；只有在 `EVEY_PLUGINS_COSIGN_POLICY=enforce` 时，失败才会阻止安装。见第 6 节。
4. **ZIP 解压**：扩展名、总大小 ≤ 500 MB、单个条目大小 ≤ 100 MB、压缩比 ≤ 100:1、没有符号链接、没有路径穿越条目；文件进入一个暂存目录（在后续所有步骤通过之前，升级永远不会触碰正在运行的版本）。
5. **CSS 清理器**（仅主题）：去掉 `@import`，阻止非 `data:` 的 `url()`，阻止全屏叠加层，阻止针对 `[type=password]` 和 `.admin-*` 的选择器，把 `z-index` 限制在 ≤ 999。
6. **主题 JS 扫描器**（仅主题）：拒绝任何对 `document.cookie`、`localStorage`、`fetch`、`eval`、`globalThis` 的引用，拒绝对宿主全局对象使用方括号访问属性、`constructor.constructor` 等。主题只能使用 `evey.theme.*` API。
7. **静态分析器**：扫描每个文本文件，查找 shell 执行、反向 shell、eval、base64 解码后执行、混淆模式、运行时安装命令、权限提升。严重发现 = 拒绝安装。警告级发现（反序列化、混淆、native_code）会升级为拒绝，除非声明了对应的权限。
8. **持久化**：把暂存目录换到正式位置，在 Postgres 中写入记录，并按需启动运行时。

如果任何一步失败，管理界面会显示确切的原因。服务器日志（`docker compose logs evey`）带有完整的堆栈。

## 5. 迭代

开发时热重载你的插件：

```sh
# 编辑代码 → 重新压缩 → 通过界面重新上传（更新按钮）
# 或者更快：直接把文件放进容器内的插件目录
docker compose exec evey ls /data/plugins/<plugin-id>/
```

只有当上传的 `version` 高于已安装的版本时，它才会被当作更新接受：每次迭代都要递增它（相同或更低的版本会被拒绝），或者先卸载。如果你添加了新权限，就会进入权限提升流程：这是设计如此，不是缺陷。

插件进程的日志出现在：
- `/admin/plugins/<id>`：界面中最后 N 行
- `docker compose logs evey | grep <plugin-id>`：完整的流

## 6. 为你的插件签名（开发时可选，分发时推荐）

如果你打算把插件分发给其他 Evey 实例，请为它签名。接收方的管理员就可以使用 `EVEY_PLUGINS_COSIGN_POLICY=warn`（或 `enforce`）来发现未签名的上传。

```sh
# 一次性：生成密钥对（妥善保管 cosign.key，绝不要提交它）
cosign generate-key-pair

# 为 ZIP 签名
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# 分发： my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

在安装插件的 Evey 实例上：

```sh
# 把公钥放在服务器能读取的地方
sudo cp cosign.pub /etc/evey/cosign.pub

# 让 Evey 指向它并切换策略
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # 或者用 "enforce" 拒绝未签名的插件
docker compose restart evey
```

Evey 在启动时验证密钥（PEM 解析）：缺失或格式错误的密钥文件会让启动失败，而不是让第一次安装失败。KMS 引用（`gcpkms://...`、`awskms://...`、`k8s://...`）会被原样接受。

## 7. 资源限制与沙箱

应用于每个插件进程的默认值：

| 环境变量 | 默认值 | 作用 |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = 选择可用的最佳模式（通常是 `landlock`）。`landlock` = Landlock LSM 允许列表 + 无条件拒绝 TCP 出站（`-restrict-net` + seccomp `-deny-inet`，`network` 权限也没有例外）+ LD_PRELOAD seccomp。`basic` = 仅 LD_PRELOAD seccomp + UID 降权（调试用）。`bubblewrap` = 完整的命名空间 + 只读绑定挂载根目录 + `--unshare-net`（同样是无条件的，对所有插件，无论是否有 `network` 权限）。`strict` = bubblewrap + LD_PRELOAD seccomp 垫片。 |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | 最大 RSS。资源监视器轮询 `/proc`，超过时停止插件。`0` 表示关闭。 |
| `EVEY_PLUGINS_CPU_NICE` | 10 | 在启动时应用的 `nice` 值（0-19），这样插件永远不会让服务器缺少 CPU。 |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | 最大线程/进程数。防止 fork 炸弹。`0` 表示关闭。 |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | 安装目录的最大大小；监视器大约每 30 秒测量一次，超过时停止插件。`0` 表示关闭。 |

需要联网的插件同时需要 `permissions: [network]` 和 `external_fetch: [<host>...]`。无论权限如何，所有插件的直接网络出站都在沙箱中被无条件切断（见上面的沙箱模式）：插件进程本身永远无法打开通往互联网的 socket。唯一的出路是宿主端的 `Fetch` RPC 代理：插件通过 `EVEY_PLUGIN_SOCK` Unix socket 调用它，宿主对照 `external_fetch` 检查目标主机，然后通过一个防 SSRF 的客户端连接（在连接时拒绝私有和保留 IP，重定向会同时对照解析出的 IP 和允许的主机重新验证）。见 `internal/adapters/driving/pluginhost/fetch.go`。

## 8. 分发插件

目前还没有官方目录：管理员通过在管理面板中上传 ZIP 来安装插件，ZIP 来自你发布它的任何地方（GitHub release、你自己的网站）。目前“分发”的含义是：

- 把 ZIP、它的 `.sig` 和你的 `cosign.pub` 一起发布（第 6 节），这样接收方管理员可以用 `EVEY_PLUGINS_COSIGN_POLICY=warn` 或 `enforce` 验证上传。
- 在 README 中写明权限和 `external_fetch` 主机：管理员在安装时会看到并需要批准它们；那里出现意外是安装被拒绝的首要原因。
- 要让你的插件与附带的示例列在一起，向 `examples/plugins/` 提交一个包含源码和 README 的 PR。示例会被审查并从源码构建；PR 不代表认可，也不带有项目的签名。

## 9. 故障排除

| 症状 | 原因 | 解决办法 |
|---|---|---|
| `bwrap: ... permission denied` | Docker 中的 `bubblewrap` 模式：默认的 seccomp 和 AppArmor 配置会阻止它需要的 `unshare` 和 `mount` 调用。 | 保持默认的 `auto` 模式，它在容器中使用 Landlock。否则应用附带的配置 `docker/seccomp-evey.json`（镜像中也有，位于 `/etc/evey/seccomp.json`），这是一次有意的安全降级：见[插件沙箱设置](sandbox-deployment.md)。 |
| `forbidden pattern "fetch(" found in theme JS` | 主题 JS 扫描器发现了被禁止的 API。 | 主题只能调用 `evey.theme.*`。阅读 `internal/pkg/themejs/scan.go` 了解哪些会被阻止。 |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | 所有通配符在安装时都会被拒绝，没有例外。 | 使用 `api.tmdb.org`（并列出你需要的每个明确的子域名），而不是 `*.tmdb.org` 或 `cdn.*.tmdb.org`。 |
| `manifest: frontend slot "admin.section" not allowed` | 插件前端被禁止进入特权区域。 | 从白名单中选择一个非关键插槽，或者重新考虑插件的设计。 |
| `cosign verify-blob: failed signature` | 服务器上的公钥与签名所用的私钥不匹配。 | 用正确的密钥重新签名，或者更新 `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`。 |
| `plugin already installed at version 1.0.0, new version must be higher` | ZIP 带有已经安装的版本。 | 递增 `plugin.yaml` 中的 `version`，或者先卸载插件。 |
| 插件停止，日志显示 `memory quota exceeded (… MB > … MB)` | 资源监视器在它超过内存限制时将其终止（退出码 137）。 | 降低内存使用，或者在 **Admin → 插件** 中它的卡片上提高插件的内存限制（`EVEY_PLUGINS_MEMORY_MB` 设置默认值）。 |
| 前端模块不可见 | 插件未启用、插槽不匹配，或者 `frontend.src` 中的 `web_root` 前缀错误。 | 检查管理插件页面（状态标记），确认插槽字符串与一个真实的 `<PluginSlot name=...>` 挂载点匹配，在网络面板中查看 `/api/v1/plugins/{id}/assets/...` 请求。 |

## 10. 进一步了解

- 端到端的安全模型：[security-model.md](security-model.md)
- 宿主 API 一览：[api-reference.md](api-reference.md)
- 可用的插槽：[api-reference.md](api-reference.md#slots)
- 各语言的 SDK 代码：`sdk/`
- 参考插件和主题：`examples/plugins/`
