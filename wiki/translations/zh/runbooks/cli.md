---
source_sha: 0cbab39ffa95c9d5b92240456049d1f9bc27697af69bc1423e9f3c1f3d4a7556
---

# Evey CLI - `evey admin`

Evey 的服务器二进制文件同时也是运维用的 CLI。不带参数执行 `evey`，它启动 HTTP/gRPC
服务器；把 `admin` 作为第一个参数，它就切换到一次性的运维模式，命令跑完就退出。

所有命令读取的配置，和服务器本身来自同一批 `EVEY_*` 环境变量（`EVEY_DB_USER`、
`EVEY_DB_PASSWORD`、`EVEY_DB_HOST`、`EVEY_DB_PORT`、`EVEY_DB_NAME`、
`EVEY_DB_SSLMODE`、`EVEY_BACKUP_DIR` 等等）。

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

其他任何第一个参数都会被拒绝（退出码 1），而不是悄悄启动服务器。`cmd/evey/admin_commands.go`
里的 Go 表是每个子命令的唯一描述：用法列表、`--help`、手册、补全脚本和容器 shell 都由
`cmd/evey/admin_cli_test.go` 对照它进行检查。

## 速查

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key (kills ALL sessions).
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line (feeds `--user` completion).
evey admin list-libraries                    Print the library root paths, one per line (feeds `--root` completion).
```

### 媒体维护

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one - the fix for slow seek and slow resume.
                                             DRY-RUN BY DEFAULT: pass --apply to rewrite. Each
                                             rewrite is validated, cross-checked, and keeps a
                                             .mkvgobak backup. --limit N for a cautious first run.
                                             --resync also repairs files with a corrupted region the
                                             strict pass refuses (skips the damage, keeps the rest,
                                             reports the lost bytes).
                                             LONG: rewrites each file in full, hours on a large
                                             library. Shows a progress bar while applying.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness
                                             (Cues index, keyframe cues, DefaultDuration) - what the
                                             on-demand HLS rung and seeking rely on.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
evey admin reclassify <media-id>             Force-recompute the encoding complexity class.
evey admin reclassify --all                  Wipe and re-queue every classified item.
```

### 诊断与容量

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (owned by the server UID?),
                                             cache dirs, ffmpeg/mkvgo and disk. --repair applies only
                                             the safe fixes (create + chown missing writable dirs).
evey admin rollback                          Read-only advisor: prints the running version, schema,
                                             newest pre-migration backup and the exact HOST commands
                                             to roll back. The container has no Docker access, by
                                             design, so it advises rather than acts.
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
                                             Needs a LIVE server (it is an HTTP client of the local
                                             gateway). Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count with TTFF and
                                             segment latencies. Scenarios: transcode, natural, mkvgo,
                                             light (80/20), balanced (50/50, default), heavy (20/80),
                                             mkvgo+N. --max-viewers caps the ramp (0 = server default
                                             12). Short sanity run:
                                             `load-test --scenario balanced --max-viewers 4`.
                                             `admission_full` is the ceiling verdict (exit 0), not an
                                             error.
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights ...] [--fps ...] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the admission cost
                                             model. Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights ...] [--crf ...] [--out rows.csv] <file>...
                                             Ladder complexity features on a few scenes per title,
                                             encoded at fixed CRFs and scored. Never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Per-title probe timing and early-stop rate on the files.
```

> 永远最新的完整列表，是不带子命令直接执行 `evey admin` - 由它打印出来。
> 2026-07-23；`reindex-no-cue`，也就是那些反复出现的续播慢故障背后的跳转索引修复，
> 当时就在缺失之列。

## 怎么执行

### 从宿主机执行，前提是整套服务在跑（Docker 开发环境 / 生产环境）

Makefile 把最常用的几项操作包成了容器内的 shell 调用，这样不用 source `.env`，
宿主机上也不用装 pg_dump：

```sh
make migrate-version
make migrate-up
make migrate-down
```

它们底下执行的是 `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'`，
所以 `evey-server` 必须在运行；没在跑的话，先用 `make dev` 把它起来。

### 直接进到运行中的容器里

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### 不使用容器的安装（不用 Docker）

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

预装好的二进制文件里，每一个子命令都已经编译进去了。

## Tab 补全与手册

二进制文件自带 bash 补全和手册，所以两者总是与它所带的子命令一致：

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh 用户：先执行 `autoload -U +X bashcompinit && bashcompinit`，再 source 同一个脚本。
补全覆盖子命令、它们的参数及参数取值（`--mode`、`--backend`、`--scenario`），`--user`
之后的用户邮箱，`--root` 之后服务器已知的媒体库根目录（以及任意目录），`ladder-bench`、
`ladder-probe` 和 `route-bench` 的文件参数只补全媒体文件，`restore-backup` 则补全 `.sql.gz` 转储。

Docker 镜像在构建时就装好了补全，它的管理 shell（`docker exec -it evey-server bash`）
在你输入 `man evey` 时会分页显示 `evey man --text`，因为镜像里没有 `man` 程序。

## 子命令详解

### `migrate`

- `up` 会按顺序应用所有待执行的迁移。设置了 `EVEY_BACKUP_DIR` 时（Docker 镜像里默认是
  `/data/backups`），迁移前会自动做一次 `pg_dump`。Up 跑到一半失败会留下
  `schema_migrations.dirty = true`；见 `migration-runbook.md`。
- `down` 只回滚一个迁移。它是外科手术式的，不是“全部推倒” - 没有 `down --all` 这种东西。
  回滚前同样会做一次 `pg_dump`。
- `version` 打印当前的 schema 版本。上一次迁移跑了一半就失败时（也就是 `Up` 的语句执行了一部分、
  没有全部执行完），它会带着 `DIRTY` 警告以退出码 1 返回。
- `--no-backup` 在两个方向上都会跳过 pg_dump。在没装 `postgres-client` 的宿主机上很有用
  （典型场景是 WSL 开发环境，Makefile 那边已经带上了这个参数）。

### `reset-password`、`rotate-jwt-key`、`restore-backup`、`reindex-search`

完整流程、前提条件和执行后的状态，见 `docs/runbooks/recovery.md`。

## `evey admin` 里*没有*的东西

有三个独立的二进制文件和 `cmd/evey` 放在一起，因为按恢复和 schema 的标准来说，
它们算不上运维命令：

| 二进制 | 用途 | 为什么单独拆出来 |
|---|---|---|
| `cmd/intro-backfill` | 流水线改动之后，对片库的一部分重跑片头识别 | **它是 API 客户端** - 带着 Bearer 令牌去调 `/api/v1/admin/media/{id}/detect-segments`，不在进程内跑。 |
| `cmd/intro-diag` | 诊断某部剧集的 pHash 片头识别为什么匹配上了（或者为什么没匹配上） | **开发者调试**工具 - 从磁盘读 trickplay 雪碧图，逐帧打印汉明距离的匹配图。还可以再跑一遍可选的音频互相关。 |
| `cmd/trickplay-verify` | 核对每一块 trickplay 图块，画的确实是记录的那个时间点的那一帧 | **QA/CI** 工具 - 用 ffmpeg 重新抽出源文件的帧，算 aHash 再和图块比对。 |

它们之所以作为独立二进制发布，是因为谁都不需要服务器那一整套配置接线 -
它们是范围明确、跑一次就完的工具，不属于恢复这个面。

## 相关手册

- `migration-runbook.md` - schema 的部署、回滚，以及 dirty 状态的恢复。
- `recovery.md` - 重置密码、轮换 JWT 密钥、恢复备份、重建搜索索引。
- `incident-runbook.md` - 服务器不对劲时的第一响应。
