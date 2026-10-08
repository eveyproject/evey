# Database and search

The variables for the Postgres database and the Meilisearch index. By default
the image runs both itself and generates their passwords: a standard
installation sets none of these. They serve an installation that brings its
own database or search engine, or tunes the embedded ones; the procedures
are in [Advanced deployments](../operations/advanced-deployments.md). None of
them has a panel equivalent.

## Database

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | unset | Unset: embedded when `EVEY_DB_HOST` is unset, external otherwise. `true` or `false` to decide explicitly. Read by the entrypoint. | none |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | Where the embedded cluster lives. It must be on a local filesystem, not a network share. Read by the entrypoint. | none |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | A gzipped `pg_dump` restored into the embedded cluster the first time it is created, then renamed `*.imported-<date>`. Ignored, with a message, when the cluster already exists. Read by the entrypoint. | none |
| `EVEY_DB_HOST` | `localhost` | The database host. **Setting it turns the embedded database off.** | none |
| `EVEY_DB_PORT` | `5432` | The database port. | none |
| `EVEY_DB_USER` | `evey` | The database login. When it is a superuser (the embedded cluster and the shipped compose), the server keeps it for the restore only and runs as `EVEY_DB_APP_USER`. | none |
| `EVEY_DB_APP_USER` | `evey_app` | The unprivileged role the server and its migrations run as when `EVEY_DB_USER` is a superuser. Created at boot, handed the database and its objects, given a new random password on every boot (never written anywhere). An SQL injection then cannot run programs or read files on the host. | none |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` runs everything as `EVEY_DB_USER`, superuser included. A way out if a database refuses the split; the boot otherwise stops and names this variable. | none |
| `EVEY_DB_NAME` | `evey` | The database name. | none |
| `EVEY_DB_PASSWORD` | generated | The password. With an external database the server refuses to start on the value `evey`, and warns on `changeme`. | none |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | A file holding the password, read when `EVEY_DB_PASSWORD` is unset. Set by the image. | none |
| `EVEY_DB_SSLMODE` | `disable` on loopback, `prefer` otherwise | `disable`, `prefer`, `require` or `verify-full`. `prefer` against a remote host logs a warning: use `require` or `verify-full`. | none |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Open connections to the database. Each costs memory on the Postgres side; raise it only for a large deployment. | none |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Connections kept idle in the pool. Capped at the open count. | none |

## Search

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` uses the Meilisearch at `EVEY_MEILI_URL` instead of the embedded one. Read by the entrypoint. | none |
| `EVEY_MEILI_URL` | `http://localhost:7700` | The Meilisearch address. | none |
| `MEILI_MASTER_KEY` | generated | The Meilisearch key. With an external instance, an empty or placeholder key refuses the start; a key under 16 characters logs a warning. | none |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | A file holding the key, read when `MEILI_MASTER_KEY` is unset. Set by the image. | none |
| `MEILI_DATA_DIR` | `/data/meili` | Where the embedded index lives. Read by the entrypoint. | none |
| `MEILI_MAX_INDEXING_MEMORY` | about 20 % of the container's memory, at least 128 MiB | The memory the embedded Meilisearch may use for indexing. Computed from the container limit, not the machine's memory, so a scan does not exhaust a small container. An explicit value is passed through. | none |

<!-- sources
internal/adapters/driven/postgres/approle.go - EnsureAppRole, HandOverToAppRole (approle_test.go)
cmd/evey/database.go - appLogin
internal/config/config.go - EVEY_DB_*, defaultSSLMode, envSecret (_FILE), EVEY_MEILI_URL, MEILI_MASTER_KEY ; WarnValidate (password 'evey' / 'changeme', sslmode prefer, idle > open, key length)
docker/entrypoint.sh - EVEY_EMBEDDED_POSTGRES, EVEY_EMBEDDED_MEILI, EVEY_POSTGRES_DATA_DIR, EVEY_POSTGRES_IMPORT, MEILI_DATA_DIR, MEILI_MAX_INDEXING_MEMORY
Dockerfile - EVEY_DB_PASSWORD_FILE, MEILI_MASTER_KEY_FILE
-->
