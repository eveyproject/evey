---
source_sha: a0eb85f44612fd3d234753208bf63c8a9fec91fc8d1020bfdb8109e199c59b19
reviewed: true
---
# 参与贡献

想参与 Evey 的代码开发，该从哪里开始，以及一个 pull request 需要通过哪些检查。写给贡献者的文档和代码一起放在 GitHub 上，
而不是放在这个网站上；本页指向它们。

## 从哪里开始

| 您想要 | 请阅读 |
| --- | --- |
| 从源码构建并运行 Evey | [DEV.md](../DEV.md) |
| 了解贡献的规则：分支、提交、测试、许可协议 | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| 半小时内了解代码的结构 | [代码地图](CODEBASE_MAP.md) |
| 理解分层和各个限界上下文 | [架构](ARCHITECTURE.md) |
| 遵循代码里已有的约定 | [代码约定](code-conventions.md) |
| 开发界面 | [前端设计系统](frontend-design-system.md) |
| 了解某个结构性选择的理由 | [架构决策](adr/README.md) |
| 发布一个版本 | [版本发布手册](runbooks/release.md) |

## 为什么要有决策记录，它们在哪里

每个结构性的选择，比如六边形分层、插件令牌的权限范围怎样划定、为什么安装只需要一个 compose 文件、
一份播放计划怎样替所有客户端做决定，都有一份架构决策记录：写明背景、权衡过的方案、最后选了什么以及代价是什么。
它们保存在仓库里，就在它们所解释的代码旁边。如果您想提议改变其中某个选择，请先从这些记录读起。

## 提交 pull request 之前

1. 运行 `make pre-push`：这是必须通过的检查，它会在本地按以下顺序运行 CI 的各项检查：vet、漏洞扫描、构建、
   带 `-race` 的单元测试、前端（类型检查、lint、单元测试）、安全扫描、lint、
   [架构检查](ARCHITECTURE.md#architecture-guards)、提交规范检查、密钥泄露扫描、proto 破坏性变更检查、
   从干净检出的代码构建、仓库和转码的集成测试检查，以及电视应用审计。
2. 为新的行为和每一个缺陷修复补上测试。
3. 外部贡献需要一次性接受[贡献者许可协议](../CLA.md)：Evey 采用 AGPL-3.0-or-later 许可，
   也可能以商业许可提供。您的作品版权仍归您所有。

!!! warning "这组检查不会构建 Docker 镜像，也不会构建 Android 应用"
    修改了 `Dockerfile` 或 Kotlin 代码之后，请在推送之前自己构建它们
    （`docker build --target runtime`，以及 Gradle 的 release 任务）。

安全问题请不要公开报告：请写信到 `security@eveyproject.org`（见 [SECURITY.md](../SECURITY.md)）。

<!-- sources
gate: Makefile - pre-push, ci-fast-vet, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
