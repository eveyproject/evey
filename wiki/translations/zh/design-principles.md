---
source_sha: be9fbbf14af7dccd420515b33353e89b5ac1154b0fec5dc22468319328e97a6c
reviewed: true
---
# 设计原则

代码遵循七条约束，每一条都附上体现它的页面和决策记录。想知道 Evey 为什么这样运作，请先读本页，
再去看它是怎样构建的（[架构](ARCHITECTURE.md)），或者某个结构性选择的理由（[决策记录](adr/README.md)）。
两条原则冲突时，列表里靠前的那条优先。

| 原则 | 体现在哪里 | 依据的决策 |
| --- | --- | --- |
| 1. 无法核实时，就拒绝访问 | 家长控制、插件安装、远程访问 | [ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md) |
| 2. 第三方代码受到隔离，而不是被信任 | 插件沙箱 | [ADR-0012](adr/0012-plugin-token-deny-by-default.md)，[ADR-0014](adr/0014-landlock-plugin-sandbox.md) |
| 3. 只要能做到，就传送原始文件 | 播放决定 | [ADR-0030](adr/0030-one-plan-one-decision.md) |
| 4. 决定依据已经证明的事实，而不是名称 | 投屏、硬件加速 | [ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md) |
| 5. 一次播放，一个决定 | 播放计划、流信息 | [ADR-0030](adr/0030-one-plan-one-decision.md)，[ADR-0024](adr/0024-playback-plan-resolver.md) |
| 6. 服务器按机器的能力调整自己 | 校准、后台工作 | - |
| 7. 设置项少，每项只在一个地方 | 安装、管理面板 | [ADR-0028](adr/0028-install-is-one-compose-file.md) |

## 1. 无法核实时，就拒绝访问

凡是决定是否放行的地方，一旦出错，就按最严格的结果处理。读不到家长控制设置的个人资料，什么内容都看不到；
无法分析代码的插件，不会被安装；远程访问一旦有了明确的选择，就按这个选择执行，而且当它拒绝所有人的访问时，会明确地给出提示。
远程访问和身份验证是两回事：前者是部署上的选择，后者才是保护媒体库的机制。

请看[用户、个人资料和家长控制](administration/users.md)、
[远程访问和 HTTPS](administration/remote-access.md)、
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md)。

## 2. 第三方代码受到隔离，而不是被信任

插件只得到它的清单里声明的权限，别的一概没有；安装前要经过分析；运行在没有直接网络访问的沙箱里；
它持有的令牌默认被拒绝。沙箱仍然无法阻止的情况，会明确写出来，而不是隐含不提。

请看[插件沙箱与威胁模型](SECURITY.md)、
[ADR-0012](adr/0012-plugin-token-deny-by-default.md)、
[ADR-0014](adr/0014-landlock-plugin-sandbox.md)。

## 3. 只要能做到，就传送原始文件

最好的结果是原样传送文件；其次是把同样的轨道换一个容器；再次是只转码需要转码的那些轨道。
画质不需要观众在播放前自己去选：服务器会保持源文件的画质，除非有真实的限制（设备、网络连接、某个上限）做不到。

请看[Evey 如何选择播放方式](playback/how-playback-is-decided.md)。

## 4. 决定依据已经证明的事实，而不是名称

播放器能做什么，看的是它的设备自己报告了什么，而不是查一份已知设备的名单。一块显卡用来做什么，
看的是它在测量中通过了什么，而不是它的型号理应支持什么。当某项能力无法证明时，服务器选择保守的答案：
不认识的投屏接收器，按最老的那一款的能力对待；播放器还没有报告的像素格式，按一条固定的兼容规则处理。
假设得太多，会导致黑屏或颜色错误；假设得太少，代价只是多一次转码。

请看[投屏到 Chromecast](playback/casting.md)、[硬件加速](administration/hardware-acceleration.md)、
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md)。

## 5. 一次播放，一个决定

一次播放只在开始时决定一次，这个会话里的每个清单文件和每个分段都按这个决定来提供。
播放器永远不会收到来自两种编码的分段；**流信息**显示的内容，来自播放流程所用的同一份传送计划，
并有双向的测试加以检查。

请看[播放链路](playback-serving.md)、
[ADR-0030](adr/0030-one-plan-one-decision.md)、
[ADR-0024](adr/0024-playback-plan-resolver.md)。

## 6. 服务器按机器的能力调整自己

服务器会测量自己能承担多少转码，超出的部分不再接受：性能较低的机器同时播放的转码流少一些，而不是把所有流都播得很差。
播放优先；有人在看的时候，后台工作会暂停。同一个镜像既能在 NAS 上运行，也能在装有好几块显卡的服务器上运行。

请看[转码与服务器容量](playback/transcoding.md)、
[后台工作和服务器负载](operations/background-work.md)。

## 7. 设置项少，每项只在一个地方

服务器启动前就必须具备的东西（存储、密钥、公网地址）写在 compose 文件里；之后可能更改的一切都在管理面板里，
保存在数据库中，只要没有必须重启的理由，修改后无需重启即可生效。凡是能做到的地方，都用检测和测量代替设置：
一次安装，就是一个 compose 文件，加上媒体库和用户。

请看[安装](installation.md)、[配置参考](configuration.md)、
[ADR-0028](adr/0028-install-is-one-compose-file.md)。

## 决策记录

结构性的选择都记录为架构决策记录，写明背景、权衡过的方案和带来的后果。它们和代码放在一起：[docs/adr](adr/README.md)。

<!-- sources
fail closed: internal/adapters/driving/http/parental.go - BlockedByParental ; internal/app/plugin/install.go
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
original first, one decision: internal/app/transcoding/decision.go ; internal/app/transcoding/route_tonemap_report_test.go ; docs/adr/0030-one-plan-one-decision.md
proved capabilities: frontend/packages/media-caps/src/castUrl.ts - castProfileFor ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
sizing: internal/app/transcoding/calibration_service.go ; i18n admin.serverConfig.cpuBudget.globalDesc
-->
