---
source_sha: 0d64371960f5bb56be25ad84664ab862f39caf2e438a08da552c1713a2274048
reviewed: false
---
# 参与贡献

Evey 以开放的方式开发，写给参与代码工作的人的文档与代码一起放在 GitHub 上，而不是放在这个网站上。本页指向它们。

## 从哪里开始

| 你想要 | 阅读 |
| --- | --- |
| 从源码构建并运行 Evey | [DEV.md](../DEV.md) |
| 了解贡献的规则：分支、提交、测试、许可协议 | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| 在半小时内熟悉代码 | [代码地图](CODEBASE_MAP.md) |
| 理解分层和限界上下文 | [架构](ARCHITECTURE.md) |
| 遵循代码已经采用的约定 | [代码约定](code-conventions.md) |
| 开发界面 | [前端设计系统](frontend-design-system.md) |
| 了解某个结构性选择的原因 | [架构决策](adr/README.md) |
| 发布一个版本 | [发布手册](runbooks/release.md) |

## 为什么需要决策记录，以及它们在哪里

每个结构性的选择（六边形布局、插件令牌的范围如何限定、为什么安装是单个 compose 文件、一个播放计划如何为所有客户端做决定）都有一份架构决策记录：背景、权衡过的选项、选择了什么以及代价是什么。它们保存在仓库中，紧挨着它们所解释的代码，在提议更改其中某个选择之前，应该从它们开始阅读。

## 在提交 pull request 之前

- `make pre-push` 是检查关口：它在本地运行 CI 的各项检查（vet、漏洞扫描、构建、带 `-race` 的单元测试、前端、lint、架构守卫）。它不会构建 Docker 镜像或 Android 应用：修改 `Dockerfile` 或 Kotlin 代码后，请自行构建它们。
- 新的行为和每个缺陷修复都需要测试。
- 外部贡献需要一次性接受[贡献者许可协议](../CLA.md)：Evey 采用 AGPL-3.0-or-later，同时也以商业许可提供。你保留自己作品的版权。

安全问题不要公开报告：请写信到 `security@eveyproject.org`（见 [SECURITY.md](../SECURITY.md)）。
