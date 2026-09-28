---
source_sha: f7107d811a1a25b00c55dbd3789c083144308003ef8ab0ea3e52905c56b1bc7c
---
# 版本发布手册

Evey 的一个版本如何从一个分支变成运维者可以固定使用的东西。写于第一个带标签的版本之前，
好让第一次发布遵循一套流程，而不是现场发明一套。

## CI 拿到标签会做什么（打标签前先读）

`.github/workflows/ci.yml` 在每次 push 和每个 `v*.*.*` 标签时构建并发布镜像，分三种扩展
运行时变体（`` (lean)、`-nodejs`、`-full`），发布到 `docker.io/gravityz/evey` 和
`ghcr.io/evey-project/evey`：

| 你推送的是 | 发布的镜像标签（每个还有 `-nodejs` / `-full`） |
|---|---|
| 一个分支（如 `beta-0.0.1`） | `beta-0.0.1`、`sha-<short>` |
| 标签 `v0.2.0-beta.1`（预发布：带连字符） | `0.2.0-beta.1`、`sha-<short>` |
| 标签 `v0.2.0`（稳定版） | `0.2.0`、`0.2`、`0`、`latest`、`sha-<short>` |

三个后果：

- **分支标签会移动。** `beta-0.0.1-full` 指向最后一次推送的内容；相隔一天的两次
  `docker compose pull` 可能运行不同的代码，而"回滚到 `beta-0.0.1-full`"根本回不去。运维者
  必须固定一个 `sha-<short>` 标签（或版本标签），绝不能是分支名 - 这正是
  `scripts/evey-update.sh` 最后打印出来、让他们填进 `.env` 的内容。
- **预发布永远不会动 `latest`。** `latest` 指最新的稳定版；封闭测试版的发布对跟踪
  `latest` 的人是不可见的。（预发布也不会得到 `X.Y` / `X` 标签：那只属于稳定版。）
- **每次构建都是多架构**（amd64 + arm64），所以 ARM 运维者可以固定其中任何一个。

提交号会写进二进制文件（`evey version`、`/healthz`）和镜像标签
（`org.opencontainers.image.revision`），因此"这是哪份代码？"永远有答案，不必相信某个标签。

## 发布预发布版（封闭测试）

1. **在确切的提交上检查全绿**：本地运行 `make pre-push`。push 时 CI 也会运行同样的检查。
2. **先在真实安装上验证。** 该提交必须已经以 `sha-<short>` 标签在参考部署上跑过，用
   `scripts/evey-update.sh --tag sha-<short> --apply` 更新，并用 `--commit <sha>` 验证。
   标签是对其他运维者的承诺；先验证，再承诺，而不是反过来。
3. **`CHANGELOG.md`**：把 `[Unreleased]` 下的条目连同日期移到新版本下。采用 Keep a
   Changelog 格式，只写面向用户的措辞。
4. **从那个提交打标签**，使用附注标签，并且只推送这个标签：
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **盯着 CI** 发布 `0.2.0-beta.1`（及各变体）。然后在参考部署上运行
   `scripts/evey-update.sh --tag 0.2.0-beta.1 --apply` - 固定版本标签会取代 `sha-` 固定。
6. **发布标签公告**，附上对应的 changelog 段落。运维者在 `image:` 行里固定
   `gravityz/evey:0.2.0-beta.1`（或 `-full`）。

预发布标签很便宜：需要时就 `-beta.2`、`-beta.3`。但它们无法撤回 - 已发布的标签会一直存在。
要撤销一次糟糕的发布，就发下一个；永远不要把标签改指到别处。

## 发布稳定版

步骤相同，标签用不带后缀的 `vX.Y.Z`。有一件预发布不需要检查的事：镜像内置的 Postgres
主版本必须记录在 changelog 里，因为使用内置数据库的运维者在它变化时必须按照
[installation.md](../installation.md) 的升级章节操作。

## 回滚版本

镜像一直保留在镜像仓库中。运维者用
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply` 回滚
（更新脚本会打印出这一行）。发布这一侧不需要撤销任何东西；修复完成后发布一个修正过的
`-beta.N+1` 即可。
