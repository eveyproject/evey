---
source_sha: af82a75f9598046f40d3b3b9257d529cb681628d24fcf9fab49c54919727e7fd
---

# 反向代理

Evey 在 **9091** 端口上同时提供 **API 和网页界面**（普通 HTTP）。反向代理只需终结 TLS，
再把所有请求转发到 `evey:9091` 即可 - **不存在需要另行构建或托管的前端**。

无论用哪种代理，都要设置以下两项，好让 Evey 知道自己的公开地址，并信任代理转发过来的请求头：

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # 仅当代理本身是容器时才需要；环回地址始终受信任
```

基础地址写成 `https://` 就等于告诉 Evey：前面有一层代理在终结 TLS。于是 Cookie 会带上
`Secure` 标记，服务器会发送 HSTS，来自可信代理的 `X-Forwarded-Proto` 也会被采纳。
不需要再开别的开关（旧的 `EVEY_FORWARDED_PROTO` 仍可用作覆盖值，但已废弃）。

## 流媒体 - 代理的默认设置会带来什么影响

保持默认配置的反向代理就能让 Evey 正常工作：所有长连接响应要么持续输出字节
（HLS 分片、直接播放、转码输出），要么每 30 秒向客户端发一次心跳（事件流），
因此默认的读取超时永远不会触发。不过有两点仍值得了解：

- **缓冲。** 会缓冲响应的代理（nginx 默认如此）在把直接播放的文件交给客户端之前，
  会先读进自己的缓冲区和临时文件 - 功能上没问题，但一部好几 GB 的影片就要绕经代理
  主机的磁盘。在流媒体路径上设置 `proxy_buffering off` 可以避免这一点。
- **扫描。** `/api/v1/libraries/*/scan` 会在扫描进行期间持续输出进度；如果代理限制了
  单个响应的总时长（这种情况少见 - nginx 和 Caddy 默认都不限制），进度流就会被截断。
  无论如何，扫描本身在服务端都会继续执行。

涉及的路径：

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

内置的 Caddy 配置，以及下文的 Nginx/Traefik 示例，都已经在这些路径上做好了两项设置
（关闭缓冲、取消超时）。像 Nginx Proxy Manager 这类图形界面代理，默认值就够用，
上述调优只是可选项。

## 安全响应头与 CSP - 如果代理自己会设置这些，请务必读完本节

Evey 会输出一份完整的 Content-Security-Policy。只要代理原样透传，就无需再做什么。

> 在 2026-07-23 之前，上面这句话并不成立：应用的策略缺少 `media-src` 和 `worker-src`，
> 于是 `blob:` 回落到 `default-src 'self'`，凡是原样透传该策略的部署，MSE 播放都会失败。
> 这个问题一直没被发现，是因为内置的 Caddy 配置会剥掉应用的响应头，改用自己精选的一份。
> 如果你运行的版本早于该日期，又把它放在 Nginx Proxy Manager 或 Traefik 之后，
> 而播放器始终无法起播，原因就在这里 - 要么在代理层按下文精选策略，要么升级版本。

也有不少部署选择把它剥掉（Caddy 里写 `header_down -Content-Security-Policy`），
改用自己精选的策略 - 这没问题，但**你的**策略必须保留下列取值，否则相关功能会悄无声息地失效：

| 取值 | 指令 | 缺失后失效的功能 |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | 所有扩展界面（iframe 一片空白）。扩展包会用用户令牌取回，再内联进一个 `srcdoc` iframe，而 srcdoc 文档继承嵌入方的策略。与此同时，真正约束扩展的是该 iframe 的 `sandbox="allow-scripts"`（不透明源，无法访问父页面的 DOM 和 Cookie）。 |
| `'wasm-unsafe-eval'` | `script-src` | 浏览器端的 AC-3/E-AC-3 播放（ac3go 的 WebAssembly 解码器无法编译 → 影片播放时没有声音）。这个取值范围很窄，只允许编译 Wasm，并**不**开放通用的 `eval()`。 |
| `https://www.gstatic.com` | `script-src`（以及 `frame-src`） | Google Cast 发送端 SDK 永远加载不出来 → 所有人都看不到投屏按钮。浏览器只能靠它发现用户局域网内的 Chromecast 设备。 |
| `blob:` | `media-src` | MSE 播放（hls.js 通过 blob URL 挂载媒体流）。 |
| `blob:` | `worker-src` | hls.js 会悄悄退回到主线程解复用（在手机上更容易卡顿）。 |

给自行精选策略的代理参考的一份可用配置：

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` 和 `object-src` 不会回落到 `default-src`，所以策略里漏掉它们，就等于放任
`<base href>` 注入以及 `<object>`/`<embed>` 嵌入。自行精选策略时请把这两项留着。

几条经验：

- **到达浏览器的 CSP 响应头必须有且只有一个。** 一旦出现两个（比如应用的和你代理的，
  或者 Caddy 的和 Nginx Proxy Manager 那段 “Advanced” 配置的），浏览器执行的是它们的
  *交集* - 最严格的那份胜出，而且失败时毫无提示。用
  `curl -sI https://your.domain | grep -ci content-security-policy` 检查一下，
  结果必须是 `1`。
- **策略要写在同一行里。** 在代理配置中把一条 CSP 拆成多行，是一种典型的静默损坏：
  拼接时会吞掉接缝处的字符（曾经 `...gstatic.com; connect-src` 变成了
  `...gstaticonnect-src`，既废掉了 Cast SDK，又把两条指令粘成了一条）。
- `script-src` 里的 `'unsafe-inline'` 确实是一笔实打实的代价，而目前它又不可或缺
  （扩展界面要用，见上表）。只有在完全不运行扩展前端的部署里，去掉它才是安全的。

