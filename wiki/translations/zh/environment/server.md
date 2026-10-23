---
source_sha: f625c3059af6349cfcff4f14c85f17c7a0a8c26cfeb5772eca562bc3b66c1047
reviewed: true
---
# 服务器与网络 { #server-and-network }

这些变量决定：服务器发布在哪个地址，它信任哪些代理，谁可以从外部访问它，以及它接受多少请求。
大多数只需在 compose 文件里设置一次；之后**管理 → 远程访问**里保存的值会优先于它们，
详见[配置参考](../configuration.md)。

## 地址和端口 { #address-and-ports }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | 公网地址。以 `https://` 开头的地址会开启 `Secure` Cookie、HSTS，并信任 `X-Forwarded-Proto`。邮件里的链接、单点登录的跳转地址和投屏地址都由它生成。 | 设置了远程访问 → **外部 URL** 时以它为准，实时生效；以 `https://` 开头的外部 URL 也会开启 HTTPS |
| `EVEY_SERVER_PORT` | `9090` | 内部 gRPC 端口。HTTP API 和网页界面监听这个端口加一（`9091`）。 | 无 |
| `EVEY_GRPC_BIND` | `127.0.0.1` | gRPC 端口监听的地址。远程访问控制不覆盖 gRPC，所以放宽它（`0.0.0.0`）会记录一条警告，此时需要您自己用防火墙限制该端口。 | 无 |
| `EVEY_SERVER_NAME` | `Evey` | 显示在登录页面、应用的服务器列表和邮件里的名称。 | 服务器配置 → 常规 → **服务器名称**，实时生效 |

## 反向代理 { #reverse-proxy }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | 仅本机回环地址 | 转发来的客户端地址可被信任的网络，用逗号分隔（代理在容器里时：`172.16.0.0/12`，确定代理地址后再缩小范围）。它加在本机回环地址之上，不会取代它；有一项格式错误，服务器就不会启动。 | 远程访问 → **受信任的代理**会实时取代这个列表；清空该字段，这个变量重新生效 |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | 受信任的代理存放客户端地址所用的请求头。 | 无 |
| `EVEY_FORWARDED_PROTO` | 自动推断 | 已弃用：是否使用 HTTPS，由 `EVEY_SERVER_BASE_URL` 的协议推断。仍可用来强制指定，但会给出警告。 | 无 |

哪些代理设置会影响播放：见[反向代理配置](../reverse-proxy.md)。

## 远程访问 { #remote-access }

这些变量只在第一次启动时提供初始值；之后**管理 → 远程访问**会保存自己的值，并且不用重启就能应用。
远程访问控制如何判断：见[远程访问和 HTTPS](../administration/remote-access.md)。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | 未设置 | 未设置：启动时自动推断，`EVEY_SERVER_BASE_URL` 或 `EVEY_REMOTE_ACCESS_EXTERNAL_URL` 是公网地址时开启。`true` 或 `false`：您明确的选择，不会被覆盖。 | 远程访问 → **启用远程访问**，实时生效 |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | 空 | 公网地址，在它和 `EVEY_SERVER_BASE_URL` 不同时使用。以 `https://` 开头的地址会开启 HTTPS 模式，和基础地址一样。 | 远程访问 → **外部 URL**，实时生效 |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | 视为家庭网络的地址段。如果某个 VPN 或 CGNAT 地址段（`100.64.0.0/10`）里的客户端应该算作本地，请把它加进来。 | 远程访问 → **LAN 网络**，实时生效 |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | 家庭网络以外的客户端的码率上限，单位 kbit/s；`0` = 不限。私有地址永远不受限。 | 远程访问 → **最大远程码率**，实时生效 |

## 请求限制 { #request-limits }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | 每个地址每分钟可以尝试登录的次数。 | 服务器配置 → 账户与安全 → **登录速率限制**，实时生效 |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | 每个地址每分钟可以向 API 发出的请求数。 | 服务器配置 → 账户与安全 → **全局速率限制**，实时生效 |

## 容器用户 { #container-user }

由入口脚本读取，不是由服务器读取。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `PUID`, `PGID` | `/media` 挂载目录的所有者 | 服务器以哪个用户和组运行。`1001` 会被拒绝（留给插件使用）。详见[配置参考](../configuration.md#file-ownership-puid-and-pgid)。 | 无 |

## 公开演示 { #public-demo }

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_DEMO` | `false` | 把服务器作为公开演示运行：管理方面的修改被锁定，不发邮件、不推送，也不发出 webhook。不要用于实际的安装。 | 无 |

<!-- sources
internal/config/config.go - EVEY_SERVER_BASE_URL, EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_SERVER_NAME, EVEY_REAL_IP_HEADER, EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_REMOTE_ACCESS_*, EVEY_DEMO, forwardedProtoFromEnv, EffectiveBaseURL
trusted proxies: internal/config/trusted_proxies.go (loopback baseline, malformed entry is an error) ; cmd/evey/admin_ops.go
remote access inference: internal/config/remote_access_resolve.go ; cmd/evey/main.go (RemoteAccessConfigKey loaded from DB)
panel: internal/adapters/driving/http/settings_handler.go (server.name, ratelimit.* live via SetLimit) ; i18n admin.remoteAccess.*, admin.serverSettings.*
puid/pgid: docker/entrypoint.sh
-->
