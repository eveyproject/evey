---
source_sha: 987860a30ec28609fee02ff92f64e4147a49477033de9aa0ea2a0f1ecdc3ef0f
reviewed: true
---
# 反向代理配置 { #reverse-proxy-configurations }

Evey 在端口 **9091** 上用普通 HTTP 同时提供 API 和网页界面；反向代理只需要终止 TLS，然后把所有请求转发到 `evey:9091`。请在下面选择您的代理（自带的 Caddy、Nginx、Traefik、Nginx Proxy Manager，或者您已经在运行的代理）。如果您要调整流媒体传输，或者自己设置安全头，请再读一读最后两节。

## 在 Evey 里登记代理 { #tell-evey-about-the-proxy }

无论用哪种代理，Evey 都需要知道它的公网地址，以及该信任哪个代理。这两项都在**管理 → 远程访问**里设置：一个 `https://` 开头的**外部 URL**；如果代理是一个容器，再把它的网络填进**受信任的代理**。两项都无需重启就会生效。正是 `https://` 地址告诉 Evey 前面有代理在终止 TLS：Cookie 会带上 `Secure` 标记，会发送 HSTS，并且会相信受信任代理发来的 `X-Forwarded-Proto`。远程访问开关和客户端真实地址的说明见[远程访问和 HTTPS](administration/remote-access.md)。

如果您的整套安装都用代码描述，同样的设置也可以来自环境变量，写在 compose 文件的 `environment:` 下（两边都设置时，以管理面板为准）：

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12   # 只有代理是容器时才需要；环回地址始终受信任
```

以前的 `EVEY_FORWARDED_PROTO` 仍然可以用来强制指定，但已经弃用：地址里的协议就足够了。

每个自带的示例都从 `EVEY_DOMAIN` 读取域名，您复制的文件里放的是一个占位值：请就在那里把它换成您的域名，写在代理服务（Caddy、Nginx）的 `EVEY_DOMAIN:` 那一行，或者写在 `Host(...)` 规则里（Traefik）。

## Caddy（自带） { #caddy-bundled }

### 文件 { #files }

[`docker/Caddyfile`](https://github.com/eveyproject/evey/blob/master/docker/Caddyfile)，由 [`docker/Dockerfile.caddy`](https://github.com/eveyproject/evey/blob/master/docker/Dockerfile.caddy) 构建成镜像；对应仓库 [`docker-compose.yml`](https://github.com/eveyproject/evey/blob/master/docker-compose.yml) 里的 `caddy` 服务。

### 设置 { #set-it-up }

1. 在 `caddy` 服务的 `EVEY_DOMAIN:` 那一行写上您的域名，并确保主机的 80 和 443 端口能到达 Caddy 容器。
2. 把仓库 compose 文件里的 `caddy` 服务加到您的 compose 文件里，然后运行 `docker compose up -d`。Caddy 会自己为 `EVEY_DOMAIN` 申请 Let's Encrypt 证书。
3. 在**管理 → 远程访问**里，设置 `https://` 开头的**外部 URL**，并在**受信任的代理**里填入 `172.16.0.0/12`。

### 它已经处理好的事情 { #what-it-already-handles }

