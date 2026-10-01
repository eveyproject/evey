---
source_sha: cd66a9194ddc930c849aec83b5d0a19bb86c2f982c6fcf9e5f4211e0a00e0cde
reviewed: false
---
# 备份与更新

## 备份包含什么

备份是**数据库**的副本：账户、个人资料、媒体库及其元数据、观看记录和继续播放位置、合集和播放列表、设置、插件记录、Webhook 和审计日志。它是一个 `.tar.gz`，包含 SQL 转储和一个记录转储 SHA256 的清单。

它**不**包含：

- 你的媒体文件；
- 转码缓存（需要时重新生成）；
- 搜索索引（从数据库重建）；
- 保存在 `./evey-data` 中的密钥：签署会话的 `jwt.key`，以及 `keys` 文件夹（数据库密码、搜索密钥）；
- 已安装插件的文件。

因此，数据库备份可以在同一个安装上恢复 Evey 的状态。为了能重建一台机器，还要保留整个 `./evey-data` 文件夹的副本：数据库备份就在其中，位于 `./evey-data/backups`。

## 计划备份

**Admin → 备份与恢复 → 计划任务**：一个 cron 表达式（例如 `0 3 * * *`，每天凌晨 3 点）和一个以天为单位的保留期。每次计划运行后，超过保留期的备份会被删除。**立即运行** 会马上创建一个备份。

<figure class="evey-shot" markdown="span">
  ![设置了每日计划的 Admin → 备份与恢复](images/admin-backup.webp){ loading=lazy }
  <figcaption>每天凌晨 3 点备份一次，保留 30 天（界面为英文）。</figcaption>
</figure>

无论计划如何，服务器在更新过程中应用迁移之前，也会自行转储数据库（见下文）。

## 恢复

**Admin → 备份与恢复** 列出备份并恢复其中一个，或者从你上传的文件恢复。恢复会替换当前的数据库。

备份只能在数据库架构相同的服务器上恢复：服务器会先检查校验和与架构版本，对于由架构不同的版本创建的备份，它会拒绝而不是加载。要连同数据一起回到旧版本，见下文的[回退](#going-back)。

当服务器无法再启动时，命令行恢复的是迁移前创建的转储（`pre-migration-*.sql.gz`），而不是这个面板的 `.tar.gz` 备份：见[恢复](runbooks/recovery.md)。

## 更新 Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

这就是全部的更新。当新版本需要更改数据库时，它会先把转储写入 `./evey-data/backups/pre-migration-*`，然后在启动时应用迁移；大型片库可能需要几分钟，容器在重新提供服务后会报告为健康。Evey 每 6 小时检查一次是否发布了更新的镜像，并在管理面板中提示；它从不自行更新。

**固定你运行的版本。** `latest` 跟随最新的稳定版本。对于你依赖的服务器，在 `image:` 行上写一个版本标签（`0.2.0`）或一个不可变的 `sha-<commit>` 标签，这样相隔一天的两次拉取运行的是同样的代码。像 `beta-0.0.1` 这样的分支标签会随每次更改而移动。

**正在运行哪个版本。** `docker exec evey-server evey version` 会显示正在运行的二进制文件的版本和提交。

### 回退

把之前的标签写到 `image:` 行上，然后运行 `docker compose up -d`。如果较新的版本已经迁移了数据库，请先恢复它的迁移前转储：`docker exec evey-server evey admin rollback` 会显示带有转储路径的具体步骤。

镜像标签、运行时变体和 GPU 变体在[高级部署](operations/advanced-deployments.md#image-tags)中说明。
