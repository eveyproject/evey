---
source_sha: 183d5d31f8aa84f9a8c2422bb3ca3ca04b531e43e8bfb4f1a2c6047199ff4871
reviewed: true
---
# 备份和更新 { #backups-and-updates }

两个习惯能让您的服务器保持安全：

- **备份。** 开启数据库的每日备份，并在别处保留一份 `evey-data` 文件夹的副本。
- **更新。** 一条命令就能获取新版本。每当更新要修改数据库时，Evey 都会先自动保存数据库。

本页先讲这两件事，再讲备份里有什么、如何恢复备份，以及如何退回旧版本。

## 定时备份 { #schedule-backups }

1. 打开**管理 → 数据库备份**中的**计划任务**卡片。
2. 在 **Cron 表达式**里设定什么时候运行。`0 3 * * *` 表示每天凌晨 3 点。（格式依次是：分、时、日、月、星期几。）
3. 在**保留天数**里设定每个备份保留多久，例如 `30`。每次定时运行后，更早的备份会被删除。
4. 勾选**启用**，然后点击**保存**。

**立即运行**会马上做一次备份，页面顶部的**创建备份**也一样。

<figure class="evey-shot" markdown="span">
  ![“管理 → 数据库备份”，设置了每日备份](images/admin-backup.webp){ loading=lazy }
  <figcaption>每天凌晨 3 点备份一次，保留 30 天（界面为英文）。</figcaption>
</figure>

备份保存在 `./evey-data/backups` 里。它就在您本来就应该复制到另一块磁盘上的那个文件夹里（见[备份里有什么](#what-a-backup-contains)）。

??? info "通过命令行"

    主机上的脚本或 cron 任务也可以做同样的备份：

    ```bash
    docker exec evey-server evey admin backup
    ```

    它遵循管理面板里的遥测开关（见下文）；`--include-telemetry` 只在这一次运行中包含遥测数据。

无论计划如何，服务器在更新修改数据库之前，也会自动保存数据库（见[更新 Evey](#updating-evey)）。

## 备份里有什么 { #what-a-backup-contains }

备份是**数据库**的副本：账户、个人资料、媒体库及其目录、观看记录和续播位置、合集、播放列表和收藏、设置、插件记录、Webhook、API 密钥、通知和审计日志。

它**不**包含：

- 您的媒体文件；
- 视频缓存（它会自动重新生成）；
- 搜索索引（会根据数据库重建）；
- 保存在 `./evey-data` 里的密钥：保护每次登录的 `jwt.key`，`keys` 文件夹（数据库密码、搜索密钥），以及 `hls_aes.key`（加密流媒体分段的密钥，首次启动时创建，权限 0600）；
- 已安装的插件文件。

**所以，也请保留整个 `./evey-data` 文件夹的副本。** 数据库备份可以在同一套安装上恢复 Evey。要从零重建一台机器，您需要这个文件夹。

??? info "遥测数据和文件格式"

    播放和服务器的测量数据（质量图表和负载图表背后的数据）默认不包含在备份里：它们增长最快，而且与您的片库无关。**在备份中包含遥测数据**（**管理 → 服务器配置**）会把它们包含在备份中，适用于手动、定时和命令行备份。恢复备份时，这些表总是会被清空。

    备份是一个 `.tar.gz` 文件，里面有数据库转储，以及一份简短说明，记录校验和（SHA256）和数据库结构版本。

## 恢复备份 { #restore-a-backup }

**管理 → 数据库备份**列出所有备份。可以从列表里选一个恢复，也可以用**从文件恢复**上传一个文件。恢复会替换当前的数据库：那次备份之后的所有内容都会丢失。

!!! warning "只能恢复数据库结构相同的版本所做的备份"
    服务器会先检查文件和数据库结构（结构版本）。如果备份来自数据库结构不同的版本，服务器会拒绝它，而不会加载它。要连同数据一起退回旧版本，见[退回旧版本](#going-back)。

如果服务器已经无法启动，命令行可以恢复更新前保存的安全副本（`pre-migration-*.sql.gz`），但不能恢复这个面板做的 `.tar.gz` 备份：见[恢复](runbooks/recovery.md)。

## 更新 Evey { #updating-evey }

在存放 compose 文件的文件夹里运行：

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

以上就是全部更新步骤。

- **您的数据会先被保存。** 如果新版本需要修改数据库，它会先把副本写到 `./evey-data/backups/pre-migration-*.sql.gz`（保留最近的三份），然后在启动时进行修改。
- 片库很大时，**可能需要几分钟**。容器重新开始应答后，会报告为健康状态。
- **有更新时 Evey 会告诉您。** 它每小时检查一次是否发布了更新的镜像，并在管理面板里提示。它从不自动更新自己。
- **正在运行哪个版本：** `docker exec evey-server evey version` 会显示版本号和提交（commit）。

??? info "锁定您运行的确切版本"

    `latest` 跟随最新的正式版本，每次更新都会变。对于您依赖的服务器，请在 `image:` 这一行使用版本标签（`0.1.0`，或只接收其修复的 `0.1`）或固定的 `sha-<commit>` 标签，这样相隔一天的两次拉取运行的是同样的代码。[安装](installation.md#common-variations)中的表格列出了每个标签能给您什么。

### 退回旧版本 { #going-back }

1. 把 compose 文件里 `image:` 这一行改回之前的标签。
2. 如果新版本修改过数据库，请先恢复它在更新前保存的副本：`docker exec evey-server evey admin rollback` 会列出具体步骤和文件路径。
3. 运行 `docker compose up -d`。

镜像标签、其他变体和显卡版本的说明见[高级部署](operations/advanced-deployments.md#image-tags-and-variants)。

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (sha-<short>, latest gated on vX.Y.Z)
schedule card controls (Enabled, Save, Run now): frontend/apps/web/src/routes/_auth/admin/backup.lazy.tsx ; i18n admin.backup.schedule.enabledLabel, save, cronHelp
-->
