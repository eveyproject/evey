---
source_sha: 6efce4118fc9a8bfa56a70e2648cc5f619c415c257832c5a9ea6da0e9e98e645
reviewed: true
---
# Base de datos y búsqueda

Las variables de la base de datos Postgres y del índice de Meilisearch. Por
defecto, la imagen ejecuta los dos por sí misma y genera sus contraseñas: una
instalación normal no define ninguna de estas variables. Sirven para una
instalación que trae su propia base de datos o su propio motor de búsqueda, o
que ajusta los integrados; los procedimientos están en
[Despliegues avanzados](../operations/advanced-deployments.md). Ninguna tiene
equivalente en el panel.

## Base de datos

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | sin definir | Sin definir: integrada cuando `EVEY_DB_HOST` no está definida, externa en caso contrario. `true` o `false` para decidirlo explícitamente. La lee el punto de entrada. | ninguno |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | Dónde está el clúster integrado. Tiene que estar en un sistema de archivos local, no en una carpeta compartida por red. La lee el punto de entrada. | ninguno |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | Un `pg_dump` comprimido con gzip que se restaura en el clúster integrado la primera vez que se crea, y que después se renombra `*.imported-<date>`. Se ignora, con un mensaje, cuando el clúster ya existe. La lee el punto de entrada. | ninguno |
| `EVEY_DB_HOST` | `localhost` | El equipo de la base de datos. **Definirla desactiva la base de datos integrada.** | ninguno |
| `EVEY_DB_PORT` | `5432` | El puerto de la base de datos. | ninguno |
| `EVEY_DB_USER` | `evey` | El usuario de la base de datos. Cuando es un superusuario (el clúster integrado y el compose que viene con Evey), el servidor lo reserva solo para la restauración y funciona como `EVEY_DB_APP_USER`. | ninguno |
| `EVEY_DB_APP_USER` | `evey_app` | El rol sin privilegios con el que funcionan el servidor y sus migraciones cuando `EVEY_DB_USER` es un superusuario. Se crea al arrancar, recibe la base de datos y sus objetos, y obtiene una contraseña aleatoria nueva en cada arranque (nunca se escribe en ningún sitio). Así, una inyección SQL no puede ejecutar programas ni leer archivos en el equipo. | ninguno |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` lo ejecuta todo como `EVEY_DB_USER`, también como superusuario. Una solución alternativa si una base de datos rechaza la separación; si no, el arranque se detiene y nombra esta variable. | ninguno |
| `EVEY_DB_NAME` | `evey` | El nombre de la base de datos. | ninguno |
| `EVEY_DB_PASSWORD` | generada | La contraseña. Con una base de datos externa, el servidor se niega a arrancar con el valor `evey` y avisa con `changeme`. | ninguno |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Un archivo que contiene la contraseña, leído cuando `EVEY_DB_PASSWORD` no está definida. La define la imagen. | ninguno |
| `EVEY_DB_SSLMODE` | `disable` en la interfaz local, `prefer` en los demás casos | `disable`, `prefer`, `require` o `verify-full`. `prefer` contra un equipo remoto escribe un aviso en el registro: usa `require` o `verify-full`. | ninguno |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Conexiones abiertas a la base de datos. Cada una cuesta memoria del lado de Postgres; súbelo solo para una instalación grande. | ninguno |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Conexiones que se mantienen en reposo en el grupo de conexiones. Limitado al número de conexiones abiertas. | ninguno |

## Búsqueda

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` usa el Meilisearch de `EVEY_MEILI_URL` en lugar del integrado. La lee el punto de entrada. | ninguno |
| `EVEY_MEILI_URL` | `http://localhost:7700` | La dirección de Meilisearch. | ninguno |
| `MEILI_MASTER_KEY` | generada | La clave de Meilisearch. Con una instancia externa, una clave vacía o de ejemplo impide el arranque; una clave de menos de 16 caracteres escribe un aviso en el registro. | ninguno |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Un archivo que contiene la clave, leído cuando `MEILI_MASTER_KEY` no está definida. La define la imagen. | ninguno |
| `MEILI_DATA_DIR` | `/data/meili` | Dónde está el índice integrado. La lee el punto de entrada. | ninguno |
| `MEILI_MAX_INDEXING_MEMORY` | alrededor del 20 % de la memoria del contenedor, como mínimo 128 MiB | La memoria que puede usar el Meilisearch integrado para indexar. Se calcula a partir del límite del contenedor, no de la memoria de la máquina, para que un escaneo no agote un contenedor pequeño. Un valor explícito se transmite tal cual. | ninguno |

<!-- sources
internal/adapters/driven/postgres/approle.go - EnsureAppRole, HandOverToAppRole (approle_test.go)
cmd/evey/database.go - appLogin
internal/config/config.go - EVEY_DB_*, defaultSSLMode, envSecret (_FILE), EVEY_MEILI_URL, MEILI_MASTER_KEY ; WarnValidate (password 'evey' / 'changeme', sslmode prefer, idle > open, key length)
docker/entrypoint.sh - EVEY_EMBEDDED_POSTGRES, EVEY_EMBEDDED_MEILI, EVEY_POSTGRES_DATA_DIR, EVEY_POSTGRES_IMPORT, MEILI_DATA_DIR, MEILI_MAX_INDEXING_MEMORY
Dockerfile - EVEY_DB_PASSWORD_FILE, MEILI_MASTER_KEY_FILE
-->
