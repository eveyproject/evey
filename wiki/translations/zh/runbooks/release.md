---
source_sha: 7e38ef34503d1582f3a4a5fc3e6171c1d2f137aa52dae78d9840504057637151
---
# 版本发布手册

本页说明 Evey 的一个版本如何从一个分支变成运维者可以固定使用的版本。本页写于第一个带标签的版本之前，
目的是让第一次发布按照既定流程进行，而不是临时制定流程。

## CI 如何处理标签（打标签前请阅读）

`.github/workflows/ci.yml` 在每次 push 和每个 `v*.*.*` 标签时构建并发布镜像，共三种插件
运行时变体（`` (完整，默认)、`-nodejs`、`-lean`），发布到 `docker.io/eveyproject/evey` 和
`ghcr.io/eveyproject/evey`：

| 您推送的内容 | 发布的镜像标签（每个标签另有 `-lean` / `-nodejs` 版本） |
|---|---|
| 一个分支（例如 `master`） | `master`、`sha-<short>` |
| 标签 `v0.2.0-rc.1`（预发布版：带连字符） | `0.2.0-rc.1`、`sha-<short>` |
| 标签 `v0.2.0`（稳定版） | `0.2.0`、`0.2`、`0`、`latest`、`sha-<short>` |

由此得出三点：

- **分支标签会变动。** `master` 指向最后一次推送的内容；相隔一天执行的两次
  `docker compose pull` 可能运行不同的代码，而“回滚到 `master`”无法回到任何确定的版本。运维者
  必须固定一个 `sha-<short>` 标签（或版本标签），不能固定分支名 - `scripts/evey-update.sh`
  在结束时会打印该标签，供运维者写入 `.env`。
- **预发布版不会影响 `latest`。** `latest` 表示最新的稳定版；跟踪 `latest` 的人不会看到封闭测试版的发布。
  （预发布版也不会获得 `X.Y` / `X` 标签：这些标签只用于稳定版。）
- **只有标签才构建多架构镜像。** `v*.*.*` 标签（稳定版或预发布版）构建 amd64 + arm64；
  推送分支只构建 amd64，除非提交信息中包含 `[arm64]`，或者手动触发该工作流。ARM 运维者
  应固定一个版本标签。

提交号会写入二进制文件（`evey version`、`/healthz`）和镜像标签
（`org.opencontainers.image.revision`），因此无需依赖标签，也始终可以确定“运行的是哪份代码”。

## 发布预发布版（封闭测试）

1. **在该提交上所有检查通过**：在本地运行 `make pre-push`。push 时 CI 也会运行相同的检查。
2. **先在真实安装上验证。** 该提交必须已经以 `sha-<short>` 标签在参考部署上运行过，用
   `scripts/evey-update.sh --tag sha-<short> --apply` 更新，并用 `--commit <sha>` 验证。
   标签是对其他运维者的承诺；应在验证之后打标签，而不是之前。
3. **`CHANGELOG.md`**：把 `[Unreleased]` 下的条目移到新版本下，并注明日期。采用 Keep a
   Changelog 格式，只使用面向用户的表述。
4. **从该提交打标签**，使用附注标签，并且只推送该标签：
   ```bash
   git tag -a v0.2.0-rc.1 -m "v0.2.0-rc.1"
   git push origin v0.2.0-rc.1
   ```
5. **查看 CI** 是否发布了 `0.2.0-rc.1`（及各变体）。然后在参考部署上运行
   `scripts/evey-update.sh --tag 0.2.0-rc.1 --apply` - 固定版本标签后，它将取代之前固定的 `sha-` 标签。
6. **公布该标签**，并附上对应的 changelog 章节。运维者在 `image:` 行中固定
   `eveyproject/evey:0.2.0-rc.1`（或 `-lean`）。

预发布标签的成本很低：需要时可以发布 `-beta.2`、`-beta.3`。但它们无法撤回 - 已发布的标签会一直保留。
要撤销一次有问题的发布，请发布下一个版本；不要将标签重新指向其他提交。

## 发布稳定版

步骤相同，标签使用不带后缀的 `vX.Y.Z`。有一项预发布版不需要的检查：镜像内置的 Postgres
主版本必须记录在 changelog 中，因为使用内置数据库的运维者在该版本变化时必须按照
[installation.md](../installation.md) 的升级章节操作。

## 回滚版本

镜像会保留在镜像仓库中。运维者使用
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply` 回滚
（更新脚本会打印出这一行）。发布一侧不需要撤销任何操作；修复完成后，发布修正后的
`-rc.N+1` 即可。
