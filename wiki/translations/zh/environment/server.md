---
source_sha: d3bbf5c5d256e29ed7bd2108d72166e0906938ea2e32684ab5496cdb6bb7755e
reviewed: false
---
# 服务器与网络

服务器发布在哪个地址、它信任谁、谁可以从外部访问它，以及它的请求限制。

## 地址与端口

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | 公网地址。`https://` 地址会开启 `Secure` Cookie、HSTS 以及对 `X-Forwarded-Proto` 的信任。电子邮件中的链接、单点登录的重定向和投放 URL 都由它构建，除非设置了 **Admin → 远程访问 → 外部 URL**，后者优先。即使这里是未加密的 HTTP，`https://` 外部 URL 也会开启 HTTPS。 |
| `EVEY_SERVER_PORT` | `9090` | 内部 gRPC 端口。HTTP API 和网页界面监听在这个端口加一（`9091`）。 |
| `EVEY_GRPC_BIND` | `127.0.0.1` | gRPC 端口监听的地址。远程访问限制不覆盖 gRPC，所以放宽它（`0.0.0.0`）会记录一条警告，并让防火墙防护成为你的责任。 |
| `EVEY_SERVER_NAME` | `Evey` | 显示在登录页面、应用的服务器列表和电子邮件中的名称。**Admin → 服务器配置** 会覆盖它。 |

## 反向代理

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | 仅环回地址 | 其转发的客户端地址会被信任的网络，逗号分隔（容器中的代理：`172.16.0.0/12`，知道后再收窄）。添加到环回地址之上，从不取代它。保存在 **Admin → 远程访问 → 受信任的代理** 中的列表会实时取代它；清空该字段会恢复它。 |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | 受信任代理放置客户端地址的请求头。 |
| `EVEY_FORWARDED_PROTO` | 自动推导 | 已弃用：HTTPS 由 `EVEY_SERVER_BASE_URL` 的协议推导。仍可作为覆盖使用，但会给出警告。 |

## 远程访问

这些只为首次启动提供初始值；之后 **Admin → 远程访问** 会保存自己的值，并无需重启即可应用。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | 未设置 | 未设置：在启动时推导，当 `EVEY_SERVER_BASE_URL` 或 `EVEY_REMOTE_ACCESS_EXTERNAL_URL` 是公网地址时开启。`true` 或 `false`：你的明确选择，永远不会被覆盖。 |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | 空 | 与 `EVEY_SERVER_BASE_URL` 不同时的公网地址。`https://` 地址会开启 HTTPS 模式，与基础 URL 一样。 |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | 被视为家庭网络的网络。添加客户端应算作本地的 VPN 或 CGNAT 地址范围（`100.64.0.0/10`）。 |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | 对家庭网络之外客户端的码率上限，单位 kbit/s；`0` = 无。私有地址从不受限。 |

限制如何工作：[远程访问与 HTTPS](../administration/remote-access.md)。

## 请求限制

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | 每个地址每分钟的登录尝试次数。**Admin → 服务器配置 → 服务器设置** 会覆盖它。 |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | 每个地址每分钟对 API 的请求数。可以在同一个地方覆盖。 |

## 容器用户

由入口脚本读取，而不是由服务器读取。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `PUID`、`PGID` | `/media` 挂载的所有者 | 服务器运行时使用的用户和组。`1001` 会被拒绝（为插件保留）。详见[配置参考](../configuration.md#file-ownership-puid-and-pgid)。 |

## 公开演示

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_DEMO` | `false` | 把服务器作为公开演示运行：管理写操作被锁定，没有电子邮件、推送或外发 Webhook。不适用于真实安装。 |
