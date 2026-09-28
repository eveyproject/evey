---
source_sha: 4ee97dc5ef2cdb11b0aa01a535284eaf511ab308d1429dbb9882b9a5838ba8ca
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

**你的电影、剧集、音乐和照片，从你自己的机器传到家里的每一块屏幕 - 还能走出家门。**

Evey 是一个自托管的媒体服务器。你把已有的文件夹交给它，它会认出里面是什么，配上海报、
简介和演职人员，然后在浏览器、Android 手机、Android TV 和 Chromecast 上播放。只要你不
发布，什么都不会离开你家；你能看什么，也不由任何别人决定。

[了解 Evey](philosophy.md){ .md-button .md-button--primary }
[安装它](installation.md){ .md-button }
[试用演示](https://demo.eveyproject.org){ .md-button }

想先随便点点看？[demo.eveyproject.org](https://demo.eveyproject.org) 上的公开演示运行的就是真正的服务器，配有
自由授权的影片：用 `demo` / `demo` 登录。它会定期重置，管理界面可以查看，但大部分是
只读的。

## 两个入口

<div class="grid cards" markdown>

-   __了解 Evey__

    ---

    Evey 想成为什么、它为观看的人带来什么，以及它能在哪些设备上运行。无需终端。

    [为什么选择 Evey](philosophy.md) · [你能得到什么](discover/features.md) ·
    [每一块屏幕](discover/apps.md)

-   __技术指南__

    ---

    安装服务器、启动后的最初十分钟、观众或管理员能改的每一项设置，以及出问题那天该怎么办。

    [安装](installation.md) · [第一步](guide/first-steps.md) ·
    [管理](guide/admin-settings.md)

</div>

## 大家为什么用它

- **它播放你手里的文件。** 设备能原样读取文件时，Evey 原封不动地发送：原画质，服务器
  不用干活。读不了时，Evey 只转换缺的那部分，有显卡就用显卡。
- **一个文件就能装好。** 一个容器，一个 compose 文件，不用自己编密码。数据库和搜索引擎
  都在容器里，并且自己完成配置。
- **它为一个家庭而设计。** 每人都有个人资料，儿童个人资料受 PIN 保护，年龄限制由服务器
  而不是应用来执行，还有一个让不同家庭一起看的房间。
- **可以放心放到互联网上。** 只有在你发布服务器时远程访问才会打开，否则保持关闭；登录在
  拿不准时直接拒绝；扩展在沙箱里运行，只拥有它们声明过的权限。
- **它是自由软件。** 采用 [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html)
  许可。代码在 [GitHub](https://github.com/Evey-project/evey) 上，服务器目前还防不住的
  东西也一并写在那里。

## 快速入口

| 你想要 | 请阅读 |
| --- | --- |
| 在自己的机器上试一试 | [安装](installation.md) |
| 用 HTTPS 把它放到互联网上 | [反向代理](reverse-proxy.md) |
| 用显卡来转码 | [转码与硬件](transcoding.md) |
| 了解每项设置的作用 | [观众设置](guide/viewer-settings.md) · [管理](guide/admin-settings.md) |
| 崩溃后把服务器救回来 | [恢复](runbooks/recovery.md) |
| 编写扩展 | [扩展](plugins.md) |

安全问题请报告至 `security@eveyproject.org` - 参见[扩展沙箱与威胁模型](SECURITY.md)。
