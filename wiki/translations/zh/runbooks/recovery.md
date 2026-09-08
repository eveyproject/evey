---
source_sha: 75f950590ae90c9a9a77cfcea37f93cbbe8eb70a0dbea1df56d154a205d2fc12
---

# 恢复手册

这里是几套命令行流程，用于你再也登不进去、怀疑 JWT 密钥泄露，
或者需要把数据库回滚到迁移前的备份这几种情况。

所有命令都以 `evey admin <subcommand>` 的形式执行，读取的配置和服务器一样，
都来自 `EVEY_*` 环境变量。它们**不会**启动服务器 - 大多数在服务器停着的时候执行也很安全。

## reset-password

给一个已有用户重新生成密码。新密码只会往 stdout 打印一次 - 请立刻记下来。

```sh
evey admin reset-password --user admin@example.com
```

输出：
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

注意：
- 用户记录的 `updated_at` 会被更新，除此之外不改动任何状态。
- 已有的会话在自然过期之前依然有效。如果你还想把该用户在所有地方都踢下线，
  请接着执行 `rotate-jwt-key`。
- 明文密码绝不会写进日志。输出弄丢了，就只能再跑一次命令。

## rotate-jwt-key

更换 RSA 签名密钥，强制作废所有活跃会话。

```sh
evey admin rotate-jwt-key
```

行为：
- 当前的密钥文件（路径为 `EVEY_JWT_PRIVATE_KEY_PATH`，默认 `/data/jwt.key`）
  会被重命名为 `<path>.rotated-<UTC-timestamp>`。
- 服务器下次启动时，`LoadOrGenerateRSAKey` 会重新生成一把 2048 位的 RSA 密钥。
- 之前签发的所有访问令牌签名校验都会失败 → 每个用户都被登出。
- 刷新令牌也是用同一把密钥签的 JWT，所以同样作废 - 不需要另外清理。

什么时候用：
- 怀疑密钥泄露。
- 需要一次性杀掉所有会话（应急响应、外包人员离场）。

## restore-backup

把一个 `pg_dump.gz`（由迁移前的备份机制产生，或者来自其他任何来源）导入到配置好的数据库里。
**会覆盖已有数据。**

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

步骤：
1. 命令在动手之前会先停 5 秒 - 按 Ctrl+C 可以中止。
2. 文件以 `gunzip | psql` 的方式流式导入配置好的数据库，
   并带上 `--single-transaction` 和 `ON_ERROR_STOP=1`。
3. 一旦出错，psql 会回滚整个事务 - 数据库保持原样。

前提条件：
- PATH 里必须有 `psql` 和 `gunzip`（生产镜像里和 `pg_dump` 一起都带了）。
- 目标数据库必须已经存在；转储文件里只有表的 DROP/CREATE，没有数据库本身。
- 配置里的 `EVEY_DB_USER` 必须是目标数据库的属主。

迁移前的备份会在每次 `migrate.Up()` 之前自动产生，存放在 `EVEY_BACKUP_DIR` 下
（默认 `/data/backups`）。只保留最近 3 份，更早的会被清理掉。
