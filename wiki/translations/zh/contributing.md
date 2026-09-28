---
source_sha: caf11b9849876eb65c290e2bc0934d093760aa2b38f6e3c27b303e9af47472f8
---
# 参与贡献

Evey 是公开开发的，为参与代码工作的人编写的文档和代码一起放在 GitHub 上，而不是这个网站上。
本页为你指路。

## 从哪里开始

| 你想要 | 请阅读 |
| --- | --- |
| 从源码构建并运行 Evey | [DEV.md](../DEV.md) |
| 了解贡献规则：分支、提交、测试、许可协议 | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| 半小时内摸清代码结构 | [代码地图](CODEBASE_MAP.md) |
| 理解分层和业务上下文 | [架构](ARCHITECTURE.md) |
| 遵循代码已有的约定 | [代码约定](code-conventions.md) |
| 参与界面开发 | [界面设计系统](frontend-design-system.md) |
| 了解某个结构性决策的由来 | [架构决策](adr/README.md) |

## 为什么要有决策记录，放在哪里

每一个结构性决策 - 六边形架构、扩展令牌的权限范围如何限定、为什么安装只需一个 compose 文件、
一个播放计划如何为所有客户端做决定 - 都有一份架构决策记录：背景、权衡过的选项、最终的选择
及其代价。它们保存在代码仓库里，紧挨着它们所解释的代码；在提议修改其中任何一项决策之前，
都应该先从这里读起。

## 提交 pull request 之前

- `make pre-push` 是唯一的检查关口。它与 CI 保持一致：这里全绿，流水线就是全绿。
- 新行为和每一个 bug 修复都需要附带测试。
- 外部贡献需要一次性接受[贡献者许可协议](../CLA.md)：Evey 采用 AGPL-3.0-or-later，同时也
  以商业许可提供。你的作品版权仍归你所有。

安全问题请不要公开报告：请写信至 `security@eveyproject.org`（参见 [SECURITY.md](../SECURITY.md)）。
