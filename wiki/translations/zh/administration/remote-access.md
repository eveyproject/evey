---
source_sha: b8448a636921ec7091b9ab98200600dd596b087d5b5d02b1d71230f1dbda6d3d
reviewed: true
---
# 远程访问和 HTTPS

在家里，Evey 的地址类似 `http://192.168.1.20:9091`。要在家庭网络以外的地方观看，您还需要两样东西：

- **一个公网地址**，例如 `https://evey.example.org`：一个指向您家中网络的域名。
- **HTTPS**，让密码和视频在互联网上加密传输。Evey 自己不做 HTTPS，而是由**反向代理**来做：它是放在 Evey 前面的一个小程序，负责安全连接，再把每个请求转交给 Evey。

本页先讲如何把服务器发布出去，再讲每项设置的作用。

## 三步发布服务器

1. **设置一个反向代理**，由它处理 HTTPS，并把所有请求转发到 Evey 的端口 `9091`。您可以使用 Evey 自带的 Caddy 配置，也可以用您已经在运行的代理。现成的配置见[反向代理配置](../reverse-proxy.md)。
2. **在 Evey 中进行设置**，位置是**管理 → 远程访问**：
    - **外部 URL**：您的公网 `https://` 地址。
    - **受信任的代理（IP 或 CIDR，逗号分隔）**：只有当代理运行在另一个容器里时才需要填。把它所在的网络填在这里，例如 Docker 默认网络是 `172.16.0.0/12`。
3. **设置完成。** 服务器一旦有了公网地址，就会自动开启远程访问。

要检查一下，请运行 `docker exec evey-server evey admin doctor`。它会显示正在使用的受信任代理；如果一台已发布的服务器正在拒绝来自家庭网络以外的人，它也会发出警告。

## 远程访问由公网地址决定

除非您自己选择，否则 Evey 会在启动时决定：

- **服务器有公网地址**（**外部 URL**，或者变量 `EVEY_SERVER_BASE_URL` 或 `EVEY_REMOTE_ACCESS_EXTERNAL_URL`）：远程访问**开启**。
- **没有公网地址**：远程访问**关闭**。只有您家庭网络里的设备能连接。

只要您手动切换过**启用远程访问**，Evey 就会保留您的选择，不再替您决定。所以，一台已发布的服务器也可以按您的意愿仅在家庭网络中使用。

远程访问只决定谁能访问服务器。它本身并不是一种保护：每个页面仍然要求登录。

**远程访问关闭时**，家庭网络以外的人在每个页面上都会被拒绝（错误 `403`）。应用会显示**远程访问已禁用**，而不是一个含糊的错误。如果服务器能从互联网访问，而远程访问却是关闭的，Evey 会在好几个地方提醒您：

- **管理 → 远程访问**显示**所有远程用户均已被锁定**；
- 服务器日志在启动时会提到这一点，之后每次拒绝都会记录请求来自哪个地址；
- `evey admin doctor` 会把它报告为失败。

## 设置

都在**管理 → 远程访问**里。每项设置都无需重启就会生效：网络设置从下一个请求开始，码率从下一次播放开始。

