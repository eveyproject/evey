---
source_sha: c19f05aea8bece6fa3bf10a82d5590583384092dde0102fcdd2192bff8c0c547
reviewed: false
---
# Base de datos y búsqueda

Por defecto la imagen ejecuta su propio Postgres y su propio Meilisearch y genera
sus contraseñas: no hace falta ninguna de estas variables. Sirven a una
instalación que trae su propia base de datos o su propio motor de búsqueda, o que
ajusta los integrados. Consulta
[Despliegues avanzados](../operations/advanced-deployments.md).

## Base de datos

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | sin definir | Sin definir: integrada cuando `EVEY_DB_HOST` no está definida, externa en caso contrario. `true` o `false` para decidirlo de forma explícita. La lee el entrypoint. |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | Dónde vive el clúster integrado. Debe estar en un sistema de archivos local, no en un recurso compartido de red. |
| `EVEY_DB_HOST` | `localhost` | El host de la base de datos. **Definirla desactiva la base de datos integrada.** |
| `EVEY_DB_PORT` | `5432` | El puerto de la base de datos. |
| `EVEY_DB_USER` | `evey` | El usuario de la base de datos. |
| `EVEY_DB_NAME` | `evey` | El nombre de la base de datos. |
| `EVEY_DB_PASSWORD` | generada | La contraseña. Con una base de datos externa, el servidor se niega a arrancar con el valor `evey`. |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Un archivo con la contraseña, que se lee cuando `EVEY_DB_PASSWORD` no está definida. La define la imagen. |
| `EVEY_DB_SSLMODE` | `disable` en loopback, `prefer` en otro caso | `disable`, `prefer`, `require` o `verify-full`. |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Conexiones abiertas a la base de datos. Cada una cuesta memoria en el lado de Postgres; súbela solo para un despliegue grande. |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Conexiones mantenidas inactivas en el pool. |

## Búsqueda

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` usa el Meilisearch de `EVEY_MEILI_URL` en lugar del integrado. La lee el entrypoint. |
| `EVEY_MEILI_URL` | `http://localhost:7700` | La dirección de Meilisearch. |
| `MEILI_MASTER_KEY` | generada | La clave de Meilisearch. Con una instancia externa, una clave vacía o de ejemplo impide el arranque. |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Un archivo con la clave, que se lee cuando `MEILI_MASTER_KEY` no está definida. La define la imagen. |
| `MEILI_MAX_INDEXING_MEMORY` | alrededor del 20 % de la memoria del contenedor, al menos 128 MiB | La memoria que el Meilisearch integrado puede usar para indexar. Se calcula a partir del límite del contenedor, no de la memoria de la máquina, para que un escaneo no agote un contenedor pequeño. Un valor explícito se transmite tal cual. |
