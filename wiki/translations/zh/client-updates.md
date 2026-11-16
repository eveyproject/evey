---
source_sha: 8b044cd0211312ed2f125ac244e95591553362666e859b5b4092edcb2ecc6c55
reviewed: true
---
# 由实例提供的应用更新 { #app-updates-served-by-the-instance }

手机和电视应用从它们所连接的 Evey 实例加载 JavaScript。这样，应用 JavaScript 的修复不需要在应用商店发布新版本，只要服务器升级，就能到达设备。原生部分的改动仍然需要商店发布。

应用只会向前走：它只接受比当前运行的更新的版本，面对较旧的服务器时保留自己的代码。

## 各部分如何配合 { #how-it-fits-together }

- **镜像带有 bundle。** `make client-updates` 为每个应用构建一个已签名的 bundle，放在 `client-updates/` 中，镜像把这个目录复制到 `/app/client-updates`（`EVEY_CLIENT_UPDATES_DIR`）。没有经过这一步构建的镜像不提供任何更新，应用保留构建时自带的代码。
- **服务器只负责交出字节。** `GET /api/v1/client-updates/{app}/manifest` 和 `.../assets/{platform}/{runtime}/{name}` 使用 Expo Updates 协议（`internal/adapters/driving/http/client_update_handler.go`）。实例不持有任何密钥，不构建任何 manifest，也只打开该目录内的文件。
- **应用负责验证。** 每个 bundle 都由项目签名，并随签名密钥的证书一起发送；应用用编译进自身的根证书 `frontend/client-updates/certificate.pem` 来校验这张证书。实例只能分发已发布的代码，其他任何内容都会在设备上被拒绝。
- **应用从不回退。** 见下文“版本下限”。
- **应用跟随当前服务器。** 选定服务器后，应用把 expo-updates 指向它，并在后台下载它的 bundle（`frontend/packages/client/src/native/clientUpdates.ts`）。bundle 在下次启动时生效；只有当它与正在运行的应用不是同一个应用版本时才会提示重新启动，而且从不出现在播放器上方。没有选定服务器时，应用不向任何人查询。

## 原生运行时版本 { #native-runtime-version }

一个 bundle 只能在它对应的原生构建上运行。每个应用在自己的 `app.json` 中声明一个 `runtimeVersion`（`mobile-1`、`tv-1`），bundle 就以这个名称存放和请求。

**凡是原生部分有改动，都要在同一次修改中提高 `runtimeVersion`**：新增或升级原生模块、升级 Expo SDK 或 React Native、配置插件、Android manifest，或绑定进 `offline-audio` 的 Go 代码。手机应用的 Android 工程保存在 git 中，所以这个值同时也在 `frontend/apps/mobile/android/app/src/main/res/values/strings.xml` 里。

实例没有为它构建时不包含的运行时准备 bundle：更旧或更新的商店构建只会保留自己的代码。

## 版本下限 { #release-floor }

签名说明的是谁发布了某个版本，而不是它是否够新。如果没有别的措施，任何服务器，或者明文 HTTP 连接路径上的任何人，都可以把一个旧的已签名版本交给应用，让已经修复的缺陷重新出现。

- 版本的日期由构建它的 commit 决定（已签名 manifest 中的 `createdAt`，即 committer 时间）。
- 每台设备保存一个下限：商店构建所基于的 commit（由 Gradle 记录），与由服务器提供并在该设备上显示过的最新版本，两者中较新的那个。下限只升不降。
- 应用不下载低于下限的任何内容，并启动自己持有的、不低于下限的最新版本，无论它来自哪台服务器。不提供任何内容或只提供更旧内容的服务器，不会改变应用现有的代码。
- 应用内置的代码始终可以运行。当存储的内容都不符合条件时，启动的就是它。它的日期同样由 commit 决定，而不是应用的编译时刻，因此同一个 commit 以及任何更晚 commit 的版本都会优先于它。
- JavaScript 一侧也保存同样的下限，并拒绝在下限之下运行。

由此得出：

- 面对较旧的服务器，应用运行的 JavaScript 比该服务器自带的更新，就像商店构建一样。服务器 API 必须与更新的客户端保持兼容。
- 当保护机制丢弃一个已经显示过的版本时，它之前的版本通常低于下限：应用会运行内置代码，直到实例发布更新的版本。
- 无法读取下限的构建不接受任何更新。
- 同一个 commit 的两次构建是日期几乎相同的两个版本。应用对每个日期只保存一个版本：如果两个日期恰好相同，第二个不会被保存，应用保留正在运行的版本。每个版本只构建一次。

没有任何未签名的内容能左右这些行为：应用忽略 manifest 过滤器、服务器定义的标头、extensions 部分和指令，也不向服务器发送安装标识或上一次崩溃的文本。

## 更新出问题时 { #when-an-update-misbehaves }

- 在第一次显示之前就崩溃的 bundle 会被 expo-updates 丢弃，回到上一个 bundle 或应用内置的代码。
- 能显示但每次启动都会退出的 bundle 由应用自己的保护机制处理（`clientUpdateBootGuard`）：连续三次启动都没有稳定下来就结束之后，下一次启动会请求同样的回退。
- 被丢弃的 bundle 不会再试。实例必须发布另一个。
- 当被丢弃的 bundle 之后没有可启动的内容（它之前的版本低于下限）时，应用会自行用内置代码重新启动。

## expo-updates 补丁 { #the-expo-updates-patch }

`frontend/patches/expo-updates@*.patch` 包含这套设计需要、而 expo-updates 没有对应设置的内容：

