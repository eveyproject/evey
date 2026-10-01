---
source_sha: 68ae93ace6a6ac1dc58140bf8cdc28c60bd2db3d0b2ce148ec58433940070f68
reviewed: false
---
# 数据库迁移

Evey 在更新时如何更改数据库架构，以及出问题时该怎么办。命令针对标准安装，其中数据库运行在 `evey-server` 容器内。

## 迁移如何运行

迁移内嵌在二进制文件中（`go:embed`），由 `golang-migrate` 在服务器启动时应用（`internal/adapters/driven/postgres/migrate.go`，文件为 `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`）。状态保存在 `schema_migrations` 表中：`{version, dirty}`。

在应用任何内容之前，服务器会把完整的转储写入 `/data/backups/pre-migration-<version>-<time>.sql.gz`（`EVEY_BACKUP_DIR`）。最近 3 个架构版本中每个版本保留最新的一个转储。

同样的操作也可以手动执行：

```sh
docker exec evey-server evey admin migrate version   # 架构为 dirty 时退出码为 1
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # 恰好一步
```

`up` 和 `down` 也会先做一次转储；`--no-backup` 会跳过它。

要对内置数据库执行 SQL：

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## 更新之前

1. **做一次备份**，在 **Admin → 备份与恢复** 中（**立即运行**），或者通过脚本：
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   自动的迁移前转储是第二份副本，而不是替代品。

2. **对于大型片库**，查看最大的几张表的大小：改写其中一张表的迁移会花更长时间。
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **检查当前版本**：`evey admin migrate version`。如果它报告架构处于 dirty 状态，先看[中途停止的迁移](#a-migration-that-stopped-halfway)。

## 运行迁移

它们在新镜像启动时运行：

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

迁移应用完毕且服务器能够响应后，容器会报告为健康：

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## 回退

`evey admin rollback` 会输出正在运行的版本、架构版本、最新的迁移前转储，以及要在主机上运行的具体命令。它不做任何更改：容器无法访问 Docker。

通常的回退方式是使用之前的镜像，并配合更新前创建的转储：见备份与更新中的[回退](../backups.md#going-back)。`evey admin migrate down` 则是一次回滚一步，适用于该步骤的 `.down.sql` 就足够的情况。

不要手动编辑 `schema_migrations`，除非是为了清除 dirty 状态（见下文），并且要先做一次转储。

当更新删除了数据（移除了某一列、改变了类型）时，请恢复迁移前的转储，而不是向下迁移。

## 中途停止的迁移

如果日志显示 `Dirty database version N. Fix and force version.`，说明某个迁移中途崩溃了。把架构恢复到你想要的状态，然后清除这个标记：

```sql
SELECT version, dirty FROM schema_migrations;

-- 手动重新应用 up.sql 的剩余部分，或者用 down.sql 回退，
-- 然后记录你实际到达的版本：
UPDATE schema_migrations SET version = <N>, dirty = false;
```

重启容器；迁移会从那里继续。恢复迁移前的转储是另一条出路。

## 编写迁移

面向贡献者。示例在 `internal/adapters/driven/postgres/migrations/` 中：

- **`000066_cascade_and_email_unique.up.sql`**：`ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`。在 ADD 期间对两张表都持有 `ACCESS EXCLUSIVE`。对小表来说开销很小，对 `media_items` / `watch_progress` 则很痛苦。大表的模式：先用 `NOT VALID` 添加，再在第二条语句中 `VALIDATE CONSTRAINT`（验证只需要 `SHARE UPDATE EXCLUSIVE`）。
- **`000067_sort_title_index.up.sql`、`000068_watch_progress_composite_index.up.sql`、`000069_quick_connect_shortcode_index.up.sql`**：普通的 `CREATE INDEX`。在大表上这会锁住写入。`CREATE INDEX CONCURRENTLY` 不能在 migrate 事务中运行。
- **`000072_syncplay_queue.up.sql`**：`ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`。在 PG 11+ 上这只涉及元数据，速度很快。但仍会短暂持有 `ACCESS EXCLUSIVE`。

准则：

- 会改写整张表的**长 ALTER**（类型变更、在旧版 PG 上添加非常量默认值）→ 在大型片库上预计会有停机时间。
- **DROP COLUMN** → 只涉及元数据，但没有备份就无法恢复该列。绝不要在推出新版本代码的同一个版本中删除列：最好分两步，先停止读取该列，部署，然后在下一个版本中删除。
- **大表上的外键** → 先 `ADD CONSTRAINT ... NOT VALID`，再在后续迁移中 `VALIDATE CONSTRAINT`。避免一次性阻塞的 `ADD CONSTRAINT ... FOREIGN KEY ...`。
- **批量 UPDATE / 回填** → 绝不在大表上用一个迁移完成。发布一个不做事的架构迁移 + 一个后台任务。
- **替换列（expand → migrate → contract）** → 不要把 `ADD COLUMN` + 回填 + `SET NOT NULL` + 删除旧列的 `DROP COLUMN` 合并到一个迁移中。这种做法之所以能工作，只是因为 Evey 目前以单实例运行，迁移在启动时同步应用：在这个窗口期内，读取已删除列的旧代码和读取新列的新代码无法共存，所以滚动部署或多副本部署会让落后的那一半返回错误。
  已知以这种方式编写、因已发布而保持原样的迁移：`000077` 和 `000098_playlists_profile_id.up.sql`（添加 `profile_id`、回填、设置 `NOT NULL`、删除 `user_id`）。
  对于新的迁移，分三个版本完成：
  1. **expand**：添加允许为空的新列，同时写入*两个*列，继续读取旧列；
  2. **migrate**：回填新列（用后台任务，而不是迁移），然后把读取切换到新列；
  3. **contract**：一旦没有任何已部署的代码再访问旧列，就 `SET NOT NULL` 并删除它。
  每一步都可以独立部署和回退，而单文件的做法做不到这一点。

## 更新之后

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

然后，如果这个版本改变了视频的编码方式，清空转码缓存（**Admin → 转码缓存**）。
