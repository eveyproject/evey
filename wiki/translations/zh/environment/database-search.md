---
source_sha: c19f05aea8bece6fa3bf10a82d5590583384092dde0102fcdd2192bff8c0c547
reviewed: false
---
# 数据库与搜索

默认情况下，镜像运行它自己的 Postgres 和 Meilisearch 并生成它们的密码：这些变量都不需要。它们用于自带数据库或搜索引擎、或者需要调整内置组件的安装。见[高级部署](../operations/advanced-deployments.md)。

## 数据库

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | 未设置 | 未设置：当 `EVEY_DB_HOST` 未设置时使用内置数据库，否则使用外部数据库。`true` 或 `false` 用于明确决定。由入口脚本读取。 |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | 内置集群所在的位置。必须位于本地文件系统上，而不是网络共享上。 |
| `EVEY_DB_HOST` | `localhost` | 数据库主机。**设置它会关闭内置数据库。** |
| `EVEY_DB_PORT` | `5432` | 数据库端口。 |
| `EVEY_DB_USER` | `evey` | 数据库用户。 |
| `EVEY_DB_NAME` | `evey` | 数据库名称。 |
| `EVEY_DB_PASSWORD` | 自动生成 | 密码。使用外部数据库时，如果值为 `evey`，服务器会拒绝启动。 |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | 存放密码的文件，在 `EVEY_DB_PASSWORD` 未设置时读取。由镜像设置。 |
| `EVEY_DB_SSLMODE` | 环回地址上为 `disable`，否则为 `prefer` | `disable`、`prefer`、`require` 或 `verify-full`。 |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | 到数据库的打开连接数。每个连接在 Postgres 一侧都会消耗内存；只有大型部署才需要提高它。 |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | 连接池中保持空闲的连接数。 |

## 搜索

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` 表示使用 `EVEY_MEILI_URL` 上的 Meilisearch，而不是内置的。由入口脚本读取。 |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Meilisearch 的地址。 |
| `MEILI_MASTER_KEY` | 自动生成 | Meilisearch 密钥。使用外部实例时，空的或占位的密钥会导致拒绝启动。 |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | 存放密钥的文件，在 `MEILI_MASTER_KEY` 未设置时读取。由镜像设置。 |
| `MEILI_MAX_INDEXING_MEMORY` | 约为容器内存的 20 %，至少 128 MiB | 内置 Meilisearch 可用于建立索引的内存。根据容器的限制而不是机器的内存计算，这样扫描不会耗尽一个小容器。显式的值会原样传递。 |