把所有请求代理到 `evey:9091`，流媒体路径不缓冲、不超时，数据即时转发。它设置好[安全一节](#security-headers--csp---read-this-if-your-proxy-sets-its-own)中的安全头和标准 CSP，并去掉应用自己的 CSP，确保到达浏览器的恰好只有一个；两个自带策略的页面会保留各自的策略。

## Nginx { #nginx }

### 文件 { #files }

[`examples/nginx/evey.conf.template`](https://github.com/eveyproject/evey/blob/master/examples/nginx/evey.conf.template)，以及一个现成的 compose 叠加文件 [`examples/nginx/docker-compose.nginx.yml`](https://github.com/eveyproject/evey/blob/master/examples/nginx/docker-compose.nginx.yml)。

### 设置 { #set-it-up }

1. 在叠加文件的 `EVEY_DOMAIN:` 那一行写上您的域名。nginx 镜像在启动时会根据模板生成配置，并替换其中的 `EVEY_DOMAIN`。
2. 把证书和密钥放到模板期望的位置，用对应的文件名（`nginx-certs` 卷里的 `/etc/ssl/certs/<domain>.pem` 和 `/etc/ssl/private/<domain>.key`）：这个叠加文件不会替您申请证书。
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`。
4. 在**管理 → 远程访问**里，设置 `https://` 开头的**外部 URL**，并在**受信任的代理**里填入 `172.16.0.0/12`。

### 它已经处理好的事情 { #what-it-already-handles }

代理到 `evey:9091`，在流媒体路径上关闭缓冲和超时，升级 SyncPlay 的 WebSocket，转发 `X-Forwarded-For` 和 `X-Forwarded-Proto`，并设置 `client_max_body_size 2G`，用于上传插件和恢复备份。它会原样传递应用自己的安全头和 CSP。

## Traefik { #traefik }

### 文件 { #files }

[`examples/traefik/docker-compose.traefik.yml`](https://github.com/eveyproject/evey/blob/master/examples/traefik/docker-compose.traefik.yml)。

### 设置 { #set-it-up }

1. 运行 Traefik，并配置一个 `websecure` 入口点、一个 `letsencrypt` 证书解析器，以及一个名为 `proxy` 的外部 Docker 网络；叠加文件里的标签默认使用这些名字。
2. 在叠加文件的 `Host(...)` 规则里写上您的域名，然后运行：

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. 在**管理 → 远程访问**里，设置 `https://` 开头的**外部 URL**，并在**受信任的代理**里填入 `proxy` 网络的地址范围（`172.16.0.0/12` 覆盖 Docker 的默认网络）。

### 它已经处理好的事情 { #what-it-already-handles }

叠加文件只添加路由标签，并加入 `proxy` 网络。Traefik 在默认设置下既不缓冲响应，也不限制响应的时长，所以不需要为流媒体做任何调整；它会自己转发 `X-Forwarded-For` 和 `X-Forwarded-Proto`，并原样传递应用的安全头。

## Nginx Proxy Manager { #nginx-proxy-manager }

### 文件 { #files }

没有：一切都在 NPM 的界面里设置。evey 容器必须在 NPM 使用的网络上，见[您已经在运行的代理](#a-proxy-you-already-run-tls-terminated-upstream)。

### 设置 { #set-it-up }

在 **Edit Proxy Host** 里：

1. **Details**：Scheme 填 `http`，Forward Hostname 填 `evey-server`，Forward Port 填 `9091`。**Websockets Support：开启**（SyncPlay 需要）。**Cache Assets：关闭**，否则它会以固定的过期时间缓存媒体片段和播放列表。Block Common Exploits 可以保持开启（如果搜索或插件上传在没有明显原因的情况下返回 403，就把它关掉）。
2. **SSL**：您的证书、Force SSL、HTTP/2。HSTS 在这里可选，因为应用已经会发送它。
3. 在**管理 → 远程访问**里，设置 `https://` 开头的**外部 URL**，并在**受信任的代理**里填入 NPM 网络的地址范围。

### 它已经处理好的事情 { #what-it-already-handles }

NPM 会自己转发 `X-Forwarded-For`、`X-Forwarded-Proto` 和 Host 头，它的默认超时也永远不会在 Evey 的流上触发。不需要其他设置。

**可选**：不让 NPM 用自己的磁盘来缓冲直接播放的文件。在 **Advanced** → *Custom Nginx Configuration* 里设置（`Custom Locations` 标签页无法写正则表达式）：

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

# 媒体库扫描在整个扫描期间持续发送进度。
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

其他所有路径（网页界面、API 的其余部分、SyncPlay 的 WebSocket）都经过 NPM 自己的代理块，也就是 Details 标签页配置的那部分。不加这段配置也不会出问题，它只是改变大文件在哪里缓冲。

## 您已经在运行的代理（TLS 在上游终止） { #a-proxy-you-already-run-tls-terminated-upstream }

如果 NAS 上已经有反向代理（厂商自带的、Nginx Proxy Manager，或者共享 Docker 网络上的 Traefik），就不需要 Caddy：Evey 会自己发出安全头和完整的 CSP，而流媒体规则无论如何也只能由直接面向客户端的代理来执行。在它后面再加一个代理没有任何好处。

### 设置 { #set-it-up }

把 evey 服务放到那个代理的网络上，目标设为 `evey-server:9091`（容器名；服务名 `evey` 也能解析）：

```yaml title="docker-compose.yml"
services:
  evey:
    # 不暴露端口：代理是唯一的入口
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # 知道代理的子网后，改成更窄的范围
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

然后，在那个代理里，为通往 `evey-server:9091` 的路由：转发 `X-Forwarded-For` 和 `X-Forwarded-Proto`，开启 WebSocket 支持，并在流媒体路径上应用下面的流媒体规则。上面 Nginx 和 Nginx Proxy Manager 两节给出了具体的配置行。

### 它已经处理好的事情 { #what-it-already-handles }

这取决于代理本身：请检查到达浏览器的 CSP 头恰好只有一个（见后面几节），并在设置好受信任的代理后运行一次 `evey admin doctor`。

## 流媒体：代理的默认设置会对它产生什么影响 { #streaming---what-a-proxys-defaults-do-to-it }

即使反向代理保持默认设置，Evey 也能正常工作：每个长时间的响应要么连续发送数据（HLS 片段、直接播放、转码输出），要么保持连接存活（事件流会发送 keep-alive），所以默认的读取超时永远不会触发。不过还有两点值得了解：

- **缓冲。** 会缓冲响应的代理（nginx 默认如此）会先把直接播放的文件读进自己的缓冲区和临时文件，客户端才能收到。这样能用，但一部几 GB 的电影就会经过代理主机的磁盘。在流媒体路径上设置 `proxy_buffering off` 可以避免这种情况。
- **扫描。** `/api/v1/libraries/*/scan` 在整个扫描期间持续发送进度；如果代理限制响应的总时长（这种情况很少见，nginx、Caddy 和 Traefik 默认都不限制），就会把它提前切断。不过扫描本身仍然会在服务器端继续进行。

涉及的路径：

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

自带的 Caddy 和 Nginx 示例都在这些路径上做了这两项设置（不缓冲、不超时）。Traefik 示例和 Nginx Proxy Manager 这类通过界面配置的代理保持默认设置，这样也没有问题；上面 NPM 的那段配置是可选的。

## 安全头和 CSP：如果您的代理自己设置安全头，请读这一节 { #security-headers--csp---read-this-if-your-proxy-sets-its-own }

Evey 会发出完整的 Content-Security-Policy。如果您的代理原样传递它，就不用再做什么了。

但很多配置会把它去掉（Caddy 里的 `header_down -Content-Security-Policy`），再自行编写一份策略。这样做没问题，但**您的**策略必须保留下面这些指令值，否则一些功能会在没有任何提示的情况下失效：

| 指令值 | 指令 | 缺少时会出问题的地方 |
| --- | --- | --- |
| `'self'` | `frame-src` | 所有插件界面和主题脚本（iframe 显示为空白）。它们运行在 `/plugin-frame.html` 里，这是应用用框架嵌入的一个 Evey 页面（见下文）。 |
| `'wasm-unsafe-eval'` | `script-src` | 浏览器里的 AC-3/E-AC-3 播放（ac3go WebAssembly 解码器无法编译，所以电影播放时没有声音）。这是一个范围很窄的指令值，只允许编译 Wasm，不允许一般的 `eval()`。 |
| `https://www.gstatic.com` | `script-src`（以及 `frame-src`） | Google Cast 发送端 SDK 无法加载，所以所有人都看不到投屏按钮。这是浏览器发现用户自己局域网里 Chromecast 的唯一途径。 |
| `blob:` | `media-src` | MSE 播放（hls.js 通过 blob URL 挂载视频流）。 |
| `blob:` | `worker-src` | hls.js 会在没有提示的情况下改为在主线程里解复用（在手机上更卡顿）。 |

对于自行编写策略的代理，这是一份确认可用的策略，也就是自带的 Caddy 发送的那一份：

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` 和 `object-src` 不会退回到 `default-src`，所以省略它们的策略对 `<base href>` 注入和 `<object>`/`<embed>` 嵌入毫无限制。如果您自行编写策略，请保留它们。`script-src` 里没有 `'unsafe-inline'`：注入到页面里的脚本不会运行。请不要把它加回去，应用里没有任何地方需要它。

有两个页面带有自己的策略，由应用发送；请在这两个路径上保留应用的头：

- `/plugin-frame.html`：插件界面和主题脚本运行的页面。它的策略把它放进沙箱（即使在 iframe 以外打开也是如此），把它的网络访问限制在插件清单声明的主机内，并且只允许应用用框架嵌入它，所以它还会发送 `X-Frame-Options: SAMEORIGIN`：不要在这个路径上用 `DENY` 覆盖它，否则所有插件界面都会显示为空白。
- `/cast-receiver.html`：Chromecast 从 Evey 播放时加载的页面。Cast 播放器会往里面注入来自 `www.gstatic.com` 的样式、字体和图片。请在这个路径上保留应用的头，或者只对这个路径应用下面的策略：

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "到达浏览器的 CSP 头必须恰好只有一个"
    如果有两个（应用的和您代理的，或者 Caddy 的和 Nginx Proxy Manager“Advanced”里那段配置的），浏览器会执行两者的*交集*：最严格的规则生效，而且出错时没有任何提示。请用 `curl -sI https://your.domain | grep -ci content-security-policy` 检查，它必须输出 `1`。

每次修改后，还要再做两项检查：

- **策略要写在一行里。** 在代理配置里把 CSP 拆成多行，是一种典型的、不会报错的损坏：拼接时会丢失连接处的字符（有一次 `...gstatic.com; connect-src` 变成了 `...gstaticonnect-src`，结果 Cast SDK 被禁用，两条指令也被合并了）。
- **插件框架保留它自己的策略：** `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame` 必须显示 `sandbox allow-scripts` 和 `SAMEORIGIN`。

## 客户端真实 IP 和远程访问控制 { #real-client-ips-and-the-remote-access-gate }

代理一旦转发客户端的真实地址，Evey 就能分辨观众是在家里还是在互联网上：**最大远程码率**和远程访问开关正是依据这一点起作用。有两条规则：

- 受信任的代理（管理面板或 `EVEY_TRUSTED_PROXIES`）必须包含这个代理：如果代理是 Docker 网络上的容器，填 `172.16.0.0/12`；如果它运行在主机上，就什么都不用加，因为环回地址始终受信任，您设置的内容会加在它之上。
- 多个代理串联时，每一个都必须保留 `X-Forwarded-For`。在 Caddy 里是 `servers { trusted_proxies static private_ranges }`；nginx 用 `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`。

开启这项功能会改变谁算作远程：一台远程访问原本关闭的服务器，会开始拒绝真正的远程客户端（`403 {"code":"remote_access_disabled"}`），尽管应用的设置一项都没改。设置了公网地址、而您又没有做过选择时，Evey 会自动开启远程访问；它在每次启动时记录当前状态，每次拒绝时记录客户端地址；`evey admin doctor` 会对“已发布但在拒绝访问”的情况报告失败。整个机制的说明见[远程访问和 HTTPS](administration/remote-access.md#the-real-client-address)。

<!-- sources
one port for API and UI, https mode from the scheme: internal/config/config.go (EVEY_FORWARDED_PROTO deprecated) ; internal/config/network_live.go - EffectiveHTTPS
bundled Caddy: docker/Caddyfile (streaming matcher, @ownPolicy, header_down CSP) ; docker/Dockerfile.caddy ; docker-compose.yml caddy service
nginx: examples/nginx/evey.conf.template ; examples/nginx/docker-compose.nginx.yml (nginx-certs volume, no ACME)
traefik: examples/traefik/docker-compose.traefik.yml (labels + proxy network only)
streaming paths: cmd/evey/gateway.go ; event streams keep-alive ; scan progress stream
csp and own-policy pages: internal/adapters/driving/http/helpers.go - securityHeadersMiddleware ; /plugin-frame.html ; /cast-receiver.html
trusted proxies, catch-all refused, doctor: internal/config/trusted_proxies.go ; cmd/evey/admin_ops.go - checkTrustedProxies
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
labels: i18n admin.remoteAccess.*
-->
