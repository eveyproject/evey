---
source_sha: b5db9ddecc482effde77439df0be753f33fe76e853d0afa1c058a21e0593880d
reviewed: false
---
# Evey CLI - `evey admin`

Evey 服务器的二进制文件同时也是运维 CLI。不带参数调用 `evey` 时，它会启动 HTTP/gRPC 服务器；第一个参数为 `admin` 时，它会切换到一次性的运维模式，命令完成后即退出。

所有命令都从与服务器本身相同的 `EVEY_*` 环境变量读取配置（`EVEY_DB_USER`、`EVEY_DB_PASSWORD`、`EVEY_DB_HOST`、`EVEY_DB_PORT`、`EVEY_DB_NAME`、`EVEY_DB_SSLMODE`、`EVEY_BACKUP_DIR`……）。

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

任何其他第一个参数都会被拒绝（退出码 1），而不是悄悄启动服务器。`cmd/evey/admin_commands.go` 中的 Go 表格是每个子命令的唯一描述：用法列表、`--help`、手册、补全脚本和容器 shell 都由 `cmd/evey/admin_cli_test.go` 对照它进行检查。

## 快速参考

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key and end every session.
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required; not the panel's .tar.gz backups).
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

### 演示实例

```
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

> 完整且始终最新的列表就是不带子命令的 `evey admin`：它会输出二进制文件包含的每个子命令。

## 如何调用

### 在容器中（标准安装）

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # 管理 shell：evey admin ...、man evey
```

镜像设置了数据库密码文件，所以这些命令无需任何额外环境就能连到内置数据库。

### 在裸机安装上（没有 Docker）

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

预装的二进制文件已经内置了所有子命令。

### 在开发环境中

Makefile 封装了迁移命令，它们在开发容器 `evey-server` 中从源代码树运行：

```sh
make migrate-version
make migrate-up
make migrate-down
```

## Tab 补全与手册

二进制文件自带 bash 补全和手册，所以两者总是与它包含的子命令一致：

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh 用户：先执行 `autoload -U +X bashcompinit && bashcompinit`，然后加载同一个脚本。补全覆盖子命令、它们的选项和选项值（`--mode`、`--backend`、`--scenario`）、`--user` 之后的用户邮箱、`--root` 之后服务器已知的媒体库根目录（以及任何目录）、`ladder-bench`、`ladder-probe` 和 `route-bench` 的文件参数仅限媒体文件，以及 `restore-backup` 的 `.sql.gz` 转储。

Docker 镜像在构建时安装补全，它的管理 shell（`docker exec -it evey-server bash`）通过分页显示 `evey man --text` 来响应 `man evey`，因为镜像中没有 `man` 程序。

## 子命令详情

### `migrate`

- `up` 按顺序应用所有待执行的迁移。当设置了 `EVEY_BACKUP_DIR` 时（Docker 镜像中默认为 `/data/backups`），会自动进行一次迁移前的 `pg_dump`。在 Up 中途失败会留下 `schema_migrations.dirty = true`；见 `migration-runbook.md`。
- `down` 恰好回滚一个迁移。这是精确的操作，而不是“全部拆掉”：没有 `down --all`。它同样会先进行一次回滚前的 `pg_dump`。
- `version` 输出当前的架构版本。当之前的某个迁移中途失败时（即部分而非全部 `Up` 语句已执行），返回退出码 1 并带有 `DIRTY` 警告。
- `--no-backup` 在两个方向上都跳过 pg_dump。适用于没有安装 `postgres-client` 的主机（典型的 WSL 开发环境，Makefile 已经传入这个选项）。

### `reset-password`、`rotate-jwt-key`、`restore-backup`、`reindex-search`

完整步骤、前提条件和事后状态见 `docs/runbooks/recovery.md`。

## `evey admin` 中*没有*的东西

有三个独立的二进制文件位于 `cmd/evey` 旁边，因为它们不是恢复或架构意义上的运维命令：

| 二进制文件 | 用途 | 为什么独立 |
|---|---|---|
| `cmd/intro-backfill` | 在流水线变更后，为片库的一部分刷新片头检测 | **API 客户端**：用 bearer 令牌访问 `/api/v1/admin/media/{id}/detect-segments`，不在进程内运行。 |
| `cmd/intro-diag` | 诊断 pHash 片头检测为什么在某部剧上匹配（或没有匹配） | **开发者调试**工具：从磁盘读取 trickplay 精灵图，逐帧输出汉明距离匹配图。可选的音频互相关步骤。 |
| `cmd/trickplay-verify` | 验证每个 trickplay 图块确实显示了所记录时间戳处的画面 | **QA/CI** 工具：用 ffmpeg 重新提取源画面，并与图块比较 aHash。 |

它们作为独立的二进制文件发布，是因为它们都不需要完整的服务器配置：它们是范围明确、一次性运行的工具，不属于恢复功能的一部分。

## 相关手册

- `migration-runbook.md`：架构部署、回滚、dirty 状态恢复。
- `recovery.md`：密码重置、JWT 密钥轮换、备份恢复、重建搜索索引。
- `incident-runbook.md`：服务器运行异常时的第一响应。
