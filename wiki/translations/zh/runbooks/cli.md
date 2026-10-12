---
source_sha: 4da6d87a4495603598041edc6216fa889229d188f8579a3799818f7ddbf73246
reviewed: true
---
# 命令行

服务器程序本身也是运维人员用的命令行：`evey admin <subcommand>` 会用和服务器相同的配置和数据库，运行一个维护或恢复工具，然后退出。本页列出这些命令，以及如何在容器里运行它们；日常操作流程（忘记密码、轮换密钥、恢复备份）见[恢复](recovery.md)。

## 运行命令

在标准安装中，命令在容器里运行，容器里已经有数据库密码和 `EVEY_*` 配置：

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # 管理用的 shell：evey admin ...、man evey
```

在不使用 Docker 的安装中，直接调用这个程序（`evey admin reset-password --user alice@example.com`）；它读取和服务器相同的 `EVEY_*` 变量（`EVEY_DB_HOST`、`EVEY_DB_USER`、`EVEY_DB_PASSWORD`、`EVEY_DB_NAME`、`EVEY_BACKUP_DIR`...）。大多数命令需要数据库；没有一个命令会启动服务器，而且大多数命令在服务器停止时运行也是安全的。

| 调用方式 | 作用 |
| --- | --- |
| `evey` | 启动服务器。 |
| `evey admin <subcommand>` | 运行一个运维工具，然后退出。只输入 `evey admin` 会列出所有命令，并以退出码 1 退出；`evey admin <subcommand> --help` 显示某个命令的详细说明。 |
| `evey version` | 版本、提交、构建日期。它不需要配置，也不需要数据库，所以在一个无法启动的容器里也能输出结果。 |
| `evey help` | 命令摘要。 |
| `evey man [--text]` | 使用手册：给 `man` 用的 roff 格式，加 `--text` 则输出纯文本。 |
| `evey completion bash` | bash 自动补全脚本。 |

其他任何第一个参数都会被拒绝（退出码 1），而不会在没有任何提示的情况下启动服务器。

## 命令列表

下面的列表就是程序自己的 `--help` 文本，分组方式和 `evey admin` 输出的一样。不带子命令运行 `evey admin`，得到的永远是当前的列表。

### 恢复

```
evey admin reset-password --user <email>     Generate a new password for the user, print it once.
                                             Every session of that user is ended at the same time.
evey admin rotate-jwt-key                    Regenerate the JWT signing key. Invalidates ALL active
                                             sessions: the key is moved aside (kept as
                                             <key>.rotated-<timestamp>), the next server boot
                                             generates a new one, and every session is ended in the
                                             database at once.
evey admin backup [--include-telemetry]
                                             Create a database backup in the backup directory, the
                                             same .tar.gz as the panel and the scheduler. Telemetry
                                             follows the panel switch (off by default);
                                             --include-telemetry includes it for this run.
evey admin restore-backup <path-to-dump.sql.gz>
                                             Restore the database from a pg_dump.gz file, in a single
                                             transaction. The whole file is checked first: a truncated
                                             or corrupt dump is refused before anything is dropped.
                                             Existing rows are OVERWRITTEN; waits 5 s for a Ctrl-C
                                             before it starts. The .sql.gz suffix is required (not the
                                             panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]
                                             Rebuild the Meilisearch index from the database (no
                                             metadata fetch). --purge clears the index first, which
                                             drops orphan documents. If Meilisearch is down, search
                                             is served by the Postgres fallback: the command reports
                                             that and does nothing.
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

`reset-password` 和 `rotate-jwt-key` 都会结束会话：前者结束这位用户的会话，后者结束所有人的会话。已打开的应用大约在 30 秒内会发现，并要求重新登录。完整步骤见[恢复](recovery.md)。

### 数据库结构

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

设置了 `EVEY_BACKUP_DIR` 时（Docker 镜像里是 `/data/backups`），`up` 和 `down` 在迁移前都会先做一次 `pg_dump`；`--no-backup` 会跳过这一步，用于没有 `pg_dump` 的主机。如果 `up` 中途失败，会留下 `schema_migrations.dirty = true`：见[数据库迁移](migration-runbook.md)。在开发环境里，Makefile 把这三条命令封装成 `make migrate-up`、`make migrate-down` 和 `make migrate-version`，在开发容器 `evey-server` 里运行。

### 媒体维护

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--mode auto|inplace|rewrite] [--deep-verify] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one, fixing slow resume. DRY-RUN BY DEFAULT: pass
                                             --apply to rewrite. Each rewrite is validated and
                                             cross-checked and keeps a .mkvgobak backup; --limit N for
                                             a cautious first run; --keep-backup=false drops the
                                             backups and enforces --deep-verify instead. --mode auto
                                             takes the surgical in-place patch when the file layout
                                             allows it and rewrites otherwise; inplace patches only
                                             (no backup); rewrite always rebuilds and swaps.
                                             --resync repairs files with a CORRUPTED region the strict
                                             pass refuses: it skips the damage, keeps the rest and
                                             reports the lost bytes. LONG: rewrites each file in full
                                             (hours on a large library); shows a progress bar.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness: a
                                             missing Cues index, cue points on a non-video track, cue
                                             times matching no keyframe, video tracks without a
                                             DefaultDuration. A file with errors falls back to the
                                             ffmpeg pipeline. --errors-only skips the warnings.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