- manifest 中的资源地址可以是相对地址，相对于 manifest 的地址解析，这样同一份已签名的 manifest 在每个实例上都有效；
- 保护机制的回退请求会丢弃更新，即使它之前成功启动过；
- 当这样做之后没有可启动的内容时，应用会重新启动，而不是崩溃；
- 版本下限，以及按版本而不是按服务器地址来启动；
- 应用内置代码的日期由它的 commit 决定；
- 已经保存的版本在由另一个地址提供时会被直接采用，不再重新下载（同一个实例分别从局域网和外网访问）；
- 每个资源都必须在已签名的 manifest 中带有哈希；
- 证书按设备时钟和下限中较晚的时间校验，版本的日期不能晚于其签名证书的到期时间；
- 上面列出的未签名输入和可识别身份的请求头都被去掉。

升级 expo-updates 时要重新制作补丁；依赖已固定在打过补丁的确切版本上。

## 构建必须具备的条件 { #what-a-build-must-have }

`frontend/scripts/check-expo-updates-patch.mjs` 对两个应用都进行检查：补丁已经在 Android 编译的源码里，证书及其元数据已配置（没有证书时 expo-updates 什么都不验证），证书是根证书且应用通过它验证，构建记录了版本下限，没有任何设置放宽应用接受的内容，原生运行时版本与 `app.json` 一致。它在应用的每次 Gradle 构建之前运行，也在 gate 中对任何改动了依赖、依赖补丁、证书或应用原生部分的分支运行。

## 密钥 { #keys }

两个密钥，由 `frontend/scripts/client-update-certificate.sh` 生成：

- **根**（`root <dir>`），只生成一次。它的证书是 `frontend/client-updates/certificate.pem`，编译进每个商店构建；它的密钥用口令加密并离线保存，另有备份。它从不放到构建机器上。丢失它或不得不更换它，就意味着两个应用都要发布新的商店构建，之后才能再次提供更新。
- **签名证书**（`leaf <dir> [days]`），默认有效期一年，由根签发。它的密钥由发布构建持有。

请在签名证书到期前续期：发布工具拒绝剩余不足 30 天的证书，设备会拒绝证书已过期的版本，因此镜像在其证书签发一年后不再交出它的 bundle（设备保留已有的内容）。没有吊销机制：泄露的签名密钥在证书到期前一直有效，而且只对日期早于到期时间的版本有效。更换根是唯一能提前截止的办法。

## 发布版本 { #releasing }

1. `EVEY_CLIENT_UPDATE_KEY_FILE=<signing key> EVEY_CLIENT_UPDATE_CERT_FILE=<signing certificate> make client-updates`
   执行一次，在发布的 commit 上，工作区干净，并且在构建镜像之前。Hermes 字节码不可复现：运行两次会得到两个不同的更新，所以一个版本的所有架构都必须带同一份输出。该工具拒绝在已有更新版本的情况下发布。
2. 构建镜像。如果 bundle 未签名、来自另一个 commit，或对应应用已不再声明的运行时，构建会失败。

CI 只对版本标签（`v*.*.*`）执行这些步骤，分为两个 job（`.github/workflows/ci.yml`）：

- 一个构建未签名的 bundle，带有应用的全部依赖，没有任何密钥；
- 一个对它们签名，只有密钥和一次 checkout：`frontend/scripts/sign-client-update.mjs` 不加载任何依赖，并且拒绝这样的 bundle：不属于打了标签的 commit、不对应应用在该 commit 声明的运行时、日期不是该 commit 的日期，或文件与 manifest 所述不符。

随后每个架构的镜像构建都使用同一份已签名的输出。如果某个标签的 bundle 无法签名，这次发布会失败，而不是不带 bundle 发布。

签名不覆盖的部分：构建 job 会运行应用的依赖，其中某个依赖可能在签名之前改动 JavaScript。签名 job 检查的是 bundle 的结构、commit 和日期，而不是它的字节码做了什么。防线是冻结的 lockfile 和对每次依赖升级的审查。

从分支构建或手工构建的镜像不带 bundle。为某个分支签名的 bundle 会被所有已发布的应用接受，无论来自哪台服务器，并把它们的下限抬到正式版本之上。

签名 job 在 GitHub 环境 `client-update-signing` 中运行，该环境保存两个 secret：`EVEY_CLIENT_UPDATE_SIGNING_KEY` 和 `EVEY_CLIENT_UPDATE_SIGNING_CERT`（都是完整的 PEM 文件），并且只允许 `v*.*.*` 标签进入：改动了 workflow 的分支读不到它们。签名证书续期时，两个都要更换。

任何私钥都不会进入仓库或镜像，密钥也只以文件路径的形式提供。

<!-- sources
routes and what the server serves: internal/adapters/driving/http/client_update_handler.go ; internal/adapters/driving/http/client_update_handler_test.go ; internal/config/config.go - EVEY_CLIENT_UPDATES_DIR
following the active server, restart prompt, launch guard: frontend/packages/client/src/native/clientUpdates.ts - followClientUpdateSource, clientUpdateBootGuard, clientUpdateMarkSettled ; frontend/packages/client/src/native/clientUpdates.test.ts ; i18n common.clientUpdate.*
floor, launch by release, certificates, dropped inputs: frontend/patches - the expo-updates patch
build checks: frontend/scripts/check-expo-updates-patch.mjs ; Dockerfile
keys and publication: frontend/scripts/client-update-certificate.sh ; frontend/scripts/build-client-update.mjs ; frontend/scripts/sign-client-update.mjs ; .github/workflows/ci.yml (client-updates-build, client-updates-sign)
-->
