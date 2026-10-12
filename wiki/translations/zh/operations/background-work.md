---
source_sha: 935a77e1c7b524e4df14c0a67f9ad57f4571098b770e8c3245fda5189aefc0c3
reviewed: true
---
# 后台工作和服务器负载

为什么没人看的时候服务器也很忙，为什么新媒体库的缩略图要好几个小时才出来：除了播放，服务器还要做自己的工作（扫描媒体库、获取图片、生成进度条缩略图、检测片头和片尾、分析文件）。本页说明这些工作如何与观看的人共用这台机器，以及在哪里设定它们能用多少。

## 播放优先

后台任务只能使用处理器的一部分，也就是**管理 → 服务器配置 → 分配的算力**里的**全局份额**：默认 **25%**；没人使用服务器时，可以调到 **50%** 让它们快一些。在**重型任务**下，可以给其中某一项（新媒体分析、Trickplay 缩略图、片头检测、复杂度分类）单独设定份额，它会取代这项任务的全局份额；份额永远不会叠加。修改从下一个处理的条目开始生效，无需重启。

无论份额是多少，只要有人在观看，后台任务就会暂停，之后再继续。因此，如果服务器正在使用，新媒体库的缩略图和片头标记要几个小时才会到齐，而不是几分钟。

## 后台在运行什么

| 工作 | 带来什么 | 什么时候运行 |
| --- | --- | --- |
| 媒体库扫描 | 新增、修改和删除的文件 | 文件变化时、按计划、以及（如果开启）启动时。见[媒体库与元数据](../administration/libraries.md)。 |
| 下载图片 | 保存在服务器上的海报和背景图，格式为 WebP | 扫描期间进行；缺失的图片每小时重试一次。关闭时，图片由浏览器从网上加载，有些浏览器会拦截。 |
| 进度条缩略图 | 拖动进度条时显示的预览图（每 7 秒一张） | 添加片名时，以及启动时为还没有缩略图的片名生成。**服务器配置**里的**启用 Trickplay 缩略图**可以关闭它们。 |
| 片头和片尾检测 | **跳过**按钮 | 持续进行，针对社区数据库没有收录的片名 |
| 关键帧索引和编码级别 | 直接播放和不重新编码的复制、正确的色彩 | 持续进行，针对在收集这些信息之前就已扫描过的文件 |

## 内存

**内存配置**（**管理 → 服务器配置**）：**低**会挂起空闲的插件以节省内存，需要时再唤醒。**自动（推荐）**在内存少于 3 GB 的机器上会选择“低”。

## 启动时读取的设置

有几项设置只在服务器启动时读取一次：保存时会记下您的选择，重启后才生效。面板会在这些设置旁边注明。它们是：**后台工作**卡片（关键帧和编码级别处理的暂停与节奏）、**片头跳过提供商**开关，以及转码的**高级限制**。

<!-- sources
processing power, heavy tasks, pause during playback: i18n admin.serverConfig.cpuBudget.* (globalLabel, tasksTitle, tasksDesc, applyHint) ; admin.activity.bgTaskHelp.*
thumbnails switch: i18n admin.serverConfig.trickplayEnabled
artwork: i18n admin.serverConfig.artworkInfo, artworkRetryDesc
thumbnails: i18n admin.activity.bgTaskHelp.trickplay_generation
memory: i18n admin.serverConfig.memoryProfileDesc
read at startup: i18n admin.serverConfig.backgroundJobs.restartWarning, introProvidersRestart ; docs/playback/transcoding.md
-->
