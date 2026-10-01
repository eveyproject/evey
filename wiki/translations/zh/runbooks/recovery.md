---
source_sha: 64f12b2cc77ed7069bf33f581c2d91e3b13b6ff04cc4a3bc62db72c3607b46fe
reviewed: false
---
# 恢复手册

用于以下情况的 CLI 步骤：你无法再登录、怀疑 JWT 密钥已泄露，或者需要把数据库回滚到迁移前的备份。

所有命令都以 `evey admin <subcommand>` 的形式运行，并从与服务器相同的 `EVEY_*` 环境变量读取配置。它们**不会**启动服务器：大多数可以在服务器停止时安全运行。

## reset-password

为一个现有用户生成新密码。新密码会在标准输出上显示一次：请立即记下。

```sh
evey admin reset-password --user admin@example.com
```

输出：

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

说明：
- 该用户的所有会话都会结束：其刷新令牌被删除，访问令牌被吊销。已打开的应用会在大约 30 秒内察觉并要求重新登录。
- 明文密码从不记录到日志中。如果你丢失了输出，就必须再次运行该命令。

## rotate-jwt-key

替换签署访问令牌的 RSA 密钥。

```sh
evey admin rotate-jwt-key
```

行为：
- 当前的密钥文件（路径 = `EVEY_JWT_PRIVATE_KEY_PATH`，默认 `/data/jwt.key`）会被重命名为 `<path>.rotated-<UTC-timestamp>`。
- 下次服务器启动时，`LoadOrGenerateRSAKey` 会重新生成一个新的 2048 位 RSA 密钥。
- 所有会话立即结束，无需等待重启：该命令会删除数据库中的所有刷新令牌并吊销所有访问令牌。已打开的应用会在大约 30 秒内察觉，所有人都需要重新登录。
- 如果无法连接数据库，密钥仍会被移到一旁，命令会提示这一点：数据库恢复响应后请再次运行。

何时使用：
- 怀疑密钥泄露：没有人能再用旧密钥签发有效的访问令牌。
- 批量终止会话。对于单个账户，在 **Admin → 用户** 中禁用它，或使用 `reset-password`，就足够了。

## restore-backup

把由迁移前备份机制（或任何其他来源）生成的 `pg_dump.gz` 应用到配置的数据库中。**会覆盖现有数据。** 文件必须以 `.sql.gz` 结尾：**Admin → 备份与恢复** 的 `.tar.gz` 备份是从面板恢复的，而不是用这个命令。

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

步骤：
1. 命令在执行任何操作之前会暂停 5 秒：按 Ctrl+C 可以中止。
2. 文件通过 `gunzip | psql` 流式写入配置的数据库，使用 `--single-transaction` 和 `ON_ERROR_STOP=1`。
3. 出现任何错误时，psql 会回滚事务：数据库保持原来的状态。

要求：
- `psql` 和 `gunzip` 必须在 PATH 中（两者都与 `pg_dump` 一起包含在生产镜像中）。
- 目标数据库必须存在；转储只包含表的 DROP/CREATE，不包含数据库本身。
- 配置的 `EVEY_DB_USER` 必须是目标数据库的所有者。

迁移前备份在每次 `migrate.Up()` 之前自动生成，位于 `EVEY_BACKUP_DIR` 下（默认 `/data/backups`）。最近 3 个架构版本中每个版本只保留最新的一个转储；其他的会被清理。
