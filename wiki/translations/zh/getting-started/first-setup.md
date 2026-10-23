---
source_sha: 48225cad93d795d54f36c6e4db0c828700357d5e317bec3f404927fba2c1cdf4
reviewed: true
---
# 首次设置 { #first-setup }

Evey 已经在运行，您也在浏览器里打开了它的地址，比如 `http://192.168.1.20:9091`（不确定地址是什么？见[安装的第 3 步](../installation.md#the-address-to-open-next)）。

在家人可以开始观看之前，还剩四步：

1. [设置向导](#1-the-setup-wizard)：大约一分钟。
2. [添加媒体库](#2-add-your-libraries)：告诉 Evey 您的媒体放在哪里。
3. [邀请家人](#3-invite-your-household)：每人一个账户，或者大家共用一个账户、每人一个个人资料。
4. [连接应用](#4-connect-the-apps)：手机、平板和电视。

## 1. 设置向导 { #1-the-setup-wizard }

只要服务器还没有管理员，打开它的地址就总会进入向导。页面顶部显示**第 1 步，共 4 步 - 语言**，还有四个小圆点，随着设置的进行逐个填满。

### 第 1 步，共 4 步 - 语言 { #step-1-of-4---language }

卡片上写着**选择服务器语言**。请选择：

- **语言**：新个人资料的默认语言，也是 Evey 获取标题和简介时使用的语言。向导会马上切换到这种语言。
- **地区**：根据语言自动填好。它决定显示哪个国家或地区的年龄分级。也可以选**自动**。

这两项以后都可以在管理面板里修改。点击**继续**。

### 第 2 步，共 4 步 - 欢迎 { #step-2-of-4---welcome }

这一步无需填写。点击**开始设置**。

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![向导第 1 步：语言和区域](../images/setup-wizard-1-language.webp){ loading=lazy data-gallery="setup" }
  <figcaption>第 1 步：服务器的语言，以及决定年龄分级的区域（界面为英文）。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![向导第 2 步：欢迎](../images/setup-wizard-2-welcome.webp){ loading=lazy data-gallery="setup" }
  <figcaption>第 2 步：无需填写任何内容。</figcaption>
</figure>

</div>

### 第 3 步，共 4 步 - 管理员账户 { #step-3-of-4---admin-account }

卡片上写着**创建管理员账户**。这是您自己的账户，用来管理服务器。

| 栏位 | 填什么 |
| --- | --- |
| **用户名** | 至少 3 个字符。 |
| **密码** | 至少 8 个字符。 |
| **确认密码** | 再输入一遍同样的密码。 |
| **服务器名称** | 可选。比如“客厅”。大家会在登录页、浏览器标签页、应用的服务器列表和邮件里看到它。以后可以在**管理 → 服务器配置 → 服务器名称**里修改。 |

点击**继续**。Evey 会创建账户并让您登录。向导不会再出现。

### 第 4 步，共 4 步 - 第一个媒体库 { #step-4-of-4---first-library }

卡片上写着**添加您的第一个媒体库**。一个媒体库就是放在一个或多个文件夹里的同一类媒体，比如“电影”。

- **媒体库名称**：比如“电影”。
- **媒体库类型**：**电影**、**电视剧**、**音乐**或**照片**。
- **路径**：从列表里选一个文件夹。列表显示 Evey 能看到的文件夹，从 `/media` 开始。如果想直接输入路径，比如 `/media/movies`，请选择**输入自定义路径...**。

点击**创建**：Evey 开始读取这个文件夹。也可以点击**跳过**，以后再添加媒体库。两种情况都会进入首页。

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![向导第 3 步：管理员账户](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>第 3 步：管理员账户；服务器名称可以不填。</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![向导第 4 步：第一个媒体库](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>第 4 步：一个名称、一个类型，以及 Evey 所看到的文件夹路径。</figcaption>
</figure>

</div>

??? note "向导没有出现，或者出现了两次"

    一旦有了管理员，这个地址就会打开登录页，访问 `/setup` 也会跳到登录页。如果两个人同时打开向导，只有先完成的那个人能成为管理员，另一个人会被带到登录页。

<figure class="evey-shot" markdown="span">
  ![演示服务器的管理仪表盘](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>登录后的管理面板：仪表盘，以及侧边菜单里的所有栏目（界面为英文）。</figcaption>
</figure>

## 2. 添加媒体库 { #2-add-your-libraries }

进入**管理 → 媒体库 → 添加媒体库**。填写名称、类型（电影、电视剧、音乐或照片），再选一个或多个文件夹。

这里的文件夹名称是 Evey 看到的样子，不是它们在您电脑上的样子。如果用的是[安装](../installation.md#the-compose-file)页里的 compose 文件，您的媒体文件夹在 Evey 里叫 `/media`。所以您电脑上的 `/srv/media/films` 文件夹，在 Evey 里就是 `/media/films`。

Evey 会自动开始读取文件夹。海报、简介和演员表会在后台陆续获取，所以大型媒体库需要几分钟才能显示完整。以后新加的文件也会自动出现。

影片没有出现，或者海报不对？请看[媒体库与元数据](../administration/libraries.md)：文件该如何命名，以及匹配错误时如何修正。

## 3. 邀请家人 { #3-invite-your-household }

为他人创建账户有两种方法，都在**管理 → 用户**里。

**发送邀请链接**（对方自己设密码）：

1. 请用其他人会用的那个地址打开管理面板，比如 `http://192.168.1.20:9091`，而不是 `http://localhost:9091`。因为链接是根据您浏览器里的地址生成的。
2. 点击**创建邀请链接**。您可以限制链接的有效期（**过期时间（可选）**）、能用几次（**最大使用次数（留空 = 无限制）**），以及新账户能看到哪些媒体库（**可访问的媒体库**）。
3. 在**邀请链接**里点击**复制链接**，然后通过消息或邮件发给对方。
4. 对方打开链接后，会看到“**您已被邀请！请填写以下信息。**”。对方设置用户名和密码（至少 8 个字符）后，就直接登录了。

**或者您自己创建账户**：点击**创建用户**，设好**用户名**和**密码**，再告诉对方。

每个账户一开始都有一个个人资料，名字就是用户名。一家人可以共用一个账户，每人一个个人资料：在**设置 → 个人资料 → 创建个人资料**里添加。给孩子用的，请勾选**儿童个人资料**，并选择**最大年龄分级**。这样一来，这个账户的其他所有个人资料都需要设置 **PIN 码（4 位数字）**，孩子就无法切换到这些个人资料。

然后给每个人发三样东西：地址、他们的用户名和密码（或邀请链接），以及专门写给他们看的页面：[用 Evey 观看](watching.md)。

更多选项（每个账户的限制、可观看时段、PIN 码、年龄限制）：[用户、个人资料与家长控制](../administration/users.md)。

## 4. 连接应用 { #4-connect-the-apps }

- **在浏览器里**，什么都不用装：就是您已经打开的那个地址。
- **在 Android 手机、平板或电视上**，应用会先要求输入服务器地址。请输入**和浏览器里一样的地址**，带上 `http://` 和端口，比如 `http://192.168.1.20:9091`。然后登录。
- **在电视上**，可以用一个在手机上确认的代码登录，不必用遥控器输入密码。

详细步骤，以及每条错误提示的意思：[连接应用](apps.md#the-server-address-to-enter)。

## 服务器自动完成的工作 { #what-the-server-does-by-itself }

下面这些都不用您设置：

- **每台设备怎么播放文件。** 每台设备会告诉服务器自己能播什么。对每部影片，服务器会选择原样发送文件、重新封装，或者只转换设备无法播放的部分。见[Evey 如何选择播放方式](../playback/how-playback-is-decided.md)。
- **能同时服务多少人。** 在第一次空闲时，服务器会测量自己用处理器和显卡（如果有）能同时转换多少视频，并且绝不接受超过这个数量的播放。这需要几分钟。在此期间，需要转换的播放会被要求稍等。见[转码与服务器容量](../playback/transcoding.md)。
- **从家庭网络以外访问。** 默认关闭，直到您给服务器设置公网地址。见[远程访问与 HTTPS](../administration/remote-access.md)。
- **数据库的安全副本**：每次会改动数据库的更新之前都会自动备份。要定期备份，只需打开一项设置：[备份与更新](../backups.md)。

## 在家庭网络以外共享此地址之前 { #before-you-share-the-address-outside-your-home }

1. 给服务器加上 HTTPS。Evey 自己只会用普通的 HTTP，在家庭网络中没有问题，但不适合在互联网上使用。[远程访问与 HTTPS](../administration/remote-access.md)。
2. 设置定期备份，并保存一份 `evey-data` 文件夹的副本。里面有数据库，还有保护所有登录的密钥。[备份与更新](../backups.md)。
