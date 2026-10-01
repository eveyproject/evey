---
source_sha: 3b28483f95cf0e01f8af366abbbe130d63e2bfdb7657c49aefe090c25a5ced6f
reviewed: false
---
# 反向代理

Evey 在 **9091** 端口上（未加密 HTTP）同时提供 **API 和网页界面**。反向代理只需终止 TLS，并把所有请求转发到 `evey:9091`：**没有需要单独构建或提供的前端**。

无论你在哪里运行代理，都要告诉 Evey 它的公网 URL 以及要信任哪个代理。最简单的地方是 **Admin → 远程访问**：一个 `https://` 的 **外部 URL**，以及当代理是容器时，在 **受信任的代理** 中填写它的网络。两者都无需重启即可生效。同样的答案也可以来自环境变量，适用于完全以代码描述的安装（两者都设置时以面板为准）：

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # 仅当代理是容器时需要；环回地址始终受信任
```

`https://` 地址就是告诉 Evey 前面有代理终止 TLS 的信号：Cookie 会带上 `Secure` 标志，会发送 HSTS，并相信来自受信任代理的 `X-Forwarded-Proto`。不需要单独的开关（旧的 `EVEY_FORWARDED_PROTO` 仍被接受作为覆盖，但已弃用）。

## 流媒体：代理的默认设置对它的影响

Evey 可以在保持默认设置的反向代理之后工作：每个长时间的响应要么持续发送字节（HLS 分段、直接播放、转码输出），要么每 30 秒向客户端发送一次心跳（事件流），所以默认的读取超时永远不会触发。不过仍有两点值得了解：

- **缓冲。** 会缓冲响应的代理（nginx 默认如此）会先把直接播放的文件读入自己的缓冲区和临时文件，再交给客户端：这能工作，但一部几 GB 的电影就会经过代理主机的磁盘。在流媒体路径上设置 `proxy_buffering off` 可以避免这一点。
- **扫描。** `/api/v1/libraries/*/scan` 会在扫描进行期间持续传送进度；限制响应总时长的代理（很少见：nginx 和 Caddy 默认都不会）会把它截断。不过扫描本身会在服务器端继续进行。

涉及的路径：

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

自带的 Caddy 以及下面的 Nginx/Traefik 示例都在这些路径上设置了这两点（不缓冲、不超时）。对于 Nginx Proxy Manager 这类通过界面管理的代理，默认设置就可以，调优只是一个可选的片段。

## 安全头与 CSP：如果你的代理设置自己的，请阅读

Evey 会发出完整的 Content-Security-Policy。如果你的代理原样传递它，就无需再做什么。

很多配置会去掉它（Caddy 中的 `header_down -Content-Security-Policy`）并使用自己的策略：这没问题，但这时**你的**策略必须保留以下这些令牌，否则某些功能会悄无声息地失效：

| 令牌 | 指令 | 缺少时失效的功能 |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | 所有插件界面（空白 iframe）。插件包使用用户的令牌获取，并内联到一个 `srcdoc` iframe 中，而 srcdoc 文档会继承嵌入它的页面的策略。在此期间限制插件的是 iframe 的 `sandbox="allow-scripts"`（不透明来源，无法访问父页面的 DOM 和 Cookie）。 |
| `'wasm-unsafe-eval'` | `script-src` | 浏览器中的 AC-3/E-AC-3 播放（ac3go WebAssembly 解码器无法编译 → 电影无声播放）。这是只允许编译 Wasm 的窄令牌，并不是通用的 `eval()`。 |
| `https://www.gstatic.com` | `script-src`（+ `frame-src`） | Google Cast 发送端 SDK 永远无法加载 → 所有人都没有投放按钮。这是浏览器发现用户本地网络中 Chromecast 的唯一方式。 |
| `blob:` | `media-src` | MSE 播放（hls.js 通过 blob URL 连接流）。 |
| `blob:` | `worker-src` | hls.js 会悄悄退回在主线程中解复用（在移动设备上更卡顿）。 |

一个适用于自定义策略代理的可靠策略：

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` 和 `object-src` 不会回退到 `default-src`，因此省略它们的策略会让 `<base href>` 注入以及 `<object>`/`<embed>` 嵌入不受限制。如果你使用自己的策略，请保留它们。

有一个页面需要更宽的策略：`/cast-receiver.html`，即 Chromecast 从 Evey 播放时加载的页面。Cast 播放器会向其中注入来自 `www.gstatic.com` 的样式、字体和图片，因此应用会为这个页面发送它自己的策略。请在这条路径上保留应用的头，或者只对这条路径应用下面的策略：

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

经验法则：

- **到达浏览器的 CSP 头必须恰好只有一个。** 如果有两个（例如应用的和你的代理的，或者 Caddy 的和 Nginx Proxy Manager "Advanced" 片段的），浏览器会执行它们的*交集*：最严格的那个胜出，而且失败是无声的。用 `curl -sI https://your.domain | grep -ci content-security-policy` 检查 → 必须输出 `1`。
- **把策略保持在一行。** 在代理配置中把 CSP 拆成多行是一种典型的无声损坏：拼接时会吞掉接缝处的字符（`...gstatic.com; connect-src` 曾经变成 `...gstaticonnect-src`，这让 Cast SDK 失效并把两条指令合并成了一条）。
- `script-src` 中的 `'unsafe-inline'` 有真实的代价，而且目前不可缺少（插件界面，见上表）。只有在不运行任何插件前端的部署上，去掉它才是安全的。

