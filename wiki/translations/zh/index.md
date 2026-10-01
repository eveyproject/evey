---
source_sha: 9e2d7867cc55d0c1e56c8b94a38f02164edd7cfdafed58c39880904efaa4e00c
reviewed: false
hide:
  - navigation
  - toc
---

# Evey 文档

Evey 是一个自托管的电影、剧集、音乐和照片服务器。每次播放时，它都会比较文件、播放器和服务器，只要播放器能用就保留原始文件，只重新编码必须编码的部分。大多数安装除了媒体库和用户之外什么都不用设置：硬件、容量和播放路线都由服务器自己检测和测量。

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![首页](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>首页：先是继续播放，然后是各个媒体库。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![一部电影的页面](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>一部电影的页面，包括海报、评分和演职人员。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![网页播放器](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>网页播放器。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![两部电影播放时的管理仪表盘](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>两个人观看时的管理仪表盘：每个人看到的画面，以及它是如何传送的（界面为英文）。</figcaption>
</figure>

</div>

## 少即是多

<div class="evey-points" markdown>

<div markdown>

**先走最便宜的路线**

先是原样的文件，然后是不经编码器的重新封装，最后只重新编码需要的轨道。处于直接播放或重封装的观众，对服务器的开销几乎只是读取一个文件，所以一台普通的机器也能同时服务很多人。
[Evey 如何选择播放方式](playback/how-playback-is-decided.md)

</div>

<div markdown>

**会自我测量的服务器**

它在处理器和每张显卡上对真实的 4K 编码计时，并且只把显卡用在它证明过的能力上。无需估算容量。
[转码与服务器容量](playback/transcoding.md)

</div>

<div markdown>

**满了就拒绝，而不是变慢**

不会超过测得的编码数量；满载时，播放器会被告知服务器正忙，而不是让每个流都变差。播放期间后台任务会暂停。
[后台任务与服务器负载](operations/background-work.md)

</div>

<div markdown>

**一个插件，三种屏幕**

同一个插件包可以在网页、Android 和 Android TV 上运行，并且知道自己落在哪种屏幕上。
[插件 API 参考](plugins/api-reference.md#on-a-television)

</div>

<div markdown>

**插件被隔离，而不是被信任**

安装前分析代码，只能访问自己的目录，危险的系统调用被拒绝，没有直接网络访问，令牌默认拒绝。
[安全模型与插件沙箱](SECURITY.md)

</div>

<div markdown>

**流信息显示的就是实际运行的**

每次播放只有一个决定，流信息从编码器所执行的计划中读取它，并给出文件没有原样播放的原因。
[Evey 如何选择播放方式](playback/how-playback-is-decided.md#reading-stream-info)

</div>

</div>

## 从这里开始

<div class="grid cards" markdown>

-   **[安装 Evey](installation.md)**

    一个容器，一个 compose 文件。需求与首次启动。

-   **[首次设置](getting-started/first-setup.md)**

    设置向导、你的媒体库、会使用服务器的人。

-   **[管理](administration/libraries.md)**

    媒体库、用户与个人资料、远程访问、硬件加速、备份。

-   **[Evey 如何选择播放方式](playback/how-playback-is-decided.md)**

    直接播放、重封装和转码，以及如何阅读流信息。

-   **[故障排除](operations/troubleshooting.md)**

    出问题时该看哪里：症状、日志、恢复。

-   **[插件与 API](plugins.md)**

    安装和编写插件与主题，服务器 API。

</div>

<p class="evey-meta">
Evey 处于测试阶段，还没有带编号的版本：本文档跟随开发分支。 ·
<a href="https://github.com/Evey-project/evey">GitHub 上的源代码</a> ·
<a href="https://demo.eveyproject.org">公开演示</a>（使用 demo / demo 登录） ·
AGPL-3.0-or-later ·
安全问题报告：security@eveyproject.org
</p>
