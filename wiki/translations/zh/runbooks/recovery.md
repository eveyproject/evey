---
source_sha: b5e748f389e0659b78085aa23f5f39cca600299c69e8ea3f206609f753a37861
reviewed: true
---
# 恢复 { #recovery }

当所有人都无法登录、用来签发访问令牌的密钥可能已经泄露，或者数据库必须回到迁移前的转储时，请打开本页。每个步骤都是在容器里运行一条 `evey admin` 命令；其他子命令见[命令行](cli.md)页面。

## 开始之前 { #before-you-start }

这些命令读取和服务器相同的 `EVEY_*` 配置，所以在服务器的容器里运行它们，不需要任何别的东西：

```sh
docker exec evey-server evey admin <subcommand>        # 服务器正在运行时
docker compose run --rm evey admin <subcommand>        # 容器已停止时
```

第二种方式会先启动内置的数据库，再运行命令，然后退出。这些命令都不会启动服务器。

## 给用户一个新密码 { #give-a-user-a-new-password }

在用户无法登录时使用，或者在怀疑账户被盗之后立即使用。新密码只显示一次，而且从不写进日志：关闭终端之前请先把它复制下来。

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

结果正常时是这样的：

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

这位用户的所有会话会立即结束：刷新令牌被删除，访问令牌被吊销。已打开的应用大约在 30 秒内会发现，并要求重新登录。如果输出丢失，再运行一次命令即可。

## 轮换签发访问令牌的密钥 { #rotate-the-key-that-signs-access-tokens }

在密钥文件可能已经泄露时使用，或者想让所有用户一次性退出登录时使用。如果只涉及一个账户，用**管理 → 用户 → 禁用**或上面的重置密码就够了。

```sh
docker exec evey-server evey admin rotate-jwt-key
```

会发生什么：

1. 密钥文件（`EVEY_JWT_PRIVATE_KEY_PATH`，默认是 `/data/jwt.key`）被重命名为 `<path>.rotated-<UTC timestamp>`。
2. 在同一个事务里，删除所有刷新令牌，并吊销所有访问令牌。已打开的应用大约在 30 秒内会发现；所有人都要重新登录。
3. 服务器下次启动时，会生成一个新的 2048 位 RSA 密钥。

之后请重启容器（`docker compose restart evey`），让新密钥生效。如果连不上数据库，密钥文件仍然会被重命名移走，命令会显示 `key rotated, but sessions could not be ended (restart the server, then run this again)`：请按此操作，先重启服务器，再运行一次这条命令。

## 恢复迁移前的转储 { #restore-a-pre-migration-dump }

当某次更新必须撤销、而且之后写入的数据可以丢弃时使用。这条命令会把一个 `.sql.gz` 转储通过 `gunzip | psql` 导入到配置好的数据库里。

!!! warning
    恢复会先删除并重建每张数据表再加载，因此当前的数据会被替换。如果您有可能还需要当前的状态，请先给它做一次备份（**管理 → 数据库备份 → 立即运行**）。

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

会发生什么：

1. 命令先读取整个文件：文件必须能完整解压，并以 `pg_dump` 的完成标记行结尾。被截断或损坏的转储会被拒绝（`refusing to restore: ...`），这时还没有删除任何东西。
2. 然后它会等待 5 秒再进行任何更改：按 Ctrl+C 可以中止。
3. `psql` 以 `--single-transaction` 和 `ON_ERROR_STOP=1` 应用转储。只要出现任何错误，事务就会回滚，数据库保持原样。
4. 成功时，最后一行是 `[admin] restored backup <path>`。

要求：

- 文件名必须以 `.sql.gz` 结尾。**管理 → 数据库备份**生成的 `.tar.gz` 归档要在面板里恢复，不能用这条命令。
- 目标数据库必须已经存在；转储会先删除并重建每张数据表再加载，所以可以恢复到已有数据的数据库上，但它不会创建数据库本身。
- `psql` 和 `gunzip` 必须在 PATH 里，镜像已经保证了这一点。

每次改变数据库结构之前，都会自动写入迁移前的转储，位于 `EVEY_BACKUP_DIR` 下（`/data/backups`，也就是主机上的 `./evey-data/backups`），文件名为 `pre-migration-<schema version>-<UTC timestamp>.sql.gz`。最近 3 个数据库结构版本中，每个版本保留最新的一份转储，其余的会被清理掉。什么时候应该恢复转储、而不是把数据库结构回退一步，见[数据库迁移](migration-runbook.md)。

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete (whole gzip + completion marker before anything runs), 5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
