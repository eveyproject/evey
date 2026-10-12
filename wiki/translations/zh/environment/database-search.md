---
source_sha: 6efce4118fc9a8bfa56a70e2648cc5f619c415c257832c5a9ea6da0e9e98e645
reviewed: true
---
# 数据库与搜索

这些是 Postgres 数据库和 Meilisearch 索引的变量。默认情况下，镜像自己运行这两者，并自动生成它们的密码：
标准安装一个都不用设置。它们用于自带数据库或搜索引擎的安装，或者用来调整内置的那两个；
具体步骤见[高级部署](../operations/advanced-deployments.md)。它们在面板里都没有对应的设置。

## 数据库

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | 未设置 | 未设置：没有设置 `EVEY_DB_HOST` 时使用内置数据库，否则使用外部数据库。设为 `true` 或 `false` 可以明确决定。由入口脚本读取。 | 无 |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | 内置数据库集群存放的位置。它必须在本地文件系统上，不能在网络共享上。由入口脚本读取。 | 无 |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | 一个用 gzip 压缩的 `pg_dump` 文件，在内置集群第一次创建时导入，然后改名为 `*.imported-<date>`。如果集群已经存在，就忽略它，并给出一条提示。由入口脚本读取。 | 无 |
| `EVEY_DB_HOST` | `localhost` | 数据库主机。**设置了它，内置数据库就会关闭。** | 无 |
| `EVEY_DB_PORT` | `5432` | 数据库端口。 | 无 |
| `EVEY_DB_USER` | `evey` | 数据库登录名。如果它是超级用户（内置集群和随附的 compose 文件就是这样），服务器只在恢复备份时使用它，平时以 `EVEY_DB_APP_USER` 运行。 | 无 |
| `EVEY_DB_APP_USER` | `evey_app` | 当 `EVEY_DB_USER` 是超级用户时，服务器和它的迁移所使用的无特权角色。它在启动时创建，数据库及其中的对象都交给它，每次启动都会得到一个新的随机密码（不写到任何地方）。这样，即使有 SQL 注入，也无法在主机上运行程序或读取文件。 | 无 |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` 让所有操作都以 `EVEY_DB_USER` 运行，包括超级用户的操作。这是数据库拒绝这种拆分时的退路；否则启动会停止，并指出这个变量。 | 无 |
| `EVEY_DB_NAME` | `evey` | 数据库名称。 | 无 |
| `EVEY_DB_PASSWORD` | 自动生成 | 密码。使用外部数据库时，如果值是 `evey`，服务器拒绝启动；如果是 `changeme`，会给出警告。 | 无 |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | 存放密码的文件，在没有设置 `EVEY_DB_PASSWORD` 时读取。由镜像设置。 | 无 |
| `EVEY_DB_SSLMODE` | 本机回环时为 `disable`，否则为 `prefer` | `disable`、`prefer`、`require` 或 `verify-full`。对远程主机使用 `prefer` 会记录一条警告：请使用 `require` 或 `verify-full`。 | 无 |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | 打开的数据库连接数。每个连接都会占用 Postgres 那边的内存；只有大型部署才需要调高。 | 无 |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | 连接池里保留的空闲连接数。不会超过打开的连接数。 | 无 |

## 搜索

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` 表示使用 `EVEY_MEILI_URL` 指向的 Meilisearch，而不是内置的那个。由入口脚本读取。 | 无 |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Meilisearch 的地址。 | 无 |
| `MEILI_MASTER_KEY` | 自动生成 | Meilisearch 的密钥。使用外部实例时，密钥为空或是占位值，服务器拒绝启动；密钥少于 16 个字符，会记录一条警告。 | 无 |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | 存放密钥的文件，在没有设置 `MEILI_MASTER_KEY` 时读取。由镜像设置。 | 无 |
| `MEILI_DATA_DIR` | `/data/meili` | 内置索引存放的位置。由入口脚本读取。 | 无 |
| `MEILI_MAX_INDEXING_MEMORY` | 约为容器内存的 20 %，至少 128 MiB | 内置的 Meilisearch 建索引时可以使用的内存。它按容器的内存上限计算，而不是按整台机器的内存，这样一次扫描不会耗尽小容器的内存。明确给出的值会原样传过去。 | 无 |

<!-- sources
internal/adapters/driven/postgres/approle.go - EnsureAppRole, HandOverToAppRole (approle_test.go)
cmd/evey/database.go - appLogin
internal/config/config.go - EVEY_DB_*, defaultSSLMode, envSecret (_FILE), EVEY_MEILI_URL, MEILI_MASTER_KEY ; WarnValidate (password 'evey' / 'changeme', sslmode prefer, idle > open, key length)
docker/entrypoint.sh - EVEY_EMBEDDED_POSTGRES, EVEY_EMBEDDED_MEILI, EVEY_POSTGRES_DATA_DIR, EVEY_POSTGRES_IMPORT, MEILI_DATA_DIR, MEILI_MAX_INDEXING_MEMORY
Dockerfile - EVEY_DB_PASSWORD_FILE, MEILI_MASTER_KEY_FILE
-->