所有内置示例都从**同一个地方**读取域名：`.env` 里的 `EVEY_DOMAIN`。设置一次，
Caddy、Nginx 和 Traefik 都会读到。

```env
EVEY_DOMAIN=media.example.com
```

## Caddy（内置）

仓库自带 [`docker/Caddyfile`](../docker/Caddyfile)（由
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) 构建成镜像）：它会为 `EVEY_DOMAIN`
自动申请 HTTPS，并把所有请求代理到 `evey:9091`，流媒体相关的例外规则也已就位。
它还会按上一节的内容精选出规范的安全响应头和 CSP（同时剥掉应用自带的 CSP，
确保只有一份到达浏览器）。把 `caddy` 服务加进你的 compose 即可，参见仓库
[`docker-compose.yml`](../docker-compose.yml) 中的 `caddy` 服务。

### 放在你已有的代理之后（TLS 由上游终结）

如果 NAS 上已经有一层反向代理（厂商自带的、Nginx Proxy Manager，或者共享 Docker
网络上的一个 Traefik），那就**不需要 Caddy**：Evey 自己会输出安全响应头和完整的 CSP，
而流媒体相关的规则本来也只有直面客户端的那层代理才能生效 - 在它后面再套一层代理毫无意义。
把 evey 服务接入那层代理所在的网络，并让它指向 `evey-server:9091`：

```yaml
services:
  evey:
    # 不映射端口：代理是唯一的入口
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # 摸清代理所在的网段后再收窄
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

接着，在那层代理里为指向 `evey-server:9091` 的路由做三件事：转发 `X-Forwarded-For`
和 `X-Forwarded-Proto`、启用 WebSocket 支持、套用上文的流媒体规则（在流媒体路径上关闭
缓冲、取消读写超时）- 下面的 Nginx、Traefik 和 Nginx Proxy Manager 各节给出了具体的配置行。

## Nginx

使用 [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
（现成的 compose 文件见
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)）。
nginx 镜像会在启动时把其中的 `EVEY_DOMAIN` 替换掉并渲染出配置 - 证书文件必须以对应的
文件名存在。该配置把请求代理到 `evey:9091`，并关闭了流媒体的缓冲 - 没有任何前端构建步骤。

## Traefik

参见 [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml)：

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

evey 容器必须与 NPM 处于同一网络（见上文“放在你已有的代理之后”）。然后打开
**Edit Proxy Host**：

1. **Details**：Scheme 选 `http`，Forward Hostname 填 `evey-server`，Forward Port
   填 `9091`。**Websockets Support 开启**（SyncPlay 需要）。**Cache Assets 关闭** -
   否则它会用固定的过期时间缓存媒体分片和播放列表。Block Common Exploits 可以保持开启
   （如果搜索或扩展上传莫名其妙返回 403，再把它关掉）。
2. **SSL**：填入你的证书，勾选 Force SSL 和 HTTP/2。HSTS 在这里是可选的 - 应用本身已经会发送。
到此为止：NPM 会自行转发 `X-Forwarded-For`、`X-Forwarded-Proto` 和 Host 请求头，
而它的默认超时对 Evey 的各类流都不会触发（见上文的流媒体一节）。

**可选** - 避免 NPM 把直接播放的文件缓冲到自己的磁盘上：
**Advanced** → *Custom Nginx Configuration*（`Custom Locations` 标签页写不了正则）：

```nginx
# 扩展上传、备份恢复。
client_max_body_size 100M;

# 流媒体 - 不缓冲，不超时。
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

# 媒体库扫描会在整个扫描期间持续输出进度。
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

其余所有路径（网页界面、API 的其他部分、SyncPlay 的 WebSocket）都走 NPM 自己的代理块，
也就是 Details 标签页配置的那一份。不加这段配置也不会有任何东西出问题 -
它只改变大文件在哪里被缓冲。

## 真实客户端 IP 与远程访问开关

一旦代理开始转发真实的客户端地址，Evey 就能区分在家观看的用户和来自互联网的用户。
广域网码率上限和远程访问开关正是依据这一点工作的，所以这两项设置只有配合正确才会表现正常：

- `EVEY_TRUSTED_PROXIES` 必须覆盖到代理（代理是 Docker 网络里的容器时填 `172.16.0.0/12`；
  代理跑在宿主机上则无需添加，环回地址始终受信任）。你填的值是**追加**到环回地址这条基线上的 -
  它永远不会取而代之，因此填了 Docker 网段也不会悄悄让开关看不到真实客户端地址。
- 多层串联的代理，每一层都要保留 `X-Forwarded-For`。在 Caddy 里对应
  `servers { trusted_proxies static private_ranges }`；nginx 里则是
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`。

**打开这项功能会改变谁算是远程用户。** 在此之前，所有请求看上去都来自代理自己的私网地址，
于是所有人都被判定为局域网用户。在此之后，真正的远程客户端会被正确识别出来 -
而一旦远程访问处于关闭状态，他们在每一条路由上都会被拒绝
（`403 {"code":"remote_access_disabled"}`）。正是这一连串变化，曾让一整户人家连续两天
用不了一台本来好好的服务器：改的是代理配置，应用设置一个字都没动，
而唯一能看到的症状就是客户端连不上，服务端却没有任何报错。

Evey 现在对此做了防范：只要配置了公开 URL，远程访问就默认开启，其状态会在每次启动时写入日志，
拒绝请求时会连同客户端 IP 一起记录，`evey admin doctor` 遇到“已发布却仍在拒绝”会直接报错，
管理面板也会给出警告。如果你改动了代理信任相关的设置，事后请跑一次 `evey admin doctor` 确认。
