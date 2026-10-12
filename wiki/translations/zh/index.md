---
source_sha: 8b5109c0af499483f4c9038e9c785a28ed41a147962487df2de2848bd91fe88b
reviewed: true
hide:
- navigation
- toc
---

# Evey { #evey }

Evey 是一个在您家中运行的媒体服务器。它存放您的电影、剧集、音乐和照片，家里每个人都可以在自己的设备上观看。您不用选择画质：按下播放，其余的由 Evey 处理。

[安装 Evey](installation.md){ .md-button .md-button--primary }
[试用演示](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>测试版，还没有正式版本号</span>
<span>自由软件，AGPL-3.0</span>
<span>演示站会自动登录（<code>demo</code> / <code>demo</code>）</span>
</p>

<p class="evey-platforms">
<strong>您需要：</strong>一台装了 Docker 的 x86-64 电脑或 NAS，以及一个放着您媒体文件的文件夹。安装只要一个文件和一条命令。<br>
<strong>观看方式：</strong>网页浏览器、Android 手机和平板、Android TV、电视的网页浏览器，以及 Chromecast。Android 和 Android TV 应用还在测试中，暂时没有上架 Google Play。
</p>

<div class="grid cards" markdown>

-   **您将负责运行服务器**

    在您的电脑或 NAS 上安装 Evey，添加媒体，邀请家人。
    [从安装开始](installation.md)

-   **有人给了您一个地址**

    家里已经有人在运行 Evey，您只想观看。
    [阅读“用 Evey 观看”](getting-started/watching.md)

</div>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![首页](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>首页：先是可以继续观看的内容，然后是您的各个媒体库，每个媒体库一行。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![电影详情页](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>电影详情页：海报、年龄分级、评分、简介和演职人员。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![网页播放器](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>网页播放器。控制栏里有：一起看、投屏，以及打开“流信息”的 (i) 按钮。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![两部电影播放时的管理仪表盘](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>两个人同时观看时的仪表盘：画面、设备，以及是直接播放还是重封装。</figcaption>
</figure>

</div>

## 适合全家人 { #for-everyone-at-home }

- **每人一个个人资料。** 每个个人资料都有自己的观看历史、播放进度、收藏和语言设置。一个家庭通常共用一个账户，每人用一个个人资料。
- **儿童个人资料。** 进入儿童个人资料不用输入 PIN 码。但要从它切换到成人的个人资料，就得输入 4 位数字的 PIN 码。儿童个人资料设有年龄上限，服务器绝不会发送超过这个年龄的内容，下载时也是如此。
- **在任何设备上继续观看。** 播放进度保存在服务器上，而不是手机里。
- **出门在外。** Android 应用可以把内容下载下来离线观看，有效期 30 天。
- **在电视上。** 用**快速连接**登录 Android TV：电视上显示一个代码，您在手机或电脑上确认即可。不必用遥控器输入密码。
- **一起看。** 几个人可以在同一时间观看同一部影片，每人用自己的设备。

音乐和照片的用法和电影、剧集一样，网页、Android 和电视上都能用。
[连接应用](getting-started/apps.md) ·
[用户、个人资料与家长控制](administration/users.md)

## 按下播放前，什么都不用选 { #nothing-to-choose-before-pressing-play }

有些文件在任何设备上都能播放。有些文件对旧手机来说负担太重，或者使用了电视无法读取的音频格式。每次您按下播放，服务器都会查看文件、设备和自身的负载，然后选择可行方式中开销最小的一种：

- **直接播放**：文件原样发送。
- **重封装**：画面和声音保持不变，直接复制到设备能接受的文件格式中。这对服务器几乎没有负担。Evey 有自己的工具来做这件事，叫 **MKVGo**；MKVGo 无法处理时，就用 FFmpeg。
- **转码**：服务器只转换设备无法播放的部分，常常只是声音。只有这种情况会占用处理器。

所以同一部电影，可以原样发给电视，同时为旧手机做转换。在播放器里，**流信息**会告诉您用的是这三种中的哪一种，以及原因。**管理 → 仪表盘**则会显示所有正在观看的人的情况。

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![网页播放器中的流信息](images/player-stream-info.webp){ loading=lazy }
  <figcaption>上面仪表盘里的第一个播放，从播放器里看到的样子。重封装：文件格式变了，画面和声音是直接复制的，最后一行说明了文件为什么不能原样发送（界面为英文）。</figcaption>
</figure>

[Evey 如何选择播放方式](playback/how-playback-is-decided.md) ·
[Evey 中的 MKVGo](playback/mkvgo.md)

## 一个容器，无需调校 { #one-container-nothing-to-tune }

整个安装就是一个 compose 文件。不用写 `.env`，也不用设置内存或处理器。

- **所有东西都在里面。** 数据库和搜索引擎都在容器里运行。它们的密码在第一次启动时自动生成。
- **服务器从不以 root 身份运行。** 只要能找到您媒体文件夹的所有者，它就以这个用户的身份运行，这样修复文件时才能写入。
- **服务器会自行测量能力。** 在空闲的时候，它会在处理器和您提供的显卡上实际转换一段视频并计时。这样它就知道自己能同时服务多少人。资源不够时，新的播放会用较低的画质，或者从忙碌的显卡换到处理器上。如果某个播放无法开始，提示信息会说明原因：服务器已满、还在测量中，或者文件还没准备好。
- **默认不能从家庭网络以外访问**，直到您给服务器设置一个公网地址。

您也可以使用自己的数据库或搜索引擎：
[高级部署](operations/advanced-deployments.md)。
[转码与服务器容量](playback/transcoding.md)

## Evey 如何对待您的文件和数据 { #what-evey-does-with-your-files-and-your-data }

**您的文件。** 在 Evey 里删除一部影片或一个媒体库，只是从 Evey 里移除，绝不会从硬盘上删除。Evey 只在三种情况下向您的媒体文件夹写入内容：您要求的修复、通过 API 下载的字幕，以及一个用来确认 Evey 还能发现新文件的小测试文件。如果把媒体文件夹以只读方式挂载，这些都不会发生。**媒体健康**会检查您的 MKV 和 WebM 文件，并列出发现的问题。只有您要求时，它才会修复文件。修复在副本上进行，副本完整检查通过后才替换原文件；默认保留原文件，每一处改动都有记录。无法修复的文件会被报告，不会被改动。
[媒体健康](runbooks/media-health.md)

**您的数据。** 本项目不运行任何接收您数据的服务：不需要在我们这里注册账户，没有使用统计，也没有崩溃报告。应用只和您的服务器通信，不连接我们运营的任何其他服务。会离开您网络的内容只有这些：服务器会从外部服务获取海报和简介（The Movie Database，以及音乐的封面来源），还会不时检查有没有新版本，这一项可以关闭。每次会改动数据库的更新之前，数据库都会先备份。
[隐私政策](privacy.md) · [备份与更新](backups.md)

## 插件隔离运行，权限受控 { #plugins-run-isolated-and-controlled }

插件可以添加元数据来源、通知渠道、首页栏目和播放器上的叠加内容。主题用来改变外观。安装插件之前，管理员能看到系统检查出它会做哪些事，以及它申请了哪些权限；新的权限必须明确同意才行。每个插件作为独立的进程运行，使用单独的系统用户，没有直接的网络访问；一旦超出内存、进程数或磁盘的限制，就会被停止。它的文件访问受到多严格的限制，取决于这台机器，**安全仪表板**会显示当前使用的模式。插件的小组件会显示在网页、Android 和电视上；在电视上只能看，不能操作。
[插件](plugins.md) ·
[安全模型与插件沙箱](SECURITY.md#sandbox-modes)

## 从哪里开始 { #find-your-way }

<div class="grid cards" markdown>

-   **[安装 Evey](installation.md)**

    需要准备什么、compose 文件、第一次启动、要打开的地址。

-   **[首次设置](getting-started/first-setup.md)**

    设置向导、您的媒体库、要观看的人。

-   **[连接应用](getting-started/apps.md)**

    在手机或电视上要输入的地址、登录方法，以及每条错误提示的意思。

-   **[用 Evey 观看](getting-started/watching.md)**

    写给观看的人：个人资料、查找影片、字幕、请求电影。

-   **[管理](administration/libraries.md)**

    媒体库、用户与家长控制、从家庭网络以外访问、显卡、备份。

-   **[故障排除](operations/troubleshooting.md)**

    每个问题一张表：看哪里、查什么、怎么解决。

-   **[Evey 如何选择播放方式](playback/how-playback-is-decided.md)**

    直接播放、重封装、转码，以及怎么看懂流信息。

-   **[插件与 API](plugins.md)**

    安装和编写插件与主题，服务器 API。

</div>

<p class="evey-meta">
本文档跟随开发分支更新。 ·
<a href="https://github.com/eveyproject/evey">GitHub 上的源代码</a> ·
安全问题报告：security@eveyproject.org ·
<a href="privacy/">隐私政策</a>
</p>
