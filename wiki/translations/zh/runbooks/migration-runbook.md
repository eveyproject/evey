---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
---

# 迁移手册

Evey 怎样执行 schema 迁移，以及怎样安全地操作它们。

## 迁移是怎么跑起来的

迁移文件是**嵌进二进制里的**（`go:embed`），由 `internal/adapters/driven/postgres/migrate.go` 在服务器启动时用 `golang-migrate/migrate/v4` 自动执行。源文件在 `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`。

`evey-server` 一启动，迁移就会自动执行。**另外**还有一套供手动控制的 CLI：`evey admin migrate up | down | version`（实现在 `cmd/evey/admin.go`；`version` 会打印当前的 schema 版本，数据库处于 dirty 状态时以 1 退出）。`Makefile` 里的 `migrate-up` / `migrate-down` 目标就是通过 `./cmd/evey admin migrate` 包了一层。设置了 `EVEY_BACKUP_DIR` 时，迁移之前会自动做一次 `pg_dump`（加 `--no-backup` 可以跳过）。

状态记在 `schema_migrations` 表里（由 golang-migrate 维护）：`{version, dirty}`。

## 迁移前的检查清单

1. **备份数据库。** Evey 自带一个备份端点（见 `docs/backups.md`）：
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   或者直接对容器执行 `pg_dump`：
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   确认导出的文件大小不为零，并且 `gunzip -t` 能通过。

2. 只要迁移动到了大表（media_items、watch_progress、tracks、hls_sessions），就**先看看表有多大**：
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. DDL 动作多的迁移，跑之前**先查活跃会话和锁**：
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   要跑会拿强锁的迁移之前，先把长时间运行的、以及 idle-in-transaction 的会话干掉（`SELECT pg_terminate_backend(pid)`）。

4. **查一下当前的迁移版本**：
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   如果 `dirty = true`，什么都别干，先按“恢复”一节处理。

## 执行一次迁移

迁移在服务器启动时执行。要应用待执行的迁移：

```sh
docker compose pull evey       # if deploying a new image
docker compose up -d evey
docker logs -f evey-server     # watch the startup log
```

跑成功的样子：没有 migrate 报错，服务器的 `/healthz` 返回 200。确认一下：

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## 回滚

用已经接好的 make 目标回退一步：

```sh
make migrate-down     # wraps `evey admin migrate down`, rolls back EXACTLY one step
make migrate-version  # confirm where you landed
```

想再往前退，就一步一步重复执行。设置了 `EVEY_BACKUP_DIR` 时，迁移之前会自动做一次
`pg_dump`。

另一条路是**把上一个镜像重新部署回去**，它内嵌的迁移到版本 N-1 就停了。

> 别手工去改 `schema_migrations`。本手册的某个早期版本声称不存在 `migrate-down` 目标，
> 让运维自己用 `psql` 跑 `.down.sql`，再手动 `UPDATE schema_migrations SET version = ...` -
> 而且是在故障当中。这段话跟它上面九行的“迁移是怎么跑起来的”自相矛盾。
> 那个目标一直都在（`Makefile:314`）。只有当数据库被标成 `dirty` 且 `migrate down`
> 拒绝执行时，才动手做这种外科手术，而且动手之前先导一份 dump。

只要上线的改动会破坏数据（DROP COLUMN、类型变更），而且备份比生产流量分叉的时间点更新，
就一律从迁移前的备份恢复。

## 恢复 - dirty 状态

如果启动日志里出现 `Dirty database version N. Fix and force version.`，说明上一次迁移跑到一半崩了。先手工把 schema 改成你想要的样子，再把这个标志清掉：

```sql
-- 先看清楚：schema_migrations 现在报的是什么？
SELECT version, dirty FROM schema_migrations;

-- 要么手工把 up.sql 里的语句再执行一遍，要么用 down.sql 回退。
-- 然后在你实际停留的那个版本上清掉 dirty 位：
UPDATE schema_migrations SET version = <N>, dirty = false;
```

重启 `evey-server`，golang-migrate 会从那里继续往下走。

## 特殊情况

拿最近几个迁移当参考（见 `internal/adapters/driven/postgres/migrations/`）：

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`。整个 ADD 期间，两张表上都拿着 `ACCESS EXCLUSIVE` 锁。小表上很便宜，落到 `media_items` / `watch_progress` 上就很难受。大表的正确写法：先用 `NOT VALID` 加上，再用第二条语句 `VALIDATE CONSTRAINT`（校验只拿 `SHARE UPDATE EXCLUSIVE`）。
- **`000067_sort_title_index.up.sql`、`000068_watch_progress_composite_index.up.sql`、`000069_quick_connect_shortcode_index.up.sql`** - 就是普通的 `CREATE INDEX`。在大表上，它会锁住写入。可以考虑在维护窗口里手动跑 `CREATE INDEX CONCURRENTLY`（它不能放在 migrate 的事务里 - 得带外执行，再把版本记上）。
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`。在 PG 11 及以上，这只改元数据，很快。但仍会短暂持有 `ACCESS EXCLUSIVE`。

几条准则：

- **会重写整张表的长 ALTER**（类型变更、在老版本 PG 上加一个非常量默认值）→ 必须先查锁，并做好停机的准备。
- **DROP COLUMN** → 只动元数据，但没有备份就再也恢复不了这一列。绝不要在上线新版本代码的同一个版本里删列 - 更好的做法是分两步：先不再读这一列，部署，下个版本再删。
- **大表上的外键** → 先 `ADD CONSTRAINT ... NOT VALID`，再在后续的迁移里 `VALIDATE CONSTRAINT`。别写成一条会阻塞的 `ADD CONSTRAINT ... FOREIGN KEY ...`。
- **批量 UPDATE / 回填** → 在大表上绝不要塞进一次迁移里。发一个空操作的 schema 迁移，加一个后台任务。
- **替换一列（扩展 → 迁移 → 收缩）** → 不要把 `ADD COLUMN` + 回填 + `SET NOT NULL` + 删掉旧列压缩进同一次迁移。这种写法之所以现在还行得通，只是因为 Evey 目前单实例运行、迁移在启动时同步执行：在那个窗口里，读旧列的老代码和读新列的新代码没法共存，所以换成滚动部署或者多副本，落后的那一半就会开始报错。
  已知这样写的迁移，因为早就上线了所以保持原样：`000077` 和 `000098_playlists_profile_id.up.sql`（加上 `profile_id`、回填、置 `NOT NULL`、删掉 `user_id`）。
  以后再写，请拆成三个版本：
  1. **扩展** - 新列以可空的方式加上，写入时*两列都写*，读的还是旧列；
  2. **迁移** - 回填新列（用后台任务，不要放在迁移里），然后把读切到新列；
  3. **收缩** - 等到线上已经没有代码碰旧列了，再 `SET NOT NULL` 并删掉它。
  每一步都能独立部署、独立回退，一个文件写完的版本做不到这一点。

## 迁移之后的检查

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

再对这次迁移涉及的几个端点做个冒烟测试（`/api/v1/...`）。