```

**管理 → 媒体健康**背后运行的是同一套引擎，用它来修复媒体库更简单：见[媒体健康](media-health.md)。

### 按影片编码

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### 诊断

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (can the server write each
                                             library root?), cache dirs, ffmpeg/mkvgo, trusted
                                             proxies, remote access and disk. One line per check
                                             (ok / WARN / FAIL) with a remediation hint; exits 1 when
                                             a check fails. --repair applies only the safe fixes
                                             (create and chown the missing writable dirs).
evey admin rollback                          Read-only advisor: running version, schema, newest
                                             pre-migration backup and the exact HOST commands to roll
                                             back. The container has no Docker access, by design, so
                                             it advises rather than acts.
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights 1080,2160] [--fps 24,30,50,60] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the cost model.
                                             Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...
                                             Encode a few scenes of each title at fixed CRFs and
                                             score encode cost against the ladder's complexity
                                             features. Runs ffmpeg for minutes; never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Run the per-title probe on the given files: probe time,
                                             early-stop rate and points. Reads and encodes only.
```

### 容量

```
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
                                             Needs a LIVE server: it is an HTTP client of the local
                                             gateway. Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count and latencies.
                                             Scenarios: transcode, natural, mkvgo, light (80/20),
                                             balanced (50/50, default), heavy (20/80), mkvgo+N.
                                             Modes: ramp (default), flash, seek, endurance. Presets:
                                             minimal (4 viewers), standard (12), max (32); an explicit
                                             --max-viewers wins. --cold-cache keeps evicting the
                                             library from the page cache so the run measures the
                                             storage, not the RAM. admission_full is the ceiling
                                             verdict (exit 0), not an error. Short sanity run:
                                             load-test --scenario balanced --max-viewers 4
```

### 补全数据来源

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## Tab 自动补全和使用手册

程序自带 bash 自动补全和使用手册，所以两者总是和它附带的子命令一致：

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh 用户：先运行 `autoload -U +X bashcompinit && bashcompinit`，再加载同一个脚本。自动补全涵盖：子命令、它们的选项和选项值（`--mode`、`--backend`、`--scenario`）、`--user` 后面的用户邮箱、`--root` 后面服务器已知的媒体库根目录（以及任意目录）、`ladder-bench`、`ladder-probe` 和 `route-bench` 的文件参数只补全媒体文件，以及 `restore-backup` 的 `.sql.gz` 转储文件。

Docker 镜像在构建时就安装好了自动补全。镜像里没有 `man` 程序，所以在它的管理 shell（`docker exec -it evey-server bash`）里输入 `man evey`，会分页显示 `evey man --text` 的内容。

## `evey admin` 未包含的程序

源代码树里，服务器旁边还有三个独立的程序。它们是开发和测试用的工具，不是运维命令，Docker 镜像里也不包含它们：

| 程序 | 用途 |
| --- | --- |
| `cmd/intro-backfill` | 在处理流程改动之后，对目录中的一部分内容重新进行片头检测。它是一个使用 bearer 令牌的 API 客户端，不在服务器进程内运行。 |
| `cmd/intro-diag` | 根据磁盘上的 trickplay 拼图，解释片头检测为什么在某部剧集上匹配成功或没有匹配成功。 |
| `cmd/trickplay-verify` | 用 ffmpeg 重新提取原始画面，检查每个 trickplay 小图显示的是否正是它记录的时间点上的画面。 |

## 相关手册

- [恢复](recovery.md)：重置密码、轮换 JWT 密钥、恢复备份。
- [数据库迁移](migration-runbook.md)：部署数据库结构、回退、从“脏”状态恢复。
- [事故手册](incident-runbook.md)：服务器运行异常时的初步处理。

<!-- sources
dispatch and top-level commands: cmd/evey/cli.go - runCLI (version, help, completion, man, unknown command refused), cmd/evey/main.go
subcommand table, groups, --help text: cmd/evey/admin_commands.go - adminCommands, adminGroups, printAdminUsage, adminUsage (exit 1 without subcommand) ; drift test cmd/evey/admin_cli_test.go
sessions ended: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL internal/app/user/token_epoch.go
backup and restore: cmd/evey/admin_backup.go - adminBackup ; cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete
container: Dockerfile (EVEY_DB_PASSWORD_FILE, EVEY_BACKUP_DIR=/data/backups, completion at build time) ; docker/evey-shell.sh - man()
makefile: Makefile migrate-up / migrate-down / migrate-version
standalone programs: cmd/intro-backfill, cmd/intro-diag, cmd/trickplay-verify
-->
