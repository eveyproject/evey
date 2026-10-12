---
source_sha: fad8d9ff7aa0e8e4c14508e1eb2fe716e13c05c66eca9acb763084c56aec898f
reviewed: true
---
# 数据库迁移

本页说明 Evey 在更新时如何改变数据库结构、前后要检查什么，以及迁移中途停止时如何恢复。它写给使用标准安装的运维人员，标准安装中数据库运行在 `evey-server` 容器里；编写迁移的贡献者可以在最后找到相关规则。

## 迁移如何运行

迁移内置在程序里，服务器启动时由 `golang-migrate` 应用。状态记录在 `schema_migrations` 表里，只有一行 `{version, dirty}`。在应用任何迁移之前，服务器会把完整的转储写到 `EVEY_BACKUP_DIR`（`/data/backups`，也就是主机上的 `./evey-data/backups`），文件名为 `pre-migration-<version>-<UTC timestamp>.sql.gz`；最近 3 个数据库结构版本中，每个版本保留最新的一份转储。

同样的操作也可以手动进行：

```sh
docker exec evey-server evey admin migrate version   # 数据库结构处于“脏”状态时，以退出码 1 退出
docker exec evey-server evey admin migrate up        # 应用所有待执行的迁移
docker exec evey-server evey admin migrate down      # 正好回退一步
```

`up` 和 `down` 都会先做一次转储；`--no-backup` 会跳过这一步。要在内置数据库上打开一个 SQL 命令行：

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## 安全地更新

1. **先做一次备份**：在**管理 → 数据库备份**里（**立即运行**），或者用脚本：
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   迁移前自动生成的转储是第二份副本，不能代替这次备份。

2. **查看当前版本**：`docker exec evey-server evey admin migrate version` 会输出 `schema version: N`。如果输出的是 `DIRTY`，请先阅读[迁移中途停止](#a-migration-that-stopped-halfway)，再进行更新。

3. **如果您的片库很大**，先看看最大的几张表：改写其中某张表的迁移会花更长时间，为此容器的健康检查允许启动过程最长持续 300 秒。
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **启动新镜像**，并查看日志，直到迁移应用完毕、服务器能够应答：
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   正常的结果是：如果有需要应用的迁移，会出现一行 `migrations: pre-migration backup written`，然后是服务器的启动横幅，之后 `curl -sf http://localhost:9091/healthz` 能得到应答。

5. **更新之后**，确认版本，并查找错误：
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   如果这个版本改变了视频的编码方式，请清空转码缓存（**管理 → 转码缓存 → 清除全部**）。

## 回到上一个版本

`docker exec evey-server evey admin rollback` 会输出正在运行的版本、数据库结构版本、最新的迁移前转储，以及要在主机上运行的准确命令。它不会改动任何东西：容器没有访问 Docker 的权限。

通常的回退方法是：使用上一个镜像，再加上更新前做的转储：见[退回旧版本](../backups.md#going-back)。如果这次更新删掉了数据（删除了某一列、改变了类型），请恢复转储，而不是向下迁移。如果某一步的 `.down.sql` 就够用，`evey admin migrate down` 则可以一次回退一个迁移。

!!! warning
    除了清除“脏”状态（见下文），不要手动修改 `schema_migrations`，而且修改之前要先做一次转储。

## 迁移中途停止

如果日志显示 `Dirty database version N. Fix and force version.`，说明某个迁移在中途崩溃了，`golang-migrate` 拒绝继续。请先把数据库结构调整到您想要的状态，然后清除这个标记：

```sql
SELECT version, dirty FROM schema_migrations;

-- 手动执行 up.sql 剩下的部分，或者用 down.sql 撤销，
-- 然后记录实际到达的版本：
UPDATE schema_migrations SET version = <N>, dirty = false;
```

重启容器；迁移会从那里继续。另一种恢复办法是恢复迁移前的转储（[恢复](recovery.md#restore-a-pre-migration-dump)）。

## 编写迁移

写给贡献者。迁移文件位于 `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`。已经发布的例子：

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`。在 ADD 执行期间，会对两张表都加 `ACCESS EXCLUSIVE` 锁。在小表上开销很小，在 `media_items` / `watch_progress` 上开销则很大。
- **`000067_sort_title_index.up.sql`**、**`000068_watch_progress_composite_index.up.sql`**、**`000069_quick_connect_shortcode_index.up.sql`** - 普通的 `CREATE INDEX`，在大表上会锁住写入。`CREATE INDEX CONCURRENTLY` 不能在迁移事务里运行。
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`。在 PG 11 及以上版本只改元数据，但仍会短暂持有 `ACCESS EXCLUSIVE` 锁。

准则：

- **耗时长的 ALTER** 如果会改写整张表（改变类型，或者在旧版 PG 上使用非常量默认值），在大型片库上就意味着停机。
- **DROP COLUMN** 只改元数据，但没有备份的话，这一列就再也找不回来了。不要在停止读取这一列的同一个版本里删除它：先停止读取，部署，再在下一个版本里删除。
- **大表上的外键**：先 `ADD CONSTRAINT ... NOT VALID`，再在后续的迁移里 `VALIDATE CONSTRAINT`（验证只需要 `SHARE UPDATE EXCLUSIVE` 锁）。
- **批量 UPDATE / 回填数据**：在大表上，绝不要放在单个迁移里完成。发布一个什么都不做的结构迁移，再加一个后台任务。
- **替换列（扩展、迁移、收缩）**：不要把 `ADD COLUMN` + 回填 + `SET NOT NULL` + `DROP COLUMN` 合并到一个文件中。这种写法之所以能用，只是因为 Evey 以单实例运行，迁移在启动时同步应用；如果是滚动部署，落后的那一半实例就会返回错误。`000077` 和 `000098_playlists_profile_id.up.sql` 就是这样写的，因为已经发布，所以保持原样。新的迁移请分三个版本完成：
  1. **扩展** - 添加可为空的新列，同时写入两列，继续读取旧列；
  2. **迁移** - 回填新列（用后台任务，而不是在迁移里做），然后改为读取新列；
  3. **收缩** - 等到已部署的代码都不再使用旧列，再 `SET NOT NULL` 并删除旧列。

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/en/admin.yaml - sidebar.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