所有自带的示例都从**一个地方**读取域名：你的 `.env` 中的 `EVEY_DOMAIN`。设置一次即可；Caddy、Nginx 和 Traefik 都会读取它。

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy（自带）

仓库提供了 [`docker/Caddyfile`](../docker/Caddyfile)（由 [`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) 构建成镜像）：它为 `EVEY_DOMAIN` 自动获取 HTTPS，并把所有请求代理到 `evey:9091`，流媒体的例外已经设置好。它还会设置上一节中的标准安全头和 CSP（并去掉应用自己的 CSP，使到达浏览器的恰好只有一个）。在你的 compose 中添加一个 `caddy` 服务（参考仓库 [`docker-compose.yml`](../docker-compose.yml) 中的 `caddy` 服务）。

### 在你已有的代理之后（TLS 在上游终止）

在已经有反向代理的 NAS 上（厂商自带的、Nginx Proxy Manager、共享 Docker 网络上的 Traefik），**不需要 Caddy**：Evey 会发出自己的安全头和完整的 CSP，而流媒体规则无论如何都只能由面向客户端的那个代理执行：在它后面再放一个代理没有任何好处。把 evey 服务放到那个代理的网络中，并指向 `evey-server:9091`：

```yaml title="docker-compose.yml"
services:
  evey:
    # 不设 ports：代理是唯一的入口
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # 知道代理的子网后再收窄
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

然后在那个代理中，为指向 `evey-server:9091` 的路由：转发 `X-Forwarded-For` 和 `X-Forwarded-Proto`，开启 WebSocket 支持，并应用上面的流媒体规则（在流媒体路径上不缓冲、没有读写超时）：下面的 Nginx、Traefik 和 Nginx Proxy Manager 部分给出了具体的配置行。

## Nginx

使用 [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)（现成的 compose 在 [`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)）。nginx 镜像在启动时生成它并替换 `EVEY_DOMAIN`：证书必须以对应的文件名存在。它代理到 `evey:9091`，并关闭了流媒体缓冲，没有前端构建步骤。

## Traefik

见 [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml)：

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

evey 容器必须位于 NPM 使用的网络中（见上文"在你已有的代理之后"）。然后，**Edit Proxy Host**：

1. **Details**：Scheme `http`，Forward Hostname `evey-server`，Forward Port `9091`。**Websockets Support: on**（SyncPlay）。**Cache Assets: off**：否则它会以固定过期时间缓存媒体分段和播放列表。Block Common Exploits 可以保持开启（如果搜索或插件上传无故返回 403，就关掉它）。
2. **SSL**：你的证书、Force SSL、HTTP/2。这里的 HSTS 是可选的：应用已经发送了它。
就这些：NPM 会自己转发 `X-Forwarded-For` / `X-Forwarded-Proto` 和 Host 头，它的默认超时也永远不会在 Evey 的流上触发（见上面的流媒体部分）。

**可选**：防止 NPM 通过自己的磁盘缓冲直接播放的文件：**Advanced** → *Custom Nginx Configuration*（`Custom Locations` 标签页无法表达这个正则表达式）：

```nginx
# 插件上传（最多 100 MB）和备份恢复（最多 2 GB）。
client_max_body_size 2G;

# 流媒体：不缓冲，不超时。
location ~ ^/api/v1/media/.+/(hls|stream) {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# 媒体库扫描在整个扫描期间持续传送进度。
location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

其他所有路径（网页界面、其余 API、SyncPlay WebSocket）都经过 NPM 自己的代理块，由 Details 标签页配置。跳过这个片段也不会有任何东西出问题：它只改变大文件在哪里被缓冲。

## 客户端真实地址与远程访问限制

一旦你的代理转发了客户端的真实地址，Evey 就能区分在家的观众和在互联网上的观众。WAN 码率上限和远程访问限制都依赖于此，所以这两项设置只有一起才能正确工作：

- 受信任的代理（面板或 `EVEY_TRUSTED_PROXIES`）必须覆盖该代理（当它是 Docker 网络上的容器时为 `172.16.0.0/12`；当它运行在主机上时无需添加，环回地址始终受信任）。你设置的内容会**添加**到环回地址这一基础之上，从不取代它，所以 Docker 的值不会悄悄地让限制看不到客户端的真实地址。
- 代理链中的每个代理都必须保留 `X-Forwarded-For`。在 Caddy 中是 `servers { trusted_proxies static private_ranges }`；nginx 使用 `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`。

**开启这一点会改变谁算作远程。** 在此之前，每个请求看起来都来自代理自己的私有地址，所以所有人都被归类为 LAN。在此之后，真正的远程客户端会被正确识别，如果远程访问是关闭的，它们会在所有路由上被拒绝（`403 {"code":"remote_access_disabled"}`），尽管应用的设置没有任何改变。

Evey 对此有所防护：在配置了公网 URL 且没有做出明确选择时，远程访问默认开启；状态会在每次启动时记录；拒绝会连同客户端 IP 一起记录；`evey admin doctor` 会对"已发布却在拒绝"明确报告失败；管理面板会显示警告。如果你更改了代理信任设置，之后请检查 `evey admin doctor`。