<figure class="evey-shot" markdown="span">
  ![“管理 → 远程访问”，带有 HTTPS 外部 URL 和一个受信任的代理](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>一台在 <code>https://</code> 地址发布的服务器，位于 Docker 网络上的代理后面（界面为英文）。</figcaption>
</figure>

| 设置 | 作用 |
| --- | --- |
| **启用远程访问** | 开启：家庭网络以外的人可以像其他人一样连接。关闭：他们会被拒绝（`403`）。 |
| **外部 URL** | 您的公网地址。邮件里的链接、单点登录和投屏都会用到它。`https://` 地址还会开启 HTTPS 模式（见下文）。 |
| **受信任的代理（IP 或 CIDR，逗号分隔）** | 这些反向代理告诉 Evey 真实访客是谁时，Evey 会相信它们。本机（环回地址）始终受信任。留空时，列表取自 `EVEY_TRUSTED_PROXIES`。 |
| **LAN 网络（CIDR，逗号分隔）** | Evey 视为“家庭网络”的地址范围。默认已列出私有地址范围。如果您使用 VPN（Tailscale、WireGuard），并希望它的用户也算作家庭网络，请把它的地址范围加在这里。 |
| **最大远程码率（Kbps，0 = 无限制）** | 对家庭网络以外的人设定的质量上限。私有地址永远不受限制。如果**管理 → 转码**也为远程观众设了上限，就采用较低的那个。见[播放方式是如何决定的](../playback/how-playback-is-decided.md#limits-on-the-session)。 |

## Evey 需要了解代理的哪些情况

Evey 在端口 9091 上用普通 HTTP 应答，HTTPS 由反向代理处理。关于代理，Evey 需要知道两件事，都在**管理 → 远程访问**里设置：

- **`https://` 开头的外部 URL。** 这告诉 Evey 有代理在处理 HTTPS。于是 Evey 会保护它的登录 Cookie（`Secure` 标记），告诉浏览器始终使用 HTTPS（HSTS），并相信受信任代理发来的 `X-Forwarded-Proto` 头。
- **受信任的代理**：当代理和 Evey 不在同一台机器上时需要。位于另一个容器里的代理，需要把它的网络填在这里，例如 Docker 默认网络是 `172.16.0.0/12`。

!!! warning "覆盖所有地址的范围会被拒绝"
    把 `0.0.0.0/0` 或 `::/0` 设为受信任代理，会让任何访客都能冒充来自任意地址。管理面板和 `evey admin doctor` 都会拒绝它。请改为填写代理自己的地址或网络。

### 改用环境变量设置

变量 `EVEY_SERVER_BASE_URL` 和 `EVEY_TRUSTED_PROXIES` 可以在 compose 文件里完成同样的设置。如果您的整套配置都写在文件里，或者服务器从第一次启动起就必须通过 HTTPS 应答，这种方式比较合适。

两边都设置时，以管理面板为准。即使基础 URL 是普通 HTTP，`https://` 开头的**外部 URL** 也会开启 HTTPS 模式；保存过的代理列表会取代变量。清空这一栏，就改为使用变量的值。见[服务器与网络](../environment/server.md#remote-access)。

## 客户端的真实地址

要让远程访问和远程质量上限起作用，Evey 必须看到每个访客的真实地址。有两项设置一起起作用：

1. 受信任的代理决定 Evey 采信哪些代理提供的访客真实地址。
2. 然后，LAN 网络决定这个地址属于“家庭网络”还是“远程”。

如果代理没有转发真实地址，或者 Evey 不信任这个代理，那么每个请求看起来都来自代理自己的私有地址。这样一来，所有人都被视为在家庭网络内：远程访问设置不起作用，也不会套用任何远程上限。

!!! note "修正代理设置会改变哪些人被视为远程"
    如果远程访问原本是关闭的，那么从代理开始转发真实地址的那一刻起，家庭网络以外的人就会被拒绝。修改代理设置后，请运行 `evey admin doctor`：它会显示正在使用的列表及其来源，对公网地址范围发出警告，并对覆盖所有地址的范围报告失败。

<!-- sources
inference and explicit choice: docs/adr/0026-remote-access-inferred-from-a-published-url.md ; internal/config/remote_access_resolve.go - ResolveRemoteAccess, IsPubliclyReachableURL
gate, live switch and LAN list: cmd/evey/gateway.go - networkMiddleware ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_LANListChangeAppliesWithoutRestart
remote cap live: internal/adapters/driving/grpcapi/playback_remote_bitrate_test.go
labels: i18n admin.remoteAccess.* (enableRemoteAccess, externalUrl, trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle)
proxy settings: docs/reverse-proxy.md
https mode and trusted proxies live: internal/config/network_live.go - EffectiveHTTPS, EffectiveTrustedProxies ; internal/adapters/driving/http/access_handler_live_test.go
catch-all range refused: internal/config/trusted_proxies.go - IsCatchAllProxyRange ; internal/config/network_live.go
doctor check: cmd/evey/admin_ops.go - checkTrustedProxies
exact field labels: i18n admin.remoteAccess.trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle ; apps message: i18n auth.remoteAccess.title
-->
