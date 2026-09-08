---
source_sha: aedd9ba6244ba2dbc7e348c929241546a233301b93c88350c78c524424a8728f
hide:
- navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

用于电影、剧集、音乐和照片的自托管媒体服务器。一个容器，你自己的硬件，你自己的文件。

[安装 Evey](installation.md){ .md-button .md-button--primary }
[查看源代码](https://github.com/Evey-project/evey){ .md-button }

## 从这里开始

<div class="grid cards" markdown>

-   __安装__

    ---

    只需一个 compose 文件，无需自行设定任何密钥。Postgres 和搜索索引都在容器内运行，
    并在首次启动时自行生成凭据。

    [安装指南](installation.md)

-   __配置__

    ---

    管理员能改的一切，以及它们各自的位置。其中绝大部分都在管理面板里，而不在环境变量中。

    [配置参考](configuration.md)

-   __置于反向代理之后__

    ---

    Caddy、nginx、Traefik 和 Apache，附带流媒体与 WebSocket 播放真正需要的请求头。

    [反向代理指南](reverse-proxy.md)

-   __保护你的数据__

    ---

    服务器会自动备份什么、不会备份什么，以及当数据库无法启动时如何恢复。

    [备份](backups.md) · [恢复](runbooks/recovery.md)

</div>

## 它能做什么

- **播放属于你的内容。** 只要客户端能原样打开文件就直接播放，不能时则使用硬件或
  软件转码，HLS 流还可以在播放途中切换音轨和画质。
- **在各类设备间跟随你。** 网页、Android、Android TV 和 Chromecast，保留续播位置、
  观看状态，并支持下载后离线观看。
- **为一个家庭而设计。** 每个配置文件都有自己的主题和语言，家长控制由服务器强制执行，
  还可以用 SyncPlay 一起观看。
- **无需重新构建即可扩展。** 扩展在沙箱中运行，使用受限令牌和默认拒绝的权限模型，
  主题则是数据而非代码。

## 参与贡献

[架构](ARCHITECTURE.md)页面说明服务器的组织方式，[架构决策](adr/README.md)记录了
每一项结构性选择的理由。编写扩展请从[扩展开发环境准备](plugins/dev-setup.md)开始。

Evey 以 [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html) 授权发布。
安全问题请报告至 `security@eveyproject.org` - 参见[威胁模型](SECURITY.md)。
